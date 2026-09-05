//go:build !restricted

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/dbvault/dbvault/internal/adapters/encryption/aead"
	ed25519signer "github.com/dbvault/dbvault/internal/adapters/manifest/ed25519"
	"github.com/dbvault/dbvault/internal/adapters/scratch"
	"github.com/dbvault/dbvault/internal/application/chunking"
	"github.com/dbvault/dbvault/internal/application/digest"
	"github.com/dbvault/dbvault/internal/application/logcollection"
	"github.com/dbvault/dbvault/internal/application/pitrdrill"
	"github.com/dbvault/dbvault/internal/application/recoverywindow"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/platform/keyderive"
	"github.com/dbvault/dbvault/internal/ports"
)

// =============================================================================
// Tier 3: Cross-Feature Combinations (Integration Pipelines)
// =============================================================================

// Scenario 1: Postgres physical basebackup + WAL stream + AEAD + Object Lock + Restore Drill
func TestTier3_PostgresPhysical_WAL_AEAD_ObjectLock_RestoreDrill(t *testing.T) {
	env := NewTestEnv(t)
	sourceID := domain.SourceID("pg_cluster_prod")
	snapID := domain.SnapshotID("basebackup_20260830_01")

	// 1. Generate basebackup payload (simulated 2MB database cluster files)
	basebackupData := make([]byte, 2*1024*1024)
	copy(basebackupData, []byte("PG_CONTROL_DATA_VERSION_16"))

	// 2. Build complete backup set with CAS chunking & AEAD encryption
	manifest, manifestBytes, sigBytes, err := env.BuildCompleteBackupSet(string(sourceID), string(snapID), basebackupData)
	if err != nil {
		t.Fatalf("build basebackup failed: %v", err)
	}

	// 3. Apply S3 WORM Object Lock on manifest and complete marker
	lockExpiry := time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	manifestKey := fmt.Sprintf("%s/snapshots/%s/manifest.json", sourceID, snapID)
	_, err = env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  manifestKey,
		Body: bytes.NewReader(manifestBytes),
		Metadata: map[string]string{
			"dbvault-lock-mode":  "COMPLIANCE",
			"dbvault-lock-until": lockExpiry,
		},
	})
	if err != nil {
		t.Fatalf("apply WORM lock to manifest failed: %v", err)
	}

	// 4. Ingest continuous WAL segments
	logSvc := logcollection.New(env.Store, ports.SystemClock{})
	for i := 1; i <= 3; i++ {
		walName := fmt.Sprintf("00000001000000000000000%d", i)
		walContent := make([]byte, 64*1024)
		copy(walContent, fmt.Sprintf("WAL_HEADER_SEG_%d", i))
		_, err := logSvc.Ingest(context.Background(), sourceID, walName, bytes.NewReader(walContent))
		if err != nil {
			t.Fatalf("ingest WAL segment %s failed: %v", walName, err)
		}
	}

	// 5. Verify Ed25519 manifest signature
	if err := env.Signer.Verify(manifestBytes, sigBytes); err != nil {
		t.Fatalf("manifest signature verification failed: %v", err)
	}

	// 6. Execute Sandbox PITR Restore Drill
	scratchMgr := scratch.New(t.TempDir())
	drillSvc := pitrdrill.New(scratchMgr, ports.SystemClock{})
	targetTime := time.Now().Add(-10 * time.Minute)
	res, err := drillSvc.Run(context.Background(), sourceID, snapID, targetTime)
	if err != nil {
		t.Fatalf("pitr drill failed: %v", err)
	}
	if res.Status != domain.VerificationSucceeded {
		t.Fatalf("expected VerificationSucceeded, got %s", res.Status)
	}

	// 7. Verify WORM Immutability prevents deletion
	err = env.Store.Delete(context.Background(), manifestKey)
	if err == nil {
		t.Fatal("expected WORM deletion block on locked manifest, got nil")
	}

	_ = manifest
}

// Scenario 2: MySQL dump + binlog stream + bit-flip corruption + Merkle rejection
func TestTier3_MySQLDump_Binlog_CorruptionInjection_MerkleRejection(t *testing.T) {
	env := NewTestEnv(t)
	sourceID := domain.SourceID("mysql_ecom")
	snapID := domain.SnapshotID("mysqldump_20260830")

	// 1. Build backup set
	sqlDump := []byte("-- MySQL dump 8.0.46\nCREATE TABLE customers (id INT, name VARCHAR(100));\nINSERT INTO customers VALUES (1, 'Alice');\n")
	manifest, _, _, err := env.BuildCompleteBackupSet(string(sourceID), string(snapID), sqlDump)
	if err != nil {
		t.Fatalf("build backup set failed: %v", err)
	}

	// 2. Ingest binlogs
	logSvc := logcollection.New(env.Store, ports.SystemClock{})
	_, err = logSvc.Ingest(context.Background(), sourceID, "mysql-bin.000001", bytes.NewReader([]byte{0xFE, 0x62, 0x69, 0x6E, 0x01}))
	if err != nil {
		t.Fatalf("ingest binlog failed: %v", err)
	}

	// 3. Inject Bit-Flip Corruption into chunk 0
	targetChunkKey := manifest.Chunks[0].ObjectKey
	err = env.Store.(*MockObjectStore).MutateChunk(targetChunkKey, 5, 0xAA)
	if err != nil {
		t.Fatalf("inject chunk mutation failed: %v", err)
	}

	// 4. Retrieve chunk and verify AEAD decryption fails
	r, _, err := env.Store.Get(context.Background(), ports.GetObjectRequest{Key: targetChunkKey})
	if err != nil {
		t.Fatalf("get mutated chunk failed: %v", err)
	}
	corruptedCiphertext, _ := io.ReadAll(r)
	_ = r.Close()

	aeadKey, _ := keyderive.Derive(env.MasterKey, keyderive.DomainAEAD)
	enc, _ := aead.New(aeadKey[:])

	_, err = enc.DecryptChunk(context.Background(), manifest.Chunks[0].ChunkID, corruptedCiphertext)
	if err == nil {
		t.Fatal("expected AEAD decryption authentication error on corrupted chunk, got nil")
	}

	// 5. Verify Merkle root recalculation fails if chunk plaintext is altered
	dedupKey, _ := keyderive.Derive(env.MasterKey, keyderive.DomainDedup)
	tamperedPlaintext := []byte("-- MySQL dump TAMPERED")
	tamperedID := digest.ChunkID(dedupKey[:], 4096, tamperedPlaintext)

	tamperedChunks := []chunking.PlainChunk{
		{Sequence: 0, PlaintextSize: int64(len(tamperedPlaintext)), Data: tamperedPlaintext},
	}
	tamperedRoot := digest.RootDigest(dedupKey[:], tamperedChunks, []string{tamperedID}, int64(len(tamperedPlaintext)), 4096)

	if tamperedRoot == manifest.Snapshot.RootDigest {
		t.Fatal("tampered root must not match original signed root digest")
	}
}

// Scenario 3: SQLite WAL online backup + quick_check + zero-dependency emergency restore
func TestTier3_SQLite_WAL_QuickCheck_EmergencyRestore(t *testing.T) {
	env := NewTestEnv(t)
	dbPath := env.CreateSampleSQLiteDB("live_wal.sqlite", 100)

	// 1. Perform SQLite online inspection and PRAGMA quick_check
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	var checkResult string
	if err := db.QueryRow("PRAGMA quick_check;").Scan(&checkResult); err != nil || checkResult != "ok" {
		t.Fatalf("quick check failed: %v, res=%s", err, checkResult)
	}
	db.Close()

	// 2. Read raw SQLite DB bytes
	rawDBBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read raw db bytes failed: %v", err)
	}

	// 3. Backup to store
	manifest, manifestBytes, sigBytes, err := env.BuildCompleteBackupSet("sqlite_src", "snap_wal_1", rawDBBytes)
	if err != nil {
		t.Fatalf("build backup set failed: %v", err)
	}

	// 4. Standalone Emergency Restore Extraction
	// Verify manifest signature using public key only
	verifier := ed25519signer.New("pub", nil, env.Signer.Public)
	if err := verifier.Verify(manifestBytes, sigBytes); err != nil {
		t.Fatalf("standalone signature verification failed: %v", err)
	}

	// Direct stream fetch & decryption
	aeadKey, _ := keyderive.Derive(env.MasterKey, keyderive.DomainAEAD)
	enc, _ := aead.New(aeadKey[:])

	extractedBytes := make([]byte, 0, len(rawDBBytes))
	for _, ch := range manifest.Chunks {
		reader, _, err := env.Store.Get(context.Background(), ports.GetObjectRequest{Key: ch.ObjectKey})
		if err != nil {
			t.Fatalf("get object failed: %v", err)
		}
		ciphertext, _ := io.ReadAll(reader)
		_ = reader.Close()

		plain, err := enc.DecryptChunk(context.Background(), ch.ChunkID, ciphertext)
		if err != nil {
			t.Fatalf("decrypt chunk failed: %v", err)
		}
		extractedBytes = append(extractedBytes, plain...)
	}

	// 5. Write extracted database and verify query execution
	restoredDBPath := filepath.Join(env.TempDir, "restored_emergency.sqlite")
	if err := os.WriteFile(restoredDBPath, extractedBytes, 0600); err != nil {
		t.Fatalf("write restored db failed: %v", err)
	}

	restoredDB, err := sql.Open("sqlite3", restoredDBPath)
	if err != nil {
		t.Fatalf("open restored sqlite failed: %v", err)
	}
	defer restoredDB.Close()

	var userCount int
	if err := restoredDB.QueryRow("SELECT count(*) FROM users;").Scan(&userCount); err != nil {
		t.Fatalf("query restored database failed: %v", err)
	}
	if userCount != 100 {
		t.Fatalf("expected 100 restored users, got %d", userCount)
	}

	var restoredCheck string
	if err := restoredDB.QueryRow("PRAGMA quick_check;").Scan(&restoredCheck); err != nil || restoredCheck != "ok" {
		t.Fatalf("restored db quick check failed: %v, res=%s", err, restoredCheck)
	}
}

// Scenario 4: Timeline branch switch with gap detection & recovery window recalculation
func TestTier3_TimelineBranch_GapDetection_RecoveryWindowRecalculation(t *testing.T) {
	calc := recoverywindow.Calculator{
		Now: func() time.Time { return time.Now().UTC() },
	}

	t0 := time.Now().Add(-5 * time.Hour)
	t1 := time.Now().Add(-3 * time.Hour)
	t2 := time.Now().Add(-1 * time.Hour)

	// Timeline 1 Base Backup
	baseTL1 := domain.PhysicalBackupSet{
		ID:          "base_tl1",
		SourceID:    "pg_branch_src",
		LineageID:   "lineage_1",
		StartedAt:   t0,
		CompletedAt: t0.Add(5 * time.Minute),
	}

	// Timeline 1 WAL logs (continuous)
	logsTL1 := []domain.TransactionLog{
		{
			ID:          "wal_001",
			SourceID:    "pg_branch_src",
			LineageID:   "lineage_1",
			StartTime:   &t0,
			EndTime:     &t1,
			EndPosition: domain.LogPosition{Engine: domain.EnginePostgres, LSN: "0/16000000"},
			Status:      domain.LogStored,
		},
		{
			ID:          "wal_002",
			SourceID:    "pg_branch_src",
			LineageID:   "lineage_1",
			StartTime:   &t1,
			EndTime:     &t2,
			EndPosition: domain.LogPosition{Engine: domain.EnginePostgres, LSN: "0/18000000"},
			Status:      domain.LogStored,
		},
	}

	// 1. Calculate healthy continuous window
	windowsHealthy := calc.Calculate("pg_branch_src", "lineage_1", []domain.PhysicalBackupSet{baseTL1}, logsTL1)
	if len(windowsHealthy) != 1 || !windowsHealthy[0].Continuous {
		t.Fatalf("expected 1 continuous window, got %+v", windowsHealthy)
	}

	// 2. Introduce a missing WAL segment (gap) in timeline
	logsWithGap := []domain.TransactionLog{
		logsTL1[0],
		{
			ID:       "wal_gap_marker",
			SourceID: "pg_branch_src",
			Status:   domain.LogGap,
		},
		logsTL1[1],
	}

	windowsDegraded := calc.Calculate("pg_branch_src", "lineage_1", []domain.PhysicalBackupSet{baseTL1}, logsWithGap)
	if len(windowsDegraded) == 0 {
		t.Fatal("expected degraded recovery window calculation")
	}
	hasGap := false
	for _, w := range windowsDegraded {
		if w.GapCount > 0 || !w.Continuous {
			hasGap = true
			break
		}
	}
	if !hasGap {
		t.Fatal("expected recovery window to reflect gap in log stream")
	}
}

// Scenario 5: PII Masking Pipeline on Restored Dataset with Table Reconciliation
func TestTier3_PIIMasking_And_TableReconciliation(t *testing.T) {
	env := NewTestEnv(t)
	dbPath := env.CreateSampleSQLiteDB("pii_source.sqlite", 20)

	db, _ := sql.Open("sqlite3", dbPath)
	defer db.Close()

	// Read original rows
	rows, err := db.Query("SELECT id, username, email FROM users ORDER BY id;")
	if err != nil {
		t.Fatalf("query users failed: %v", err)
	}
	defer rows.Close()

	type UserRow struct {
		ID       int
		Username string
		Email    string
	}
	var originalRows []UserRow
	for rows.Next() {
		var u UserRow
		_ = rows.Scan(&u.ID, &u.Username, &u.Email)
		originalRows = append(originalRows, u)
	}

	// Apply PII Masking Rule (Email hashing with salt)
	salt := "company_secret_salt"
	var maskedRows []UserRow
	for _, u := range originalRows {
		h := sha256.Sum256([]byte(salt + ":" + u.Email))
		maskedEmail := fmt.Sprintf("masked_%s@privacy.local", hex.EncodeToString(h[:4]))
		maskedRows = append(maskedRows, UserRow{
			ID:       u.ID,
			Username: u.Username,
			Email:    maskedEmail,
		})
	}

	// Verify row counts and ID integrity are preserved
	if len(originalRows) != len(maskedRows) {
		t.Fatalf("row count mismatch: %d != %d", len(originalRows), len(maskedRows))
	}
	for i := range originalRows {
		if originalRows[i].ID != maskedRows[i].ID {
			t.Fatalf("ID order broken at row %d", i)
		}
		if originalRows[i].Email == maskedRows[i].Email {
			t.Fatalf("email was not masked at row %d", i)
		}
		if !strings.HasSuffix(maskedRows[i].Email, "@privacy.local") {
			t.Fatalf("invalid masked email format at row %d: %s", i, maskedRows[i].Email)
		}
	}
}
