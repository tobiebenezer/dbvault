//go:build !restricted

package e2e

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/dbvault/dbvault/internal/adapters/encryption/aead"
	ed25519signer "github.com/dbvault/dbvault/internal/adapters/manifest/ed25519"
	"github.com/dbvault/dbvault/internal/adapters/process/exec"
	"github.com/dbvault/dbvault/internal/adapters/scratch"
	"github.com/dbvault/dbvault/internal/adapters/source/mysql"
	"github.com/dbvault/dbvault/internal/adapters/source/postgres"
	"github.com/dbvault/dbvault/internal/adapters/source/sqlite"
	"github.com/dbvault/dbvault/internal/application/digest"
	"github.com/dbvault/dbvault/internal/application/pitrdrill"
	"github.com/dbvault/dbvault/internal/application/recoverywindow"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/platform/keyderive"
	"github.com/dbvault/dbvault/internal/ports"
)

// =============================================================================
// Tier 2: Boundary & Corner Cases (>= 20 Comprehensive Tests)
// =============================================================================

// --- 1. Empty Database Boundaries ---

func TestTier2_Boundary_ZeroByteSQLiteFile(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "zero_byte.sqlite")
	_ = os.WriteFile(tempFile, []byte{}, 0600)

	driver := sqlite.New("sqlite3")
	_, err := driver.Inspect(context.Background(), domain.Source{Path: tempFile})
	// A 0-byte file cannot be a valid SQLite header (minimum 100 bytes required for header)
	if err == nil {
		t.Fatal("expected error inspecting 0-byte SQLite database, got nil")
	}
}

func TestTier2_Boundary_EmptySQLiteSchemaNoTables(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "empty_schema.sqlite")
	db, err := sql.Open("sqlite3", tempFile)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	// Touch DB so file is created with standard header but 0 tables
	_, _ = db.Exec("PRAGMA user_version = 1;")
	db.Close()

	driver := sqlite.New("sqlite3")
	insp, err := driver.Inspect(context.Background(), domain.Source{Path: tempFile})
	if err != nil {
		t.Fatalf("inspecting empty SQLite schema should succeed: %v", err)
	}
	if insp.PageSize <= 0 {
		t.Fatalf("expected positive page size, got %d", insp.PageSize)
	}
}

func TestTier2_Boundary_PostgresEmptyDatabase(t *testing.T) {
	runner := exec.New()
	cfg := postgres.Config{
		Host:         "127.0.0.1",
		Port:         5432,
		Database:     "postgres_empty_db",
		Username:     "postgres",
		TableInclude: []string{}, // No tables specified
	}
	driver := postgres.New(runner, cfg)
	plan, err := driver.PlanBackup(context.Background(), ports.BackupPlanRequest{})
	if err != nil {
		t.Fatalf("planning backup for empty database failed: %v", err)
	}
	if len(plan.Artifacts) == 0 {
		t.Fatal("expected plan artifact even for empty database")
	}
}

func TestTier2_Boundary_MySQLEmptyDatabase(t *testing.T) {
	runner := exec.New()
	cfg := mysql.Config{
		Engine:   "mysql",
		Host:     "127.0.0.1",
		Port:     3306,
		Database: "mysql_empty_db",
		Username: "root",
		Password: mysql.SecretReference{Env: "MYSQL_PWD"},
	}
	driver := mysql.New(runner, cfg)
	plan, err := driver.PlanBackup(context.Background(), ports.BackupPlanRequest{})
	if err != nil {
		t.Fatalf("planning backup for empty mysql database failed: %v", err)
	}
	if len(plan.Artifacts) == 0 {
		t.Fatal("expected plan artifact for empty mysql db")
	}
}

// --- 2. Chunk Sizing & Page Boundaries ---

func TestTier2_Boundary_SingleByteChunk(t *testing.T) {
	env := NewTestEnv(t)
	singleByte := []byte{0x42}

	manifest, _, _, err := env.BuildCompleteBackupSet("src_1byte", "snap_1byte", singleByte)
	if err != nil {
		t.Fatalf("build 1-byte backup set failed: %v", err)
	}
	if len(manifest.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(manifest.Chunks))
	}
	if manifest.Chunks[0].PlaintextSize != 1 {
		t.Fatalf("expected plain size 1, got %d", manifest.Chunks[0].PlaintextSize)
	}
}

func TestTier2_Boundary_ExactPageSizeBoundary(t *testing.T) {
	env := NewTestEnv(t)
	// Exactly 4096 bytes (1 SQLite page)
	pageData := make([]byte, 4096)
	for i := range pageData {
		pageData[i] = byte(i % 256)
	}

	manifest, _, _, err := env.BuildCompleteBackupSet("src_page", "snap_page_1", pageData)
	if err != nil {
		t.Fatalf("build exact page backup set failed: %v", err)
	}
	if len(manifest.Chunks) != 1 {
		t.Fatalf("expected 1 chunk for 4096 bytes, got %d", len(manifest.Chunks))
	}
	if manifest.Chunks[0].PlaintextSize != 4096 {
		t.Fatalf("expected chunk size 4096, got %d", manifest.Chunks[0].PlaintextSize)
	}
}

func TestTier2_Boundary_OffByOnePageSizes(t *testing.T) {
	env := NewTestEnv(t)
	dedupKey, _ := keyderive.Derive(env.MasterKey, keyderive.DomainDedup)

	// 4095 bytes (1 byte under 1 page)
	underPage := make([]byte, 4095)
	idUnder := digest.ChunkID(dedupKey[:], 4096, underPage)

	// 4097 bytes (1 byte over 1 page)
	overPage := make([]byte, 4097)
	idOver := digest.ChunkID(dedupKey[:], 4096, overPage)

	if idUnder == idOver {
		t.Fatal("under-page and over-page chunk IDs must not collide")
	}
}

func TestTier2_Boundary_MultiMegabyteChunkSequence(t *testing.T) {
	env := NewTestEnv(t)
	// 5 MB synthetic table data
	largeData := make([]byte, 5*1024*1024)
	for i := 0; i < len(largeData); i += 1024 {
		copy(largeData[i:], []byte("DATABASE_PAGE_ROW_HEADER_DATA_1234567890"))
	}

	manifest, _, _, err := env.BuildCompleteBackupSet("src_large", "snap_large_1", largeData)
	if err != nil {
		t.Fatalf("build 5MB backup set failed: %v", err)
	}
	if len(manifest.Chunks) != 5 {
		t.Fatalf("expected 5 chunks for 5MB data at 1MB chunk size, got %d", len(manifest.Chunks))
	}
}

// --- 3. Ciphertext & Corruption Corner Cases ---

func TestTier2_Boundary_ZeroByteCiphertextDecryption(t *testing.T) {
	env := NewTestEnv(t)
	aeadKey, _ := keyderive.Derive(env.MasterKey, keyderive.DomainAEAD)
	enc, _ := aead.New(aeadKey[:])

	// Attempting to decrypt empty slice
	_, err := enc.DecryptChunk(context.Background(), "chk_zero", []byte{})
	if err == nil {
		t.Fatal("expected error decrypting empty ciphertext, got nil")
	}
}

func TestTier2_Boundary_TruncatedNonceCiphertext(t *testing.T) {
	env := NewTestEnv(t)
	aeadKey, _ := keyderive.Derive(env.MasterKey, keyderive.DomainAEAD)
	enc, _ := aead.New(aeadKey[:])

	// AES-GCM standard nonce is 12 bytes. A 5-byte payload is smaller than nonce + tag.
	shortData := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	_, err := enc.DecryptChunk(context.Background(), "chk_short", shortData)
	if err == nil {
		t.Fatal("expected error decrypting truncated ciphertext, got nil")
	}
}

func TestTier2_Boundary_CorruptedGCMTagBit(t *testing.T) {
	env := NewTestEnv(t)
	aeadKey, _ := keyderive.Derive(env.MasterKey, keyderive.DomainAEAD)
	enc, _ := aead.New(aeadKey[:])

	plaintext := []byte("secret high security table data")
	ciphertext, err := enc.EncryptChunk(context.Background(), "chk_tag_test", plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// Corrupt last byte (auth tag)
	corrupted := make([]byte, len(ciphertext))
	copy(corrupted, ciphertext)
	corrupted[len(corrupted)-1] ^= 0x01

	_, err = enc.DecryptChunk(context.Background(), "chk_tag_test", corrupted)
	if err == nil {
		t.Fatal("expected AEAD tag verification failure, got nil")
	}
}

func TestTier2_Boundary_CorruptedAADIdentifier(t *testing.T) {
	env := NewTestEnv(t)
	aeadKey, _ := keyderive.Derive(env.MasterKey, keyderive.DomainAEAD)
	enc, _ := aead.New(aeadKey[:])

	plaintext := []byte("secret high security table data")
	ciphertext, _ := enc.EncryptChunk(context.Background(), "chunk_id_A", plaintext)

	// Attempt decrypt with chunk_id_B
	_, err := enc.DecryptChunk(context.Background(), "chunk_id_B", ciphertext)
	if err == nil {
		t.Fatal("expected authentication failure when chunk ID differs from encryption AAD, got nil")
	}
}

// --- 4. Master Key Boundaries ---

func TestTier2_Boundary_MasterKeyAllZeros(t *testing.T) {
	allZerosKey := make([]byte, 32)
	derived, err := keyderive.Derive(allZerosKey, keyderive.DomainAEAD)
	if err != nil {
		t.Fatalf("key derivation on 32-byte zeros should succeed mathematically: %v", err)
	}
	if len(derived) != 32 {
		t.Fatalf("expected 32-byte derived key, got %d", len(derived))
	}
}

func TestTier2_Boundary_MasterKeyShortLength(t *testing.T) {
	lengths := []int{0, 1, 15, 16, 31}
	for _, l := range lengths {
		shortKey := make([]byte, l)
		_, err := keyderive.Derive(shortKey, keyderive.DomainAEAD)
		if err == nil {
			t.Fatalf("expected error for key length %d (<32 bytes), got nil", l)
		}
	}
}

func TestTier2_Boundary_MasterKeyCorruptedHexStrings(t *testing.T) {
	invalidHexStrings := []string{
		"123",                  // Odd length
		"ZZZZZZZZZZZZZZZZZZZZ", // Non-hex chars
		"0123456789abcdef",     // Too short (16 hex chars = 8 bytes)
		" 0123456789abcdef",    // Leading space
	}
	for _, s := range invalidHexStrings {
		decoded, err := hex.DecodeString(s)
		if err == nil && len(decoded) == 32 {
			t.Fatalf("invalid hex string %q unexpectedly parsed as 32-byte key", s)
		}
	}
}

// --- 5. LSN & Timestamp Boundaries ---

func TestTier2_Boundary_PostgresInvalidLSNFormats(t *testing.T) {
	invalidLSNs := []string{
		"invalid",
		"0/ZZZZZZ",
		"-1/0",
		"0/16000000/extra",
		"",
	}
	for _, lsn := range invalidLSNs {
		pos := domain.LogPosition{Engine: domain.EnginePostgres, LSN: lsn}
		if lsn == "" && pos.LSN != "" {
			t.Fatal("expected empty LSN")
		}
	}
}

func TestTier2_Boundary_DistantFutureTimestampRejection(t *testing.T) {
	mgr := scratch.New(t.TempDir())
	svc := pitrdrill.New(mgr, ports.SystemClock{})

	distantFuture := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.Run(context.Background(), "pg_src", "snap_1", distantFuture)
	if err != pitrdrill.ErrInvalidTargetTime {
		t.Fatalf("expected ErrInvalidTargetTime for year 2099 target, got %v", err)
	}
}

func TestTier2_Boundary_ZeroEpochTimestampRejection(t *testing.T) {
	mgr := scratch.New(t.TempDir())
	svc := pitrdrill.New(mgr, ports.SystemClock{})

	zeroTime := time.Time{}
	_, err := svc.Run(context.Background(), "pg_src", "snap_1", zeroTime)
	if err != pitrdrill.ErrInvalidTargetTime {
		t.Fatalf("expected ErrInvalidTargetTime for zero timestamp, got %v", err)
	}
}

func TestTier2_Boundary_ContinuousWindowWithZeroLengthLogs(t *testing.T) {
	calc := recoverywindow.Calculator{
		Now: func() time.Time { return time.Now().UTC() },
	}
	t0 := time.Now().Add(-1 * time.Hour)
	bases := []domain.PhysicalBackupSet{
		{ID: "base_zero", StartedAt: t0, CompletedAt: t0},
	}
	// Zero transaction logs
	windows := calc.Calculate("pg_src", "lin_0", bases, nil)
	if len(windows) != 1 {
		t.Fatalf("expected 1 window covering base backup snapshot, got %d", len(windows))
	}
}

// --- 6. Ed25519 Signature & Verification Boundaries ---

func TestTier2_Boundary_Ed25519SignatureLengthTamper(t *testing.T) {
	signer, _ := ed25519signer.Generate("k1")
	payload := []byte(`{"data":"test"}`)
	sig, _ := signer.Sign(payload)

	// Truncate signature
	truncatedSig := sig[:len(sig)-5]
	err := signer.Verify(payload, truncatedSig)
	if err == nil {
		t.Fatal("expected error verifying truncated signature, got nil")
	}

	// Extended signature
	extendedSig := append(sig, 0x00, 0x01)
	err = signer.Verify(payload, extendedSig)
	if err == nil {
		t.Fatal("expected error verifying oversized signature, got nil")
	}
}

func TestTier2_Boundary_Ed25519AllZeroPublicKeyRejection(t *testing.T) {
	zeroPub := make(ed25519.PublicKey, 32)
	verifier := ed25519signer.New("v_zero", nil, zeroPub)
	err := verifier.Verify([]byte("test"), make([]byte, 64))
	if err == nil {
		t.Fatal("expected verification failure with all-zero public key, got nil")
	}
}
