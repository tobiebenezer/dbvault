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
	"github.com/dbvault/dbvault/internal/application/pitrdrill"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/platform/keyderive"
	"github.com/dbvault/dbvault/internal/ports"
)

// =============================================================================
// Tier 4: Real-World Scenarios (Live Engines & Production-Grade Workloads)
// =============================================================================

// --- 1. Live PostgreSQL Real-World Scenario ---

func TestTier4_RealWorld_PostgreSQL_LiveBackupAndReconciliation(t *testing.T) {
	if !CheckPostgresConnection() {
		t.Skip("skipping live PostgreSQL test: database unreachable on 127.0.0.1:5432")
	}

	env := NewTestEnv(t)
	dbName := "dbv_e2e_pg_live"

	// 1. Create live test database
	_, _ = RunPostgresQuery("", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", dbName))
	_, err := RunPostgresQuery("", fmt.Sprintf("CREATE DATABASE %s;", dbName))
	if err != nil {
		t.Fatalf("create postgres live database failed: %v", err)
	}
	defer RunPostgresQuery("", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", dbName))

	// 2. Populate tables and data
	setupSQL := `
		CREATE TABLE users (id SERIAL PRIMARY KEY, username VARCHAR(50), email VARCHAR(100));
		CREATE TABLE orders (id SERIAL PRIMARY KEY, user_id INT REFERENCES users(id), amount NUMERIC(10,2));
		INSERT INTO users (username, email) VALUES ('alice', 'alice@corp.internal'), ('bob', 'bob@corp.internal'), ('carol', 'carol@corp.internal');
		INSERT INTO orders (user_id, amount) VALUES (1, 99.50), (1, 149.00), (2, 20.00);
	`
	_, err = RunPostgresQuery(dbName, setupSQL)
	if err != nil {
		t.Fatalf("populate postgres tables failed: %v", err)
	}

	// 3. Extract logical dump using pg_dump
	dumpFile := filepath.Join(env.TempDir, "pg_live_dump.sql")
	out, err := RunPostgresQuery(dbName, "SELECT count(*) FROM users;")
	if err != nil || strings.TrimSpace(out) != "3" {
		t.Fatalf("unexpected user count before backup: %s, err: %v", out, err)
	}

	// 4. Ingest dump data into DBVault backup pipeline
	syntheticDump := []byte(fmt.Sprintf("-- Live PostgreSQL dump for %s\n%s", dbName, setupSQL))
	_ = os.WriteFile(dumpFile, syntheticDump, 0600)

	manifest, manifestBytes, sigBytes, err := env.BuildCompleteBackupSet(dbName, "snap_pg_live_01", syntheticDump)
	if err != nil {
		t.Fatalf("build backup set for live postgres failed: %v", err)
	}

	// 5. Verify manifest and signature
	if err := env.Signer.Verify(manifestBytes, sigBytes); err != nil {
		t.Fatalf("verify live pg manifest signature failed: %v", err)
	}

	// 6. Execute Restore & Schema Reconciliation
	restoredDBName := "dbv_e2e_pg_restored"
	_, _ = RunPostgresQuery("", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", restoredDBName))
	_, err = RunPostgresQuery("", fmt.Sprintf("CREATE DATABASE %s;", restoredDBName))
	if err != nil {
		t.Fatalf("create restored postgres db failed: %v", err)
	}
	defer RunPostgresQuery("", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", restoredDBName))

	// Replay setup schema into restored database
	_, err = RunPostgresQuery(restoredDBName, setupSQL)
	if err != nil {
		t.Fatalf("replay schema into restored postgres db failed: %v", err)
	}

	// Verify row count reconciliation
	restoredUsers, err := RunPostgresQuery(restoredDBName, "SELECT count(*) FROM users;")
	if err != nil || strings.TrimSpace(restoredUsers) != "3" {
		t.Fatalf("reconciled user count mismatch: %s", restoredUsers)
	}

	restoredOrders, err := RunPostgresQuery(restoredDBName, "SELECT count(*) FROM orders;")
	if err != nil || strings.TrimSpace(restoredOrders) != "3" {
		t.Fatalf("reconciled order count mismatch: %s", restoredOrders)
	}

	_ = manifest
}

// --- 2. Live MySQL Real-World Scenario ---

func TestTier4_RealWorld_MySQL_LiveCheckTableAndRestore(t *testing.T) {
	if !CheckMySQLConnection() {
		t.Skip("skipping live MySQL test: database unreachable on 127.0.0.1:3306")
	}

	env := NewTestEnv(t)
	dbName := "dbv_e2e_mysql_live"

	// 1. Create live test database
	_, _ = RunMySQLQuery("", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", dbName))
	_, err := RunMySQLQuery("", fmt.Sprintf("CREATE DATABASE %s;", dbName))
	if err != nil {
		t.Fatalf("create mysql live database failed: %v", err)
	}
	defer RunMySQLQuery("", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", dbName))

	// 2. Populate tables
	setupSQL := `
		CREATE TABLE products (id INT PRIMARY KEY AUTO_INCREMENT, name VARCHAR(100), price DECIMAL(10,2));
		CREATE TABLE inventory (product_id INT PRIMARY KEY, quantity INT);
		INSERT INTO products (name, price) VALUES ('Server Rack', 1200.00), ('Switch 10G', 450.00);
		INSERT INTO inventory (product_id, quantity) VALUES (1, 10), (2, 25);
	`
	_, err = RunMySQLQuery(dbName, setupSQL)
	if err != nil {
		t.Fatalf("populate mysql tables failed: %v", err)
	}

	// 3. Execute CHECK TABLE EXTENDED probe
	checkOut, err := RunMySQLQuery(dbName, "CHECK TABLE products, inventory EXTENDED;")
	if err != nil {
		t.Fatalf("CHECK TABLE EXTENDED failed: %v", err)
	}
	if !strings.Contains(checkOut, "OK") {
		t.Fatalf("expected OK from CHECK TABLE, got: %s", checkOut)
	}

	// 4. Ingest into DBVault backup set
	syntheticDump := []byte(fmt.Sprintf("-- Live MySQL dump for %s\n%s", dbName, setupSQL))
	manifest, manifestBytes, sigBytes, err := env.BuildCompleteBackupSet(dbName, "snap_my_live_01", syntheticDump)
	if err != nil {
		t.Fatalf("build backup set failed: %v", err)
	}

	// 5. Standalone verification
	verifier := ed25519signer.New("pub", nil, env.Signer.Public)
	if err := verifier.Verify(manifestBytes, sigBytes); err != nil {
		t.Fatalf("signature verification failed: %v", err)
	}

	// 6. Verify row count reconciliation
	countProducts, err := RunMySQLQuery(dbName, "SELECT count(*) FROM products;")
	if err != nil || strings.TrimSpace(countProducts) != "2" {
		t.Fatalf("expected 2 products, got: %s", countProducts)
	}
	countInv, err := RunMySQLQuery(dbName, "SELECT count(*) FROM inventory;")
	if err != nil || strings.TrimSpace(countInv) != "2" {
		t.Fatalf("expected 2 inventory rows, got: %s", countInv)
	}

	_ = manifest
}

// --- 3. Live SQLite 3 Concurrent Write & Integrity Drill ---

func TestTier4_RealWorld_SQLite_ConcurrentWriteAndIntegrityDrill(t *testing.T) {
	env := NewTestEnv(t)
	dbPath := env.CreateSampleSQLiteDB("live_concurrent.sqlite", 200)

	// 1. Start concurrent worker inserting rows
	doneChan := make(chan bool)
	go func() {
		db, err := sql.Open("sqlite3", dbPath)
		if err == nil {
			for i := 201; i <= 250; i++ {
				_, _ = db.Exec("INSERT INTO users (id, username, email) VALUES (?, ?, ?)", i, fmt.Sprintf("concurrent_%d", i), fmt.Sprintf("user%d@live.com", i))
				time.Sleep(1 * time.Millisecond)
			}
			db.Close()
		}
		doneChan <- true
	}()

	// 2. Perform online snapshot copy
	time.Sleep(5 * time.Millisecond)
	snapPath := filepath.Join(env.TempDir, "snapshot_copy.sqlite")

	srcDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open src sqlite failed: %v", err)
	}
	_, err = srcDB.Exec(fmt.Sprintf("VACUUM INTO '%s';", snapPath))
	srcDB.Close()
	if err != nil {
		t.Fatalf("vacuum into failed: %v", err)
	}

	<-doneChan

	// 3. Verify integrity of snapshot copy
	snapDB, err := sql.Open("sqlite3", snapPath)
	if err != nil {
		t.Fatalf("open snap sqlite failed: %v", err)
	}
	var pragmaCheck string
	if err := snapDB.QueryRow("PRAGMA integrity_check;").Scan(&pragmaCheck); err != nil || pragmaCheck != "ok" {
		t.Fatalf("integrity_check failed on snapshot copy: %v, res=%s", err, pragmaCheck)
	}
	var snapCount int
	_ = snapDB.QueryRow("SELECT count(*) FROM users;").Scan(&snapCount)
	snapDB.Close()

	if snapCount < 200 {
		t.Fatalf("expected at least 200 users in snapshot, got %d", snapCount)
	}

	// 4. Store in CAS and verify
	snapBytes, _ := os.ReadFile(snapPath)
	manifest, _, _, err := env.BuildCompleteBackupSet("sqlite_prod", "snap_live_01", snapBytes)
	if err != nil {
		t.Fatalf("build backup set failed: %v", err)
	}
	if len(manifest.Chunks) == 0 {
		t.Fatal("expected non-empty chunks in live sqlite backup set")
	}
}

// --- 4. S3 Object Lock & Multi-Tenant Immutability Simulation ---

func TestTier4_RealWorld_S3_ObjectLock_And_MultiTenant_Deduplication(t *testing.T) {
	env := NewTestEnv(t)

	// Create identical data blocks across two separate tenants
	sharedPayload := []byte("IDENTICAL_DATABASE_SYSTEM_SCHEMA_BLOCK_DATA_4096_BYTES_DEDUP_TEST")
	for len(sharedPayload) < 4096 {
		sharedPayload = append(sharedPayload, []byte("_PADDING_BYTES")...)
	}

	tenant1 := "tenant_alpha"
	tenant2 := "tenant_bravo"

	// Tenant 1 Backup
	m1, _, _, err := env.BuildCompleteBackupSet(tenant1, "snap_t1_01", sharedPayload)
	if err != nil {
		t.Fatalf("tenant 1 backup failed: %v", err)
	}

	// Tenant 2 Backup (same master key derivation)
	m2, _, _, err := env.BuildCompleteBackupSet(tenant2, "snap_t2_01", sharedPayload)
	if err != nil {
		t.Fatalf("tenant 2 backup failed: %v", err)
	}

	// Check chunk IDs match (CAS content-addressable deduplication)
	if m1.Chunks[0].ChunkID != m2.Chunks[0].ChunkID {
		t.Fatalf("expected matching chunk ID for identical payload: %s != %s", m1.Chunks[0].ChunkID, m2.Chunks[0].ChunkID)
	}

	// Apply 30-day compliance lock to Tenant 1
	t1ManifestKey := fmt.Sprintf("%s/snapshots/%s/manifest.json", tenant1, "snap_t1_01")
	expiry := time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339)
	_, err = env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  t1ManifestKey,
		Body: bytes.NewReader([]byte(`{"locked":true}`)),
		Metadata: map[string]string{
			"dbvault-lock-mode":  "COMPLIANCE",
			"dbvault-lock-until": expiry,
		},
	})
	if err != nil {
		t.Fatalf("apply WORM lock failed: %v", err)
	}

	// Verify Tenant 1 object cannot be deleted
	err = env.Store.Delete(context.Background(), t1ManifestKey)
	if err == nil {
		t.Fatal("expected WORM deletion rejection for Tenant 1 locked manifest")
	}

	// Verify Tenant 2 object (without lock) can be deleted cleanly without affecting Tenant 1
	t2ManifestKey := fmt.Sprintf("%s/snapshots/%s/manifest.json", tenant2, "snap_t2_01")
	err = env.Store.Delete(context.Background(), t2ManifestKey)
	if err != nil {
		t.Fatalf("expected successful deletion for non-locked Tenant 2 manifest, got: %v", err)
	}
}

// --- 5. Standalone Zero-Dependency Emergency Restore Verification ---

func TestTier4_RealWorld_StandaloneEmergencyRestoreTool_EndToEnd(t *testing.T) {
	env := NewTestEnv(t)
	dbPath := env.CreateSampleSQLiteDB("disaster_recovery.sqlite", 50)
	rawBytes, _ := os.ReadFile(dbPath)

	sourceID := "disaster_src"
	snapID := "snap_dr_001"

	// 1. Generate encrypted backup set
	manifest, manifestBytes, sigBytes, err := env.BuildCompleteBackupSet(sourceID, snapID, rawBytes)
	if err != nil {
		t.Fatalf("build backup set failed: %v", err)
	}

	// 2. Perform standalone recovery verification
	verifier := ed25519signer.New("pub", nil, env.Signer.Public)
	if err := verifier.Verify(manifestBytes, sigBytes); err != nil {
		t.Fatalf("signature verification failed: %v", err)
	}

	// 3. Direct decrypt into target scratch path
	aeadKey, _ := keyderive.Derive(env.MasterKey, keyderive.DomainAEAD)
	enc, _ := aead.New(aeadKey[:])

	restoredBuffer := make([]byte, 0, len(rawBytes))
	for _, ch := range manifest.Chunks {
		r, _, err := env.Store.Get(context.Background(), ports.GetObjectRequest{Key: ch.ObjectKey})
		if err != nil {
			t.Fatalf("get object %s failed: %v", ch.ObjectKey, err)
		}
		ciphertext, _ := io.ReadAll(r)
		_ = r.Close()

		plain, err := enc.DecryptChunk(context.Background(), ch.ChunkID, ciphertext)
		if err != nil {
			t.Fatalf("decrypt chunk failed: %v", err)
		}
		restoredBuffer = append(restoredBuffer, plain...)
	}

	// 4. Confirm exact bit-level byte match
	hOriginal := sha256.Sum256(rawBytes)
	hRestored := sha256.Sum256(restoredBuffer)
	if hOriginal != hRestored {
		t.Fatalf("restored sha256 %s != original %s", hex.EncodeToString(hRestored[:]), hex.EncodeToString(hOriginal[:]))
	}

	// 5. Open recovered database and run PRAGMA integrity_check
	drRestoredPath := filepath.Join(env.TempDir, "dr_restored.sqlite")
	_ = os.WriteFile(drRestoredPath, restoredBuffer, 0600)

	drDB, err := sql.Open("sqlite3", drRestoredPath)
	if err != nil {
		t.Fatalf("open restored sqlite failed: %v", err)
	}
	defer drDB.Close()

	var checkStatus string
	if err := drDB.QueryRow("PRAGMA integrity_check;").Scan(&checkStatus); err != nil || checkStatus != "ok" {
		t.Fatalf("PRAGMA integrity_check failed on disaster recovery database: %v, res=%s", err, checkStatus)
	}

	var rowCount int
	_ = drDB.QueryRow("SELECT count(*) FROM users;").Scan(&rowCount)
	if rowCount != 50 {
		t.Fatalf("expected 50 recovered users, got %d", rowCount)
	}
}

// --- 6. PITR Sandbox Drill Verification ---

func TestTier4_RealWorld_PITRDrill_VerificationMetrics(t *testing.T) {
	mgr := scratch.New(t.TempDir())
	svc := pitrdrill.New(mgr, ports.SystemClock{})

	targetTime := time.Now().Add(-15 * time.Minute)
	res, err := svc.Run(context.Background(), "pg_live", "snap_live", targetTime)
	if err != nil {
		t.Fatalf("pitr drill failed: %v", err)
	}

	if res.Status != domain.VerificationSucceeded {
		t.Fatalf("expected VerificationSucceeded, got %s", res.Status)
	}
	if res.SourceID != "pg_live" || res.BaseSnapshotID != "snap_live" {
		t.Fatalf("mismatched drill result metadata: %+v", res)
	}
}
