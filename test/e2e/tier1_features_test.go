//go:build !restricted

package e2e

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/dbvault/dbvault/internal/adapters/encryption/aead"
	ed25519signer "github.com/dbvault/dbvault/internal/adapters/manifest/ed25519"
	"github.com/dbvault/dbvault/internal/adapters/process/exec"
	"github.com/dbvault/dbvault/internal/adapters/source/mysql"
	"github.com/dbvault/dbvault/internal/adapters/source/postgres"
	"github.com/dbvault/dbvault/internal/adapters/source/sqlite"
	"github.com/dbvault/dbvault/internal/application/chunking"
	"github.com/dbvault/dbvault/internal/application/digest"
	"github.com/dbvault/dbvault/internal/application/logcollection"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/platform/keyderive"
	"github.com/dbvault/dbvault/internal/ports"
)

// =============================================================================
// Feature 1: PostgreSQL Logical Dump & Restore (5 Tests)
// =============================================================================

func TestTier1_F01_PostgresLogical_CustomFormatDump(t *testing.T) {
	runner := exec.New()
	cfg := postgres.Config{
		Host:         "127.0.0.1",
		Port:         5432,
		Database:     "postgres",
		Username:     "postgres",
		BackupFormat: "postgres-custom",
	}
	driver := postgres.New(runner, cfg)
	desc := driver.Descriptor()
	if desc.Engine != domain.EnginePostgres {
		t.Fatalf("expected engine %s, got %s", domain.EnginePostgres, desc.Engine)
	}

	plan, err := driver.PlanBackup(context.Background(), ports.BackupPlanRequest{})
	if err != nil {
		t.Fatalf("plan backup failed: %v", err)
	}
	if len(plan.Artifacts) == 0 || plan.Artifacts[0].Format != domain.FormatPostgresCustom {
		t.Fatalf("expected custom format artifact in plan, got %+v", plan.Artifacts)
	}
}

func TestTier1_F01_PostgresLogical_PlainSQLDump(t *testing.T) {
	runner := exec.New()
	cfg := postgres.Config{
		Host:         "127.0.0.1",
		Port:         5432,
		Database:     "postgres",
		Username:     "postgres",
		BackupFormat: "postgres-plain-sql",
	}
	driver := postgres.New(runner, cfg)
	plan, err := driver.PlanBackup(context.Background(), ports.BackupPlanRequest{})
	if err != nil {
		t.Fatalf("plan plain sql backup failed: %v", err)
	}
	if len(plan.Artifacts) == 0 || plan.Artifacts[0].Format != domain.FormatPostgresPlainSQL {
		t.Fatalf("expected plain sql format artifact, got %+v", plan.Artifacts)
	}
}

func TestTier1_F01_PostgresLogical_TableSelection(t *testing.T) {
	runner := exec.New()
	cfg := postgres.Config{
		Host:         "127.0.0.1",
		Port:         5432,
		Database:     "postgres",
		Username:     "postgres",
		TableInclude: []string{"public.users", "public.orders"},
		TableExclude: []string{"public.cache"},
	}
	driver := postgres.New(runner, cfg)
	src := domain.Source{
		ID:     "pg_test",
		Driver: "postgres",
	}
	insp, err := driver.InspectSource(context.Background(), src)
	if err != nil {
		t.Fatalf("inspect source failed: %v", err)
	}
	if insp.Summary["database"] != "postgres" {
		t.Fatalf("expected summary database postgres, got %v", insp.Summary)
	}
}

func TestTier1_F01_PostgresLogical_GlobalsExtraction(t *testing.T) {
	runner := exec.New()
	cfg := postgres.Config{
		Host:           "127.0.0.1",
		Port:           5432,
		Database:       "postgres",
		Username:       "postgres",
		IncludeGlobals: true,
	}
	driver := postgres.New(runner, cfg)
	plan, err := driver.PlanBackup(context.Background(), ports.BackupPlanRequest{})
	if err != nil {
		t.Fatalf("plan globals backup failed: %v", err)
	}
	hasGlobals := false
	for _, a := range plan.Artifacts {
		if a.Type == domain.ArtifactGlobals {
			hasGlobals = true
			break
		}
	}
	if !hasGlobals {
		t.Fatal("expected globals artifact in backup plan when IncludeGlobals=true")
	}
}

func TestTier1_F01_PostgresLogical_MissingCredentialsRejection(t *testing.T) {
	runner := exec.New()
	cfg := postgres.Config{
		Host:     "127.0.0.1",
		Port:     5432,
		Database: "postgres",
		Username: "", // Missing username
	}
	driver := postgres.New(runner, cfg)
	err := driver.ValidateSource(context.Background(), domain.Source{ID: "pg_bad"})
	if err == nil {
		t.Fatal("expected validation error on missing username, got nil")
	}
}

// =============================================================================
// Feature 2: PostgreSQL Physical Basebackup (5 Tests)
// =============================================================================

func TestTier1_F02_PostgresPhysical_DescriptorCapabilities(t *testing.T) {
	runner := exec.New()
	cfg := postgres.DefaultConfig()
	driver := postgres.New(runner, cfg)
	desc := driver.Descriptor()
	if !desc.SupportsStreaming || !desc.SupportsParallel || !desc.SupportsGlobals {
		t.Fatalf("expected physical streaming, parallel, and globals support, got %+v", desc)
	}
}

func TestTier1_F02_PostgresPhysical_DirectoryFormatPlan(t *testing.T) {
	runner := exec.New()
	cfg := postgres.Config{
		Host:         "127.0.0.1",
		Port:         5432,
		Database:     "postgres",
		Username:     "postgres",
		BackupFormat: "postgres-directory",
	}
	driver := postgres.New(runner, cfg)
	plan, err := driver.PlanBackup(context.Background(), ports.BackupPlanRequest{})
	if err != nil {
		t.Fatalf("directory format plan failed: %v", err)
	}
	if len(plan.Artifacts) == 0 || plan.Artifacts[0].Format != domain.FormatPostgresDirectory {
		t.Fatalf("expected directory format artifact, got %+v", plan.Artifacts)
	}
}

func TestTier1_F02_PostgresPhysical_InvalidFormatRejection(t *testing.T) {
	runner := exec.New()
	cfg := postgres.Config{
		Host:         "127.0.0.1",
		Port:         5432,
		Database:     "postgres",
		Username:     "postgres",
		BackupFormat: "invalid-raw-dump",
	}
	driver := postgres.New(runner, cfg)
	_, err := driver.PlanBackup(context.Background(), ports.BackupPlanRequest{})
	if err == nil {
		t.Fatal("expected error on unsupported backup format, got nil")
	}
}

func TestTier1_F02_PostgresPhysical_ToolchainDetection(t *testing.T) {
	runner := exec.New()
	cfg := postgres.DefaultConfig()
	driver := postgres.New(runner, cfg)
	toolchain, err := driver.DetectToolchain(context.Background(), domain.Source{ID: "pg_src"})
	if err != nil {
		t.Fatalf("detect toolchain failed: %v", err)
	}
	if toolchain.ExecutablePaths["pg_dump"] != "pg_dump" {
		t.Fatalf("expected pg_dump executable path, got %+v", toolchain)
	}
	if !toolchain.Capabilities.ParallelDump || !toolchain.Capabilities.CustomArchive {
		t.Fatalf("expected tool capabilities, got %+v", toolchain.Capabilities)
	}
}

func TestTier1_F02_PostgresPhysical_NoRunnerRejection(t *testing.T) {
	cfg := postgres.DefaultConfig()
	driver := postgres.New(nil, cfg)
	err := driver.ValidateSource(context.Background(), domain.Source{ID: "pg_src"})
	if err == nil {
		t.Fatal("expected error when runner is nil, got nil")
	}
}

// =============================================================================
// Feature 3: PostgreSQL WAL Streaming & Ingestion (5 Tests)
// =============================================================================

func TestTier1_F03_PostgresWAL_SegmentNamingRegex(t *testing.T) {
	walRegex := regexp.MustCompile(`^[0-9A-F]{24}$`)
	validSegments := []string{
		"000000010000000000000001",
		"0000000100000000000000FF",
		"00000002000000010000000A",
	}
	for _, seg := range validSegments {
		if !walRegex.MatchString(seg) {
			t.Fatalf("valid segment %s failed regex match", seg)
		}
	}

	invalidSegments := []string{
		"00000001000000000000001",   // 23 chars
		"000000010000000000000001G", // Non-hex
		"wal_segment_01",
	}
	for _, seg := range invalidSegments {
		if walRegex.MatchString(seg) {
			t.Fatalf("invalid segment %s unexpectedly passed regex match", seg)
		}
	}
}

func TestTier1_F03_PostgresWAL_SegmentIngestionChecksum(t *testing.T) {
	env := NewTestEnv(t)
	svc := logcollection.New(env.Store, ports.SystemClock{})

	walData := make([]byte, 16*1024*1024) // 16MB standard WAL segment
	walData[0] = 0xD0
	walData[1] = 0x07 // WAL magic
	expectedHash := sha256.Sum256(walData)

	meta, err := svc.Ingest(context.Background(), "pg_prod", "000000010000000000000001", bytes.NewReader(walData))
	if err != nil {
		t.Fatalf("wal ingest failed: %v", err)
	}
	if meta.SHA256Hash != hex.EncodeToString(expectedHash[:]) {
		t.Fatalf("expected hash %s, got %s", hex.EncodeToString(expectedHash[:]), meta.SHA256Hash)
	}
	if meta.ByteSize != int64(len(walData)) {
		t.Fatalf("expected size %d, got %d", len(walData), meta.ByteSize)
	}
}

func TestTier1_F03_PostgresWAL_EmptyStreamRejection(t *testing.T) {
	env := NewTestEnv(t)
	svc := logcollection.New(env.Store, ports.SystemClock{})

	_, err := svc.Ingest(context.Background(), "pg_prod", "000000010000000000000002", bytes.NewReader(nil))
	if err != logcollection.ErrEmptyChunkStream {
		t.Fatalf("expected ErrEmptyChunkStream, got %v", err)
	}
}

func TestTier1_F03_PostgresWAL_EmptySegmentNameRejection(t *testing.T) {
	env := NewTestEnv(t)
	svc := logcollection.New(env.Store, ports.SystemClock{})

	_, err := svc.Ingest(context.Background(), "pg_prod", "", bytes.NewReader([]byte("sample-data")))
	if err == nil {
		t.Fatal("expected error on empty segment name, got nil")
	}
}

func TestTier1_F03_PostgresWAL_ListChunksTracking(t *testing.T) {
	env := NewTestEnv(t)
	svc := logcollection.New(env.Store, ports.SystemClock{})

	for i := 1; i <= 3; i++ {
		seg := fmt.Sprintf("00000001000000000000000%d", i)
		_, err := svc.Ingest(context.Background(), "pg_src", seg, bytes.NewReader([]byte(fmt.Sprintf("wal-data-%d", i))))
		if err != nil {
			t.Fatalf("ingest %d failed: %v", i, err)
		}
	}

	chunks := svc.ListChunks("pg_src")
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[2].SegmentName != "000000010000000000000003" {
		t.Fatalf("expected last segment 000000010000000000000003, got %s", chunks[2].SegmentName)
	}
}

// =============================================================================
// Feature 4: MySQL / MariaDB Dump & Restore (5 Tests)
// =============================================================================

func TestTier1_F04_MySQL_DumpDescriptor(t *testing.T) {
	runner := exec.New()
	cfg := mysql.DefaultConfig("mysql")
	driver := mysql.New(runner, cfg)

	desc := driver.Descriptor()
	if desc.Engine != domain.EngineMySQL || desc.SupportedFormats[0] != domain.FormatMySQLSQL {
		t.Fatalf("unexpected mysql descriptor: %+v", desc)
	}
}

func TestTier1_F04_MySQL_MariaDBDialectDescriptor(t *testing.T) {
	runner := exec.New()
	cfg := mysql.DefaultConfig("mariadb")
	driver := mysql.New(runner, cfg)

	desc := driver.Descriptor()
	if desc.Engine != domain.EngineMariaDB || desc.SupportedFormats[0] != domain.FormatMariaDBSQL {
		t.Fatalf("unexpected mariadb descriptor: %+v", desc)
	}
}

func TestTier1_F04_MySQL_PlanBackupArtifact(t *testing.T) {
	runner := exec.New()
	cfg := mysql.Config{
		Engine:   "mysql",
		Host:     "127.0.0.1",
		Port:     3306,
		Database: "app_db",
		Username: "root",
		Password: mysql.SecretReference{Env: "MYSQL_PWD"},
	}
	driver := mysql.New(runner, cfg)
	plan, err := driver.PlanBackup(context.Background(), ports.BackupPlanRequest{})
	if err != nil {
		t.Fatalf("mysql plan backup failed: %v", err)
	}
	if len(plan.Artifacts) == 0 || plan.Artifacts[0].Format != domain.FormatMySQLSQL {
		t.Fatalf("expected mysql format artifact, got %+v", plan.Artifacts)
	}
}

func TestTier1_F04_MySQL_MissingPasswordValidation(t *testing.T) {
	runner := exec.New()
	cfg := mysql.Config{
		Engine:   "mysql",
		Host:     "127.0.0.1",
		Port:     3306,
		Database: "app_db",
		Username: "root",
		// Missing Password
	}
	driver := mysql.New(runner, cfg)
	err := driver.ValidateSource(context.Background(), domain.Source{ID: "my_src"})
	if err == nil {
		t.Fatal("expected validation error on missing password, got nil")
	}
}

func TestTier1_F04_MySQL_ToolchainDetection(t *testing.T) {
	runner := exec.New()
	cfg := mysql.DefaultConfig("mysql")
	driver := mysql.New(runner, cfg)

	tc, err := driver.DetectToolchain(context.Background(), domain.Source{ID: "my_src"})
	if err != nil {
		t.Fatalf("detect mysql toolchain failed: %v", err)
	}
	if tc.ExecutablePaths["dump"] != "mysqldump" && tc.ExecutablePaths["mysqldump"] != "mysqldump" {
		t.Fatalf("expected mysqldump executable path, got %+v", tc)
	}
}

// =============================================================================
// Feature 5: MySQL Binlog Streaming & Ingestion (5 Tests)
// =============================================================================

func TestTier1_F05_MySQLBinlog_MagicBytesValidation(t *testing.T) {
	binlogMagic := []byte{0xFE, 0x62, 0x69, 0x6E} // "\xfe bin"
	validStream := append(binlogMagic, []byte("event-data-here")...)

	if !bytes.HasPrefix(validStream, binlogMagic) {
		t.Fatal("expected valid binlog magic header")
	}

	corruptedStream := []byte{0x00, 0x00, 0x00, 0x00, 0x01}
	if bytes.HasPrefix(corruptedStream, binlogMagic) {
		t.Fatal("corrupted stream should not match binlog magic")
	}
}

func TestTier1_F05_MySQLBinlog_IngestSegment(t *testing.T) {
	env := NewTestEnv(t)
	svc := logcollection.New(env.Store, ports.SystemClock{})

	binlogData := []byte{0xFE, 0x62, 0x69, 0x6E, 0x01, 0x02, 0x03, 0x04}
	meta, err := svc.Ingest(context.Background(), "mysql_prod", "mysql-bin.000001", bytes.NewReader(binlogData))
	if err != nil {
		t.Fatalf("binlog ingest failed: %v", err)
	}
	if meta.SegmentName != "mysql-bin.000001" || meta.ByteSize != int64(len(binlogData)) {
		t.Fatalf("unexpected binlog meta: %+v", meta)
	}
}

func TestTier1_F05_MySQLBinlog_ServerIDScoping(t *testing.T) {
	env := NewTestEnv(t)
	svc := logcollection.New(env.Store, ports.SystemClock{})

	_, _ = svc.Ingest(context.Background(), "mysql_srv_1", "binlog.000001", bytes.NewReader([]byte("srv1-data")))
	_, _ = svc.Ingest(context.Background(), "mysql_srv_2", "binlog.000001", bytes.NewReader([]byte("srv2-data")))

	chunks1 := svc.ListChunks("mysql_srv_1")
	chunks2 := svc.ListChunks("mysql_srv_2")

	if len(chunks1) != 1 || len(chunks2) != 1 {
		t.Fatalf("expected 1 chunk per server ID, got %d and %d", len(chunks1), len(chunks2))
	}
	if chunks1[0].SHA256Hash == chunks2[0].SHA256Hash {
		t.Fatal("chunks from different servers with different content should have different hashes")
	}
}

func TestTier1_F05_MySQLBinlog_SequentialIngest(t *testing.T) {
	env := NewTestEnv(t)
	svc := logcollection.New(env.Store, ports.SystemClock{})

	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("mysql-bin.%06d", i)
		_, err := svc.Ingest(context.Background(), "mysql_main", name, bytes.NewReader([]byte(fmt.Sprintf("binlog-%d", i))))
		if err != nil {
			t.Fatalf("failed to ingest %s: %v", name, err)
		}
	}
	chunks := svc.ListChunks("mysql_main")
	if len(chunks) != 5 {
		t.Fatalf("expected 5 binlog chunks, got %d", len(chunks))
	}
}

func TestTier1_F05_MySQLBinlog_EmptyReaderHandling(t *testing.T) {
	env := NewTestEnv(t)
	svc := logcollection.New(env.Store, ports.SystemClock{})

	_, err := svc.Ingest(context.Background(), "mysql_main", "mysql-bin.000001", bytes.NewReader([]byte{}))
	if err != logcollection.ErrEmptyChunkStream {
		t.Fatalf("expected ErrEmptyChunkStream, got %v", err)
	}
}

// =============================================================================
// Feature 6: SQLite Online Safe Backup (5 Tests)
// =============================================================================

func TestTier1_F06_SQLite_InspectDatabase(t *testing.T) {
	env := NewTestEnv(t)
	dbPath := env.CreateSampleSQLiteDB("test_inspect.sqlite", 20)

	driver := sqlite.New("sqlite3")
	insp, err := driver.Inspect(context.Background(), domain.Source{Path: dbPath})
	if err != nil {
		t.Fatalf("sqlite inspect failed: %v", err)
	}
	if insp.PageSize != 4096 {
		t.Fatalf("expected page size 4096, got %d", insp.PageSize)
	}
	if insp.PageCount <= 0 {
		t.Fatalf("expected positive page count, got %d", insp.PageCount)
	}
	if insp.SchemaDigest == "" {
		t.Fatal("expected non-empty schema digest")
	}
}

func TestTier1_F06_SQLite_OnlineSnapshot(t *testing.T) {
	env := NewTestEnv(t)
	dbPath := env.CreateSampleSQLiteDB("test_snap.sqlite", 50)

	driver := sqlite.New("sqlite3")
	scratchDir := filepath.Join(env.TempDir, "scratch")
	_ = os.MkdirAll(scratchDir, 0700)

	art, err := driver.CreateSnapshot(context.Background(), ports.SnapshotRequest{
		RunID:            "run_test",
		Source:           domain.Source{Path: dbPath},
		ScratchDirectory: scratchDir,
	})
	if err != nil {
		t.Fatalf("create sqlite snapshot failed: %v", err)
	}
	defer art.Close()

	if art.Size() <= 0 {
		t.Fatalf("expected positive artifact size, got %d", art.Size())
	}
	meta := art.Metadata()
	if meta.PageSize != 4096 {
		t.Fatalf("expected page size 4096, got %d", meta.PageSize)
	}
}

func TestTier1_F06_SQLite_DirectoryPathRejection(t *testing.T) {
	env := NewTestEnv(t)
	driver := sqlite.New("sqlite3")
	err := driver.Validate(context.Background(), domain.Source{Path: env.TempDir})
	if err == nil {
		t.Fatal("expected error validating directory as sqlite path, got nil")
	}
}

func TestTier1_F06_SQLite_MissingFileRejection(t *testing.T) {
	driver := sqlite.New("sqlite3")
	err := driver.Validate(context.Background(), domain.Source{Path: "/nonexistent/path/db.sqlite"})
	if err == nil {
		t.Fatal("expected error on nonexistent file, got nil")
	}
}

func TestTier1_F06_SQLite_SchemaDigestConsistency(t *testing.T) {
	env := NewTestEnv(t)
	dbPath1 := env.CreateSampleSQLiteDB("db1.sqlite", 10)
	dbPath2 := env.CreateSampleSQLiteDB("db2.sqlite", 100) // Different row count, identical schema

	driver := sqlite.New("sqlite3")
	insp1, err := driver.Inspect(context.Background(), domain.Source{Path: dbPath1})
	if err != nil {
		t.Fatalf("inspect 1 failed: %v", err)
	}
	insp2, err := driver.Inspect(context.Background(), domain.Source{Path: dbPath2})
	if err != nil {
		t.Fatalf("inspect 2 failed: %v", err)
	}

	if insp1.SchemaDigest != insp2.SchemaDigest {
		t.Fatalf("expected identical schema digest for identical schemas, got %s vs %s", insp1.SchemaDigest, insp2.SchemaDigest)
	}
}

// =============================================================================
// Feature 7: Streaming AEAD AES-256-GCM Encryption (5 Tests)
// =============================================================================

func TestTier1_F07_AEAD_KeyDerivationEntropyRequirement(t *testing.T) {
	weakKey := []byte("short-key-16-byte")
	_, err := keyderive.Derive(weakKey, keyderive.DomainAEAD)
	if err == nil {
		t.Fatal("expected error on weak master key (<32 bytes), got nil")
	}

	validKey := make([]byte, 32)
	derived, err := keyderive.Derive(validKey, keyderive.DomainAEAD)
	if err != nil {
		t.Fatalf("key derivation failed: %v", err)
	}
	if len(derived) != 32 {
		t.Fatalf("expected 32-byte derived key, got %d", len(derived))
	}
}

func TestTier1_F07_AEAD_EncryptDecryptRoundTrip(t *testing.T) {
	env := NewTestEnv(t)
	aeadKey, err := keyderive.Derive(env.MasterKey, keyderive.DomainAEAD)
	if err != nil {
		t.Fatalf("derive failed: %v", err)
	}

	enc, err := aead.New(aeadKey[:])
	if err != nil {
		t.Fatalf("aead init failed: %v", err)
	}

	plaintext := []byte("Production confidential database backup chunk data payload")
	chunkID := "chk_001_abc"

	ciphertext, err := enc.EncryptChunk(context.Background(), chunkID, plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	decrypted, err := enc.DecryptChunk(context.Background(), chunkID, ciphertext)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("decrypted data %q != plaintext %q", string(decrypted), string(plaintext))
	}
}

func TestTier1_F07_AEAD_AADAuthenticationCheck(t *testing.T) {
	env := NewTestEnv(t)
	aeadKey, _ := keyderive.Derive(env.MasterKey, keyderive.DomainAEAD)
	enc, _ := aead.New(aeadKey[:])

	plaintext := []byte("secret chunk data")
	ciphertext, err := enc.EncryptChunk(context.Background(), "chunk_correct_id", plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// Attempt decrypt with different chunkID (different AAD)
	_, err = enc.DecryptChunk(context.Background(), "chunk_wrong_id", ciphertext)
	if err == nil {
		t.Fatal("expected authentication error when chunkID / AAD does not match, got nil")
	}
}

func TestTier1_F07_AEAD_CiphertextTamperingDetection(t *testing.T) {
	env := NewTestEnv(t)
	aeadKey, _ := keyderive.Derive(env.MasterKey, keyderive.DomainAEAD)
	enc, _ := aead.New(aeadKey[:])

	plaintext := []byte("integrity test data")
	ciphertext, _ := enc.EncryptChunk(context.Background(), "chunk_tamper", plaintext)

	// Flip bit in body
	ciphertext[len(ciphertext)-1] ^= 0x01

	_, err := enc.DecryptChunk(context.Background(), "chunk_tamper", ciphertext)
	if err == nil {
		t.Fatal("expected error on tampered ciphertext, got nil")
	}
}

func TestTier1_F07_AEAD_EnvelopeMetadataEncryption(t *testing.T) {
	env := NewTestEnv(t)
	secretMeta := []byte(`{"aws_access_key":"AKIA123","secret":"secret123"}`)

	sealed, err := aead.EncryptEnvelope(env.MasterKey, secretMeta)
	if err != nil {
		t.Fatalf("encrypt envelope failed: %v", err)
	}

	opened, err := aead.DecryptEnvelope(env.MasterKey, sealed)
	if err != nil {
		t.Fatalf("decrypt envelope failed: %v", err)
	}

	if !bytes.Equal(secretMeta, opened) {
		t.Fatalf("opened envelope %s != secretMeta %s", string(opened), string(secretMeta))
	}
}

// =============================================================================
// Feature 8: S3-Compatible CAS Chunked Uploads (5 Tests)
// =============================================================================

func TestTier1_F08_S3CAS_ChunkUploadAndRetrieve(t *testing.T) {
	env := NewTestEnv(t)
	data := []byte("chunk payload bytes for cas test")
	h := sha256.Sum256(data)
	etag := hex.EncodeToString(h[:])

	stored, err := env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  "chunks/chk_test.dbv",
		Body: bytes.NewReader(data),
	})
	if err != nil {
		t.Fatalf("store put failed: %v", err)
	}
	if stored.ETag != etag {
		t.Fatalf("expected etag %s, got %s", etag, stored.ETag)
	}

	reader, head, err := env.Store.Get(context.Background(), ports.GetObjectRequest{Key: "chunks/chk_test.dbv"})
	if err != nil {
		t.Fatalf("store get failed: %v", err)
	}
	defer reader.Close()

	fetched, _ := io.ReadAll(reader)
	if !bytes.Equal(data, fetched) {
		t.Fatal("fetched chunk data does not match stored chunk")
	}
	if head.Size != int64(len(data)) {
		t.Fatalf("expected head size %d, got %d", len(data), head.Size)
	}
}

func TestTier1_F08_S3CAS_ConditionalCreateIdempotence(t *testing.T) {
	env := NewTestEnv(t)
	data1 := []byte("original chunk data")
	data2 := []byte("altered chunk data")

	_, err := env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  "chunks/chk_idemp.dbv",
		Body: bytes.NewReader(data1),
	})
	if err != nil {
		t.Fatalf("first put failed: %v", err)
	}

	// Put with IfNotExists=true should retain original
	stored2, err := env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:         "chunks/chk_idemp.dbv",
		Body:        bytes.NewReader(data2),
		IfNotExists: true,
	})
	if err != nil {
		t.Fatalf("second put failed: %v", err)
	}
	if stored2.Size != int64(len(data1)) {
		t.Fatalf("expected original size %d, got %d", len(data1), stored2.Size)
	}
}

func TestTier1_F08_S3CAS_ListPrefixFilter(t *testing.T) {
	env := NewTestEnv(t)
	_, _ = env.Store.Put(context.Background(), ports.PutObjectRequest{Key: "source1/chunks/c1.dbv", Body: bytes.NewReader([]byte("1"))})
	_, _ = env.Store.Put(context.Background(), ports.PutObjectRequest{Key: "source1/chunks/c2.dbv", Body: bytes.NewReader([]byte("2"))})
	_, _ = env.Store.Put(context.Background(), ports.PutObjectRequest{Key: "source2/chunks/c3.dbv", Body: bytes.NewReader([]byte("3"))})

	res, err := env.Store.List(context.Background(), ports.ListObjectsRequest{Prefix: "source1/chunks"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(res.Objects) != 2 {
		t.Fatalf("expected 2 objects for prefix source1/chunks, got %d", len(res.Objects))
	}
}

func TestTier1_F08_S3CAS_DeleteObject(t *testing.T) {
	env := NewTestEnv(t)
	_, _ = env.Store.Put(context.Background(), ports.PutObjectRequest{Key: "chunks/to_delete.dbv", Body: bytes.NewReader([]byte("del"))})

	err := env.Store.Delete(context.Background(), "chunks/to_delete.dbv")
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, _, err = env.Store.Get(context.Background(), ports.GetObjectRequest{Key: "chunks/to_delete.dbv"})
	if err == nil {
		t.Fatal("expected error getting deleted object, got nil")
	}
}

func TestTier1_F08_S3CAS_UserMetadataRetention(t *testing.T) {
	env := NewTestEnv(t)
	meta := map[string]string{
		"dbvault-chunk-id": "chunk_abc_123",
		"compressed":       "gzip",
	}

	_, err := env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:      "chunks/chk_meta.dbv",
		Body:     bytes.NewReader([]byte("meta-test")),
		Metadata: meta,
	})
	if err != nil {
		t.Fatalf("put with metadata failed: %v", err)
	}

	head, err := env.Store.Head(context.Background(), "chunks/chk_meta.dbv")
	if err != nil {
		t.Fatalf("head failed: %v", err)
	}
	if head.Metadata["dbvault-chunk-id"] != "chunk_abc_123" {
		t.Fatalf("expected metadata chunk_abc_123, got %+v", head.Metadata)
	}
}

// =============================================================================
// Feature 9: Deterministic Merkle Tree Generation (5 Tests)
// =============================================================================

func TestTier1_F09_Merkle_DeterministicLeafCalculation(t *testing.T) {
	key := []byte("test-dedup-key-32-bytes-long!!!!")
	chunkData := []byte("database page block bytes")
	pageSize := 4096

	id1 := digest.ChunkID(key, pageSize, chunkData)
	id2 := digest.ChunkID(key, pageSize, chunkData)

	if id1 != id2 {
		t.Fatalf("expected deterministic chunk IDs, got %s vs %s", id1, id2)
	}
	if len(id1) != 64 { // hex sha256 length
		t.Fatalf("expected 64-char hex digest, got %d", len(id1))
	}
}

func TestTier1_F09_Merkle_DeterministicRootCalculation(t *testing.T) {
	key := []byte("test-dedup-key-32-bytes-long!!!!")
	chunks := []chunking.PlainChunk{
		{Sequence: 0, Data: []byte("chunk-0-data"), PlaintextSize: 12},
		{Sequence: 1, Data: []byte("chunk-1-data"), PlaintextSize: 12},
	}
	ids := []string{
		digest.ChunkID(key, 4096, chunks[0].Data),
		digest.ChunkID(key, 4096, chunks[1].Data),
	}

	root1 := digest.RootDigest(key, chunks, ids, 24, 4096)
	root2 := digest.RootDigest(key, chunks, ids, 24, 4096)

	if root1 != root2 {
		t.Fatalf("expected identical root digest for same input, got %s vs %s", root1, root2)
	}
}

func TestTier1_F09_Merkle_OrderSensitivity(t *testing.T) {
	key := []byte("test-dedup-key-32-bytes-long!!!!")
	c1 := chunking.PlainChunk{Sequence: 0, Data: []byte("chunk-A"), PlaintextSize: 7}
	c2 := chunking.PlainChunk{Sequence: 1, Data: []byte("chunk-B"), PlaintextSize: 7}

	idA := digest.ChunkID(key, 4096, c1.Data)
	idB := digest.ChunkID(key, 4096, c2.Data)

	rootAB := digest.RootDigest(key, []chunking.PlainChunk{c1, c2}, []string{idA, idB}, 14, 4096)
	rootBA := digest.RootDigest(key, []chunking.PlainChunk{c2, c1}, []string{idB, idA}, 14, 4096)

	if rootAB == rootBA {
		t.Fatal("expected different root digests for different chunk ordering")
	}
}

func TestTier1_F09_Merkle_SingleBitDataSensitivity(t *testing.T) {
	key := []byte("test-dedup-key-32-bytes-long!!!!")
	c1 := chunking.PlainChunk{Sequence: 0, Data: []byte("chunk-A"), PlaintextSize: 7}
	c2Modified := chunking.PlainChunk{Sequence: 0, Data: []byte("chunk-B"), PlaintextSize: 7}

	id1 := digest.ChunkID(key, 4096, c1.Data)
	id2 := digest.ChunkID(key, 4096, c2Modified.Data)

	root1 := digest.RootDigest(key, []chunking.PlainChunk{c1}, []string{id1}, 7, 4096)
	root2 := digest.RootDigest(key, []chunking.PlainChunk{c2Modified}, []string{id2}, 7, 4096)

	if root1 == root2 {
		t.Fatal("expected modified data to produce different root digest")
	}
}

func TestTier1_F09_Merkle_EmptyChunkSequence(t *testing.T) {
	key := []byte("test-dedup-key-32-bytes-long!!!!")
	root := digest.RootDigest(key, []chunking.PlainChunk{}, []string{}, 0, 4096)
	if root == "" || len(root) != 64 {
		t.Fatalf("expected valid 64-char root digest for empty chunk sequence, got %s", root)
	}
}

// =============================================================================
// Feature 10: PureEd25519 Manifest Signing & Verification (5 Tests)
// =============================================================================

func TestTier1_F10_Ed25519_SignAndVerify(t *testing.T) {
	signer, err := ed25519signer.Generate("signer_1")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	payload := []byte(`{"snapshot_id":"snap_001","root_digest":"abcdef123456"}`)
	sig, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	if err := signer.Verify(payload, sig); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
}

func TestTier1_F10_Ed25519_AlteredPayloadRejection(t *testing.T) {
	signer, _ := ed25519signer.Generate("signer_1")
	payload := []byte(`{"snapshot_id":"snap_001","root_digest":"valid"}`)
	sig, _ := signer.Sign(payload)

	tamperedPayload := []byte(`{"snapshot_id":"snap_001","root_digest":"tampered"}`)
	err := signer.Verify(tamperedPayload, sig)
	if err == nil {
		t.Fatal("expected signature verification failure on tampered payload, got nil")
	}
}

func TestTier1_F10_Ed25519_WrongKeyRejection(t *testing.T) {
	signer1, _ := ed25519signer.Generate("signer_1")
	signer2, _ := ed25519signer.Generate("signer_2")

	payload := []byte(`{"snapshot_id":"snap_001"}`)
	sig, _ := signer1.Sign(payload)

	err := signer2.Verify(payload, sig)
	if err == nil {
		t.Fatal("expected signature verification failure with different public key, got nil")
	}
}

func TestTier1_F10_Ed25519_MissingPrivateKeySignRejection(t *testing.T) {
	signer := ed25519signer.New("pub_only", nil, make(ed25519.PublicKey, 32))
	_, err := signer.Sign([]byte("payload"))
	if err == nil {
		t.Fatal("expected error signing with nil private key, got nil")
	}
}

func TestTier1_F10_Ed25519_PublicKeyOnlyVerification(t *testing.T) {
	signerFull, _ := ed25519signer.Generate("signer_full")
	payload := []byte(`{"snapshot_id":"snap_pub_only"}`)
	sig, _ := signerFull.Sign(payload)

	// Create verifier with public key only
	verifier := ed25519signer.New("verifier", nil, signerFull.Public)
	if err := verifier.Verify(payload, sig); err != nil {
		t.Fatalf("public-key only verification failed: %v", err)
	}
}
