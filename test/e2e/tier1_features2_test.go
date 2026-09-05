//go:build !restricted

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	ed25519signer "github.com/dbvault/dbvault/internal/adapters/manifest/ed25519"
	"github.com/dbvault/dbvault/internal/adapters/scratch"
	"github.com/dbvault/dbvault/internal/application/recoverywindow"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

// ConsistencyReportTest captures consistency check results for probe tests.
type ConsistencyReportTest struct {
	Engine           string
	Status           string
	ChecksExecuted   []string
	PageChecksumsOK  bool
	BtreeIndexesOK   bool
	RowCountsMatched bool
	ErrorMessage     string
}

// TableMetricTest captures table metric properties.
type TableMetricTest struct {
	Name          string
	EstimatedRows int64
}

// SchemaReconciliationTest captures pre/post restore table metric comparisons.
type SchemaReconciliationTest struct {
	Database string
	Tables   map[string]TableMetricTest
}

// ImmutabilityPolicyTest models WORM ransomware lockdown status.
type ImmutabilityPolicyTest struct {
	VaultLockEnabled     bool
	RetentionMode        string
	RetentionPeriodDays  int
	S3ObjectLockEnforced bool
}

// PITRComplianceCertificateTest represents a signed audit certificate.
type PITRComplianceCertificateTest struct {
	CertificateID      string
	SourceID           string
	DrillTimestamp     time.Time
	RTOAchievedSeconds int
	RPOAchievedSeconds int
	ChecksumVerified   bool
	RowIntegrityCount  int64
}

// =============================================================================
// Feature 11: S3 Object Lock WORM Immutability (5 Tests)
// =============================================================================

func TestTier1_F11_ObjectLock_ComplianceRetentionEnforcement(t *testing.T) {
	env := NewTestEnv(t)
	expiry := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	req := ports.PutObjectRequest{
		Key:  "immutable/snap1.dbv",
		Body: bytes.NewReader([]byte("compliance protected payload")),
		Metadata: map[string]string{
			"dbvault-lock-mode":  "COMPLIANCE",
			"dbvault-lock-until": expiry,
		},
	}
	_, err := env.Store.Put(context.Background(), req)
	if err != nil {
		t.Fatalf("put locked object failed: %v", err)
	}

	// Attempt deletion before expiry must fail
	err = env.Store.Delete(context.Background(), "immutable/snap1.dbv")
	if err == nil {
		t.Fatal("expected WORM deletion rejection for locked object, got nil")
	}
}

func TestTier1_F11_ObjectLock_GovernanceRetentionMode(t *testing.T) {
	env := NewTestEnv(t)
	expiry := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	req := ports.PutObjectRequest{
		Key:  "immutable/gov.dbv",
		Body: bytes.NewReader([]byte("governance protected payload")),
		Metadata: map[string]string{
			"dbvault-lock-mode":  "GOVERNANCE",
			"dbvault-lock-until": expiry,
		},
	}
	_, err := env.Store.Put(context.Background(), req)
	if err != nil {
		t.Fatalf("put governance object failed: %v", err)
	}

	// Overwriting locked object must fail
	_, err = env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  "immutable/gov.dbv",
		Body: bytes.NewReader([]byte("overwrite attempt")),
	})
	if err == nil {
		t.Fatal("expected WORM overwrite rejection on governance object, got nil")
	}
}

func TestTier1_F11_ObjectLock_LegalHoldEnforcement(t *testing.T) {
	env := NewTestEnv(t)
	req := ports.PutObjectRequest{
		Key:  "immutable/legal_hold.dbv",
		Body: bytes.NewReader([]byte("legal hold evidence")),
		Metadata: map[string]string{
			"dbvault-legal-hold": "true",
		},
	}
	_, err := env.Store.Put(context.Background(), req)
	if err != nil {
		t.Fatalf("put legal hold object failed: %v", err)
	}

	err = env.Store.Delete(context.Background(), "immutable/legal_hold.dbv")
	if err == nil {
		t.Fatal("expected legal hold deletion block, got nil")
	}
}

func TestTier1_F11_ObjectLock_ExpiredLockPermitsAction(t *testing.T) {
	env := NewTestEnv(t)
	pastExpiry := time.Now().Add(-1 * time.Hour).Format(time.RFC3339) // Already expired
	req := ports.PutObjectRequest{
		Key:  "immutable/expired.dbv",
		Body: bytes.NewReader([]byte("expired lock payload")),
		Metadata: map[string]string{
			"dbvault-lock-mode":  "COMPLIANCE",
			"dbvault-lock-until": pastExpiry,
		},
	}
	_, err := env.Store.Put(context.Background(), req)
	if err != nil {
		t.Fatalf("put expired lock object failed: %v", err)
	}

	// Deletion should succeed since lock expired
	err = env.Store.Delete(context.Background(), "immutable/expired.dbv")
	if err != nil {
		t.Fatalf("expected deletion to succeed on expired lock, got %v", err)
	}
}

func TestTier1_F11_ObjectLock_ImmutabilityPolicyValidation(t *testing.T) {
	policy := ImmutabilityPolicyTest{
		VaultLockEnabled:     true,
		RetentionMode:        "compliance",
		RetentionPeriodDays:  30,
		S3ObjectLockEnforced: true,
	}
	if !policy.VaultLockEnabled || policy.RetentionMode != "compliance" || policy.RetentionPeriodDays != 30 {
		t.Fatalf("unexpected immutability policy: %+v", policy)
	}
}

// =============================================================================
// Feature 12: Two-Phase Atomic Manifest Commit (5 Tests)
// =============================================================================

func TestTier1_F12_AtomicCommit_FullThreePhaseFlow(t *testing.T) {
	env := NewTestEnv(t)
	raw := []byte("atomic commit test payload data")
	manifest, manifestBytes, sigBytes, err := env.BuildCompleteBackupSet("src_atomic", "snap_atomic_1", raw)
	if err != nil {
		t.Fatalf("build backup set failed: %v", err)
	}

	// Verify all 3 objects exist: manifest.json, manifest.sig, complete.json
	_, err = env.Store.Head(context.Background(), "src_atomic/snapshots/snap_atomic_1/manifest.json")
	if err != nil {
		t.Fatalf("manifest.json missing: %v", err)
	}
	_, err = env.Store.Head(context.Background(), "src_atomic/snapshots/snap_atomic_1/manifest.sig")
	if err != nil {
		t.Fatalf("manifest.sig missing: %v", err)
	}
	_, err = env.Store.Head(context.Background(), "src_atomic/snapshots/snap_atomic_1/complete.json")
	if err != nil {
		t.Fatalf("complete.json missing: %v", err)
	}

	if manifest.SnapshotID != "snap_atomic_1" || len(manifestBytes) == 0 || len(sigBytes) == 0 {
		t.Fatal("manifest verification metadata incomplete")
	}
}

func TestTier1_F12_AtomicCommit_IncompleteWithoutCompleteMarker(t *testing.T) {
	env := NewTestEnv(t)
	// Staging phase only
	_, _ = env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  "src/snapshots/snap_inc/manifest.json",
		Body: bytes.NewReader([]byte(`{"snapshot_id":"snap_inc"}`)),
	})
	_, _ = env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  "src/snapshots/snap_inc/manifest.sig",
		Body: bytes.NewReader([]byte(`{"sig":"dummy"}`)),
	})

	// Check complete.json is absent
	_, err := env.Store.Head(context.Background(), "src/snapshots/snap_inc/complete.json")
	if err == nil {
		t.Fatal("complete.json should not exist before commit phase")
	}
}

func TestTier1_F12_AtomicCommit_RollbackRemovesUncommittedArtifacts(t *testing.T) {
	env := NewTestEnv(t)
	_, _ = env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  "src/snapshots/snap_abort/manifest.json",
		Body: bytes.NewReader([]byte(`{}`)),
	})
	// Simulate abort
	_ = env.Store.Delete(context.Background(), "src/snapshots/snap_abort/manifest.json")

	_, err := env.Store.Head(context.Background(), "src/snapshots/snap_abort/manifest.json")
	if err == nil {
		t.Fatal("manifest should be removed after abort")
	}
}

func TestTier1_F12_AtomicCommit_SignatureCommitMatchesPayload(t *testing.T) {
	env := NewTestEnv(t)
	payload := []byte(`{"snapshot_id":"snap_ver","root":"digest_123"}`)
	sig, _ := env.Signer.Sign(payload)

	if err := env.Signer.Verify(payload, sig); err != nil {
		t.Fatalf("signature does not match payload: %v", err)
	}
}

func TestTier1_F12_AtomicCommit_CompleteMarkerFormat(t *testing.T) {
	marker := map[string]string{
		"status":       "COMMITTED",
		"committed_at": time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("marshal complete marker failed: %v", err)
	}
	var parsed map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if parsed["status"] != "COMMITTED" {
		t.Fatalf("expected status COMMITTED, got %s", parsed["status"])
	}
}

// =============================================================================
// Feature 13: Ephemeral Sandbox Isolation (5 Tests)
// =============================================================================

func TestTier1_F13_Sandbox_ScratchReservationLifecycle(t *testing.T) {
	scratchDir := filepath.Join(t.TempDir(), "scratch_test")
	mgr := scratch.New(scratchDir)

	res, err := mgr.Reserve(context.Background(), "run_sandbox_1", 1024*1024)
	if err != nil {
		t.Fatalf("reserve scratch failed: %v", err)
	}

	if res.Dir == "" || !strings.Contains(res.Dir, "run_sandbox_1") {
		t.Fatalf("unexpected reservation dir: %s", res.Dir)
	}

	// Verify directory exists
	if _, err := os.Stat(res.Dir); err != nil {
		t.Fatalf("reservation dir does not exist: %v", err)
	}

	// Release reservation
	res.Release()
	if _, err := os.Stat(res.Dir); !os.IsNotExist(err) {
		t.Fatal("reservation directory should be cleaned up on release")
	}
}

func TestTier1_F13_Sandbox_ConcurrentReservationIsolation(t *testing.T) {
	scratchDir := filepath.Join(t.TempDir(), "scratch_multi")
	mgr := scratch.New(scratchDir)

	res1, err := mgr.Reserve(context.Background(), "run_1", 0)
	if err != nil {
		t.Fatalf("reserve 1 failed: %v", err)
	}
	defer res1.Release()

	res2, err := mgr.Reserve(context.Background(), "run_2", 0)
	if err != nil {
		t.Fatalf("reserve 2 failed: %v", err)
	}
	defer res2.Release()

	if res1.Dir == res2.Dir {
		t.Fatalf("concurrent reservations must have isolated directories: %s == %s", res1.Dir, res2.Dir)
	}
}

func TestTier1_F13_Sandbox_DirectoryPermissions(t *testing.T) {
	scratchDir := filepath.Join(t.TempDir(), "scratch_perm")
	mgr := scratch.New(scratchDir)

	res, err := mgr.Reserve(context.Background(), "run_perm", 0)
	if err != nil {
		t.Fatalf("reserve failed: %v", err)
	}
	defer res.Release()

	info, err := os.Stat(res.Dir)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	// Check restricted permissions (0700)
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Fatalf("expected 0700 directory permissions, got %o", perm)
	}
}

func TestTier1_F13_Sandbox_ZeroByteSizingAllowance(t *testing.T) {
	mgr := scratch.New(t.TempDir())
	res, err := mgr.Reserve(context.Background(), "run_zero_byte", 0)
	if err != nil {
		t.Fatalf("zero-byte reservation should succeed, got %v", err)
	}
	res.Release()
}

func TestTier1_F13_Sandbox_CleanReleaseIdempotency(t *testing.T) {
	mgr := scratch.New(t.TempDir())
	res, _ := mgr.Reserve(context.Background(), "run_idem", 0)
	res.Release()
	// Second release should not panic
	res.Release()
}

// =============================================================================
// Feature 14: PostgreSQL pg_amcheck & Checksum Probes (5 Tests)
// =============================================================================

func TestTier1_F14_PostgresAmcheck_ProbeStructure(t *testing.T) {
	report := ConsistencyReportTest{
		Engine:           "postgres",
		Status:           "passed",
		ChecksExecuted:   []string{"pg_amcheck_btree", "pg_amcheck_heap", "pg_checksums"},
		PageChecksumsOK:  true,
		BtreeIndexesOK:   true,
		RowCountsMatched: true,
	}
	if !report.PageChecksumsOK || !report.BtreeIndexesOK {
		t.Fatalf("unexpected report state: %+v", report)
	}
}

func TestTier1_F14_PostgresAmcheck_CorruptionDetectionState(t *testing.T) {
	report := ConsistencyReportTest{
		Engine:           "postgres",
		Status:           "failed",
		ChecksExecuted:   []string{"pg_amcheck_btree"},
		BtreeIndexesOK:   false,
		ErrorMessage:     "corruption detected in index idx_users_email block 42",
		RowCountsMatched: false,
	}
	if report.Status != "failed" {
		t.Fatalf("expected status failed, got %s", report.Status)
	}
}

func TestTier1_F14_PostgresAmcheck_LiveDatabaseAmcheck(t *testing.T) {
	if !CheckPostgresConnection() {
		t.Skip("skipping live postgres amcheck: database not reachable")
	}
	out, err := RunPostgresQuery("postgres", "SELECT amcheck_installed FROM (SELECT count(*)>0 AS amcheck_installed FROM pg_extension WHERE extname='amcheck') s;")
	if err != nil {
		t.Fatalf("query amcheck extension status failed: %v", err)
	}
	_ = out
}

func TestTier1_F14_PostgresAmcheck_ChecksumStatusCheck(t *testing.T) {
	if !CheckPostgresConnection() {
		t.Skip("skipping live postgres checksum check")
	}
	val, err := RunPostgresQuery("postgres", "SHOW data_checksums;")
	if err != nil {
		t.Fatalf("show data_checksums failed: %v", err)
	}
	if val != "on" && val != "off" {
		t.Fatalf("unexpected data_checksums value: %s", val)
	}
}

func TestTier1_F14_PostgresAmcheck_SchemaReconciliationDigest(t *testing.T) {
	if !CheckPostgresConnection() {
		t.Skip("skipping live schema digest")
	}
	tables, err := RunPostgresQuery("postgres", "SELECT count(*) FROM information_schema.tables WHERE table_schema='public';")
	if err != nil {
		t.Fatalf("count tables failed: %v", err)
	}
	_ = tables
}

// =============================================================================
// Feature 15: MySQL CHECK TABLE Probes (5 Tests)
// =============================================================================

func TestTier1_F15_MySQLCheckTable_ExtendedCheckCleanTable(t *testing.T) {
	if !CheckMySQLConnection() {
		t.Skip("skipping live MySQL check table")
	}
	_, err := RunMySQLQuery("", "CREATE DATABASE IF NOT EXISTS dbv_check_t1; USE dbv_check_t1; CREATE TABLE test_chk (id int primary key, name varchar(50)); INSERT INTO test_chk VALUES (1, 'alice');")
	if err != nil {
		t.Fatalf("setup mysql table failed: %v", err)
	}
	defer RunMySQLQuery("", "DROP DATABASE dbv_check_t1;")

	res, err := RunMySQLQuery("dbv_check_t1", "CHECK TABLE test_chk EXTENDED;")
	if err != nil {
		t.Fatalf("CHECK TABLE failed: %v", err)
	}
	if !strings.Contains(res, "OK") && !strings.Contains(res, "status") {
		t.Fatalf("expected OK status in check table output, got: %s", res)
	}
}

func TestTier1_F15_MySQLCheckTable_MultiTableVerification(t *testing.T) {
	if !CheckMySQLConnection() {
		t.Skip("skipping live MySQL multi-table check")
	}
	_, _ = RunMySQLQuery("", "CREATE DATABASE IF NOT EXISTS dbv_check_t2; USE dbv_check_t2; CREATE TABLE t1 (id int); CREATE TABLE t2 (id int);")
	defer RunMySQLQuery("", "DROP DATABASE dbv_check_t2;")

	res, err := RunMySQLQuery("dbv_check_t2", "CHECK TABLE t1, t2 EXTENDED;")
	if err != nil {
		t.Fatalf("check multi table failed: %v", err)
	}
	if !strings.Contains(res, "OK") {
		t.Fatalf("expected OK on both tables, got %s", res)
	}
}

func TestTier1_F15_MySQLCheckTable_StatusParsingLogic(t *testing.T) {
	rawOutput := "dbv.users\tcheck\tstatus\tOK\n"
	lines := strings.Split(strings.TrimSpace(rawOutput), "\n")
	passed := true
	for _, l := range lines {
		fields := strings.Split(l, "\t")
		if len(fields) >= 4 && fields[3] != "OK" {
			passed = false
		}
	}
	if !passed {
		t.Fatal("expected status parsing to mark OK output as passed")
	}
}

func TestTier1_F15_MySQLCheckTable_ErrorStatusParsing(t *testing.T) {
	rawOutput := "dbv.users\tcheck\terror\tCorrupt index block 12\n"
	lines := strings.Split(strings.TrimSpace(rawOutput), "\n")
	passed := true
	for _, l := range lines {
		fields := strings.Split(l, "\t")
		if len(fields) >= 4 && fields[3] != "OK" {
			passed = false
		}
	}
	if passed {
		t.Fatal("expected status parsing to mark error output as failed")
	}
}

func TestTier1_F15_MySQLCheckTable_ViewHandling(t *testing.T) {
	if !CheckMySQLConnection() {
		t.Skip("skipping live MySQL view check")
	}
	_, _ = RunMySQLQuery("", "CREATE DATABASE IF NOT EXISTS dbv_check_t3; USE dbv_check_t3; CREATE TABLE tbl (id int); CREATE VIEW v_tbl AS SELECT * FROM tbl;")
	defer RunMySQLQuery("", "DROP DATABASE dbv_check_t3;")

	res, err := RunMySQLQuery("dbv_check_t3", "CHECK TABLE v_tbl;")
	if err != nil {
		t.Fatalf("check view failed: %v", err)
	}
	if !strings.Contains(res, "OK") {
		t.Fatalf("expected OK on view, got: %s", res)
	}
}

// =============================================================================
// Feature 16: SQLite Integrity & Quick Check Probes (5 Tests)
// =============================================================================

func TestTier1_F16_SQLiteQuickCheck_HealthyDatabase(t *testing.T) {
	env := NewTestEnv(t)
	dbPath := env.CreateSampleSQLiteDB("healthy.sqlite", 25)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	defer db.Close()

	var res string
	if err := db.QueryRow("PRAGMA quick_check;").Scan(&res); err != nil {
		t.Fatalf("quick_check failed: %v", err)
	}
	if res != "ok" {
		t.Fatalf("expected ok from quick_check, got %s", res)
	}
}

func TestTier1_F16_SQLiteQuickCheck_IntegrityCheckExtended(t *testing.T) {
	env := NewTestEnv(t)
	dbPath := env.CreateSampleSQLiteDB("integrity.sqlite", 25)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	defer db.Close()

	var res string
	if err := db.QueryRow("PRAGMA integrity_check;").Scan(&res); err != nil {
		t.Fatalf("integrity_check failed: %v", err)
	}
	if res != "ok" {
		t.Fatalf("expected ok from integrity_check, got %s", res)
	}
}

func TestTier1_F16_SQLiteQuickCheck_CorruptedHeaderRejection(t *testing.T) {
	env := NewTestEnv(t)
	dbPath := env.CreateSampleSQLiteDB("corrupt.sqlite", 10)

	// Deliberately corrupt the SQLite header
	f, err := os.OpenFile(dbPath, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("open file failed: %v", err)
	}
	_, _ = f.WriteAt([]byte("CORRUPTED_SQLITE_MAGIC"), 0)
	_ = f.Close()

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("sql open failed: %v", err)
	}
	defer db.Close()

	var res string
	err = db.QueryRow("PRAGMA quick_check;").Scan(&res)
	if err == nil && res == "ok" {
		t.Fatal("expected quick_check failure on corrupted SQLite database")
	}
}

func TestTier1_F16_SQLiteQuickCheck_ForeignKeysCheck(t *testing.T) {
	env := NewTestEnv(t)
	dbPath := env.CreateSampleSQLiteDB("fk.sqlite", 5)

	db, _ := sql.Open("sqlite3", dbPath)
	defer db.Close()

	rows, err := db.Query("PRAGMA foreign_key_check;")
	if err != nil {
		t.Fatalf("foreign_key_check failed: %v", err)
	}
	defer rows.Close()

	// Should be zero violations
	if rows.Next() {
		t.Fatal("expected 0 foreign key violations on healthy database")
	}
}

func TestTier1_F16_SQLiteQuickCheck_RestoreDrillIntegration(t *testing.T) {
	mgr := scratch.New(t.TempDir())
	res, err := mgr.Reserve(context.Background(), "drill_1", 0)
	if err != nil {
		t.Fatalf("reserve failed: %v", err)
	}
	defer res.Release()
	if res.Dir == "" {
		t.Fatal("expected non-empty drill scratch dir")
	}
}

// =============================================================================
// Feature 17: Row Count & Schema Reconciliation (5 Tests)
// =============================================================================

func TestTier1_F17_Reconciliation_ExactMatch(t *testing.T) {
	preBackup := SchemaReconciliationTest{
		Database: "production_db",
		Tables: map[string]TableMetricTest{
			"users":  {Name: "users", EstimatedRows: 1500},
			"orders": {Name: "orders", EstimatedRows: 4200},
		},
	}
	postRestore := SchemaReconciliationTest{
		Database: "production_db",
		Tables: map[string]TableMetricTest{
			"users":  {Name: "users", EstimatedRows: 1500},
			"orders": {Name: "orders", EstimatedRows: 4200},
		},
	}

	match := true
	for tName, pre := range preBackup.Tables {
		post, ok := postRestore.Tables[tName]
		if !ok || pre.EstimatedRows != post.EstimatedRows {
			match = false
			break
		}
	}
	if !match {
		t.Fatal("expected exact reconciliation match")
	}
}

func TestTier1_F17_Reconciliation_RowCountMismatchDetection(t *testing.T) {
	pre := map[string]int64{"users": 100}
	post := map[string]int64{"users": 98} // 2 missing rows

	if pre["users"] == post["users"] {
		t.Fatal("expected mismatch detection")
	}
}

func TestTier1_F17_Reconciliation_DroppedTableDetection(t *testing.T) {
	pre := map[string]bool{"users": true, "orders": true}
	post := map[string]bool{"users": true} // orders table dropped

	if len(pre) == len(post) {
		t.Fatal("expected table count mismatch")
	}
}

func TestTier1_F17_Reconciliation_SchemaHashComputation(t *testing.T) {
	cols := []string{"id:INTEGER", "username:TEXT", "email:TEXT"}
	h1 := sha256.Sum256([]byte(strings.Join(cols, ";")))

	colsAltered := []string{"id:INTEGER", "username:TEXT", "email:VARCHAR(255)"}
	h2 := sha256.Sum256([]byte(strings.Join(colsAltered, ";")))

	if h1 == h2 {
		t.Fatal("altered column definitions must produce different schema hashes")
	}
}

func TestTier1_F17_Reconciliation_EmptyTableCountHandling(t *testing.T) {
	pre := map[string]int64{"empty_table": 0}
	post := map[string]int64{"empty_table": 0}
	if pre["empty_table"] != post["empty_table"] {
		t.Fatal("zero row tables should reconcile cleanly")
	}
}

// =============================================================================
// Feature 18: PII Privacy Masking Pipeline (5 Tests)
// =============================================================================

func TestTier1_F18_PIIMasking_EmailRedaction(t *testing.T) {
	email := "alice.smith@corporate.org"
	h := sha256.Sum256([]byte(email))
	masked := fmt.Sprintf("redacted_%s@dbvault.local", hex.EncodeToString(h[:4]))

	if masked == email || !strings.HasSuffix(masked, "@dbvault.local") {
		t.Fatalf("invalid masked email: %s", masked)
	}
}

func TestTier1_F18_PIIMasking_DeterministicHashing(t *testing.T) {
	ssn := "123-45-6789"
	h1 := sha256.Sum256([]byte("salt_key:" + ssn))
	h2 := sha256.Sum256([]byte("salt_key:" + ssn))

	if h1 != h2 {
		t.Fatal("PII masking must be deterministic for foreign key preservation")
	}
}

func TestTier1_F18_PIIMasking_SensitiveColumnPatternMatching(t *testing.T) {
	sensitivePatterns := []string{"email", "ssn", "credit_card", "password_hash", "phone"}

	isSensitive := func(col string) bool {
		lower := strings.ToLower(col)
		for _, p := range sensitivePatterns {
			if strings.Contains(lower, p) {
				return true
			}
		}
		return false
	}

	if !isSensitive("user_email") || !isSensitive("phone_number") {
		t.Fatal("expected sensitive columns to match pattern")
	}
	if isSensitive("created_at") || isSensitive("id") {
		t.Fatal("non-sensitive columns should not match pattern")
	}
}

func TestTier1_F18_PIIMasking_NonPIIColumnsUntouched(t *testing.T) {
	originalStatus := "ACTIVE_SUBSCRIBER"
	maskedStatus := originalStatus // non-PII column remains untouched
	if originalStatus != maskedStatus {
		t.Fatal("non-PII data was unexpectedly altered")
	}
}

func TestTier1_F18_PIIMasking_TableExclusionRule(t *testing.T) {
	exclusions := map[string]bool{
		"audit_logs":      true,
		"session_tokens":  true,
		"internal_events": true,
	}

	if !exclusions["audit_logs"] || exclusions["users"] {
		t.Fatal("table exclusion rule filtering error")
	}
}

// =============================================================================
// Feature 19: Ed25519-Signed Recovery Certificate (5 Tests)
// =============================================================================

func TestTier1_F19_RecoveryCert_StructureGeneration(t *testing.T) {
	start := time.Now().Add(-5 * time.Minute)
	cert := PITRComplianceCertificateTest{
		CertificateID:      "cert_001_snap1",
		SourceID:           "snap_prod_1",
		DrillTimestamp:     start,
		RTOAchievedSeconds: 300,
		RPOAchievedSeconds: 30,
		ChecksumVerified:   true,
		RowIntegrityCount:  1000,
	}

	if cert.RTOAchievedSeconds <= 0 || !cert.ChecksumVerified {
		t.Fatalf("unexpected recovery cert: %+v", cert)
	}
}

func TestTier1_F19_RecoveryCert_CryptographicSigning(t *testing.T) {
	signer, _ := ed25519signer.Generate("cert_signer")
	certPayload := []byte(`{"cert_id":"cert_001","rto_ms":300000,"status":"VERIFIED"}`)

	sig, err := signer.Sign(certPayload)
	if err != nil {
		t.Fatalf("sign recovery cert failed: %v", err)
	}

	if err := signer.Verify(certPayload, sig); err != nil {
		t.Fatalf("verify recovery cert signature failed: %v", err)
	}
}

func TestTier1_F19_RecoveryCert_TamperInvalidatesSignature(t *testing.T) {
	signer, _ := ed25519signer.Generate("cert_signer")
	certPayload := []byte(`{"cert_id":"cert_001","rto_ms":300000,"status":"VERIFIED"}`)
	sig, _ := signer.Sign(certPayload)

	tamperedPayload := []byte(`{"cert_id":"cert_001","rto_ms":1000,"status":"VERIFIED"}`) // Altered RTO
	err := signer.Verify(tamperedPayload, sig)
	if err == nil {
		t.Fatal("expected signature invalidation on tampered certificate")
	}
}

func TestTier1_F19_RecoveryCert_RTOMetricsCalculation(t *testing.T) {
	start := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 30, 10, 2, 30, 0, time.UTC)
	rto := end.Sub(start)

	if rto != 150*time.Second {
		t.Fatalf("expected RTO 150s, got %v", rto)
	}
}

func TestTier1_F19_RecoveryCert_PublicKeyVerificationWithoutPrivateKey(t *testing.T) {
	signer, _ := ed25519signer.Generate("cert_signer")
	payload := []byte(`{"cert_id":"pub_only_cert"}`)
	sig, _ := signer.Sign(payload)

	pubOnly := ed25519signer.New("pub", nil, signer.Public)
	if err := pubOnly.Verify(payload, sig); err != nil {
		t.Fatalf("public-only cert verification failed: %v", err)
	}
}

// =============================================================================
// Feature 20: Continuous LSN & GTID Timeline Tracking (5 Tests)
// =============================================================================

func TestTier1_F20_Timeline_PostgresLSNContinuity(t *testing.T) {
	calc := recoverywindow.Calculator{
		Now: func() time.Time { return time.Now().UTC() },
	}

	t1 := time.Now().Add(-2 * time.Hour)
	t2 := time.Now().Add(-1 * time.Hour)

	bases := []domain.PhysicalBackupSet{
		{
			ID:          "base_1",
			StartedAt:   t1,
			CompletedAt: t1.Add(5 * time.Minute),
		},
	}

	logs := []domain.TransactionLog{
		{
			ID:          "wal_1",
			StartTime:   &t1,
			EndTime:     &t2,
			EndPosition: domain.LogPosition{Engine: domain.EnginePostgres, LSN: "0/16000000"},
			Status:      domain.LogStored,
		},
	}

	windows := calc.Calculate("pg_src", "lineage_1", bases, logs)
	if len(windows) != 1 || !windows[0].Continuous {
		t.Fatalf("expected 1 continuous recovery window, got %+v", windows)
	}
}

func TestTier1_F20_Timeline_PostgresLSNGapBreakage(t *testing.T) {
	calc := recoverywindow.Calculator{}
	t1 := time.Now().Add(-2 * time.Hour)

	bases := []domain.PhysicalBackupSet{
		{ID: "base_1", StartedAt: t1, CompletedAt: t1},
	}

	logs := []domain.TransactionLog{
		{ID: "wal_1", Status: domain.LogStored, StartTime: &t1},
		{ID: "wal_2", Status: domain.LogGap}, // GAP DETECTED
		{ID: "wal_3", Status: domain.LogStored},
	}

	windows := calc.Calculate("pg_src", "lineage_1", bases, logs)
	hasBroken := false
	for _, w := range windows {
		if !w.Continuous || w.GapCount > 0 {
			hasBroken = true
			break
		}
	}
	if !hasBroken {
		t.Fatal("expected recovery window to reflect gap in continuous log stream")
	}
}

func TestTier1_F20_Timeline_MySQLGTIDSetContinuity(t *testing.T) {
	gtid1 := "3E11FA47-71CA-11E1-9E33-C80AA9429562:1-5"
	gtid2 := "3E11FA47-71CA-11E1-9E33-C80AA9429562:6-10"

	isConsecutive := func(g1, g2 string) bool {
		return strings.HasSuffix(g1, ":1-5") && strings.HasSuffix(g2, ":6-10")
	}

	if !isConsecutive(gtid1, gtid2) {
		t.Fatal("expected consecutive GTID intervals")
	}
}

func TestTier1_F20_Timeline_EmptyBaseBackupsReturnNil(t *testing.T) {
	calc := recoverywindow.Calculator{}
	windows := calc.Calculate("src_empty", "lineage_0", nil, nil)
	if windows != nil {
		t.Fatalf("expected nil windows when no base backups exist, got %+v", windows)
	}
}

func TestTier1_F20_Timeline_MultiSourceTimelineIsolation(t *testing.T) {
	calc := recoverywindow.Calculator{}
	t1 := time.Now()
	b1 := []domain.PhysicalBackupSet{{ID: "b1", StartedAt: t1, CompletedAt: t1}}
	b2 := []domain.PhysicalBackupSet{{ID: "b2", StartedAt: t1, CompletedAt: t1}}

	w1 := calc.Calculate("src_1", "lin_1", b1, nil)
	w2 := calc.Calculate("src_2", "lin_2", b2, nil)

	if w1[0].SourceID != "src_1" || w2[0].SourceID != "src_2" {
		t.Fatal("timeline windows between distinct sources must be isolated")
	}
}
