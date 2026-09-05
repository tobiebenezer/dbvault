//go:build !restricted

package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/dbvault/dbvault/internal/adapters/encryption/aead"
	ed25519signer "github.com/dbvault/dbvault/internal/adapters/manifest/ed25519"
	"github.com/dbvault/dbvault/internal/application/chunking"
	"github.com/dbvault/dbvault/internal/application/digest"
	mf "github.com/dbvault/dbvault/internal/application/manifest"
	"github.com/dbvault/dbvault/internal/platform/keyderive"
	"github.com/dbvault/dbvault/internal/ports"
)

// TestEnv contains reusable test resources and helpers for E2E tests.
type TestEnv struct {
	T         *testing.T
	TempDir   string
	MasterKey []byte
	Signer    ed25519signer.Signer
	Store     ports.ObjectStore
	Now       time.Time
}

// NewTestEnv creates a fresh, isolated test environment for an E2E test.
func NewTestEnv(t *testing.T) *TestEnv {
	dir := t.TempDir()
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatalf("failed to generate random test master key: %v", err)
	}

	signer, err := ed25519signer.Generate("test-key-1")
	if err != nil {
		t.Fatalf("failed to generate Ed25519 test signer: %v", err)
	}

	memStore := NewMockObjectStore()

	return &TestEnv{
		T:         t,
		TempDir:   dir,
		MasterKey: key,
		Signer:    signer,
		Store:     memStore,
		Now:       time.Date(2026, 8, 30, 19, 0, 0, 0, time.UTC),
	}
}

// CreateSampleSQLiteDB creates an SQLite database with sample tables and rows.
func (env *TestEnv) CreateSampleSQLiteDB(name string, rowCount int) string {
	dbPath := filepath.Join(env.TempDir, name)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		env.T.Fatalf("failed to create sqlite db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			username TEXT NOT NULL,
			email TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE orders (
			id INTEGER PRIMARY KEY,
			user_id INTEGER,
			amount DECIMAL(10, 2),
			FOREIGN KEY (user_id) REFERENCES users(id)
		);
	`)
	if err != nil {
		env.T.Fatalf("failed to execute sqlite schema: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		env.T.Fatalf("failed to begin tx: %v", err)
	}
	stmt, err := tx.Prepare("INSERT INTO users (id, username, email) VALUES (?, ?, ?)")
	if err != nil {
		env.T.Fatalf("failed to prepare stmt: %v", err)
	}
	defer stmt.Close()

	for i := 1; i <= rowCount; i++ {
		_, err := stmt.Exec(i, fmt.Sprintf("user_%d", i), fmt.Sprintf("user_%d@example.com", i))
		if err != nil {
			env.T.Fatalf("failed to insert user row: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		env.T.Fatalf("failed to commit user rows: %v", err)
	}

	return dbPath
}

// MockObjectStore is an in-memory object store with WORM immutability and metadata support.
type MockObjectStore struct {
	mu      sync.RWMutex
	objects map[string]storedItem
}

type storedItem struct {
	data         []byte
	etag         string
	lastModified time.Time
	metadata     map[string]string
	lockExpires  time.Time
	legalHold    bool
	mode         string // "COMPLIANCE" or "GOVERNANCE"
}

// NewMockObjectStore initializes a new mock store.
func NewMockObjectStore() *MockObjectStore {
	return &MockObjectStore{
		objects: make(map[string]storedItem),
	}
}

func (m *MockObjectStore) Name() string {
	return "mock-s3-store"
}

func (m *MockObjectStore) Validate(ctx context.Context) error {
	return nil
}

func (m *MockObjectStore) Capabilities(ctx context.Context) (ports.StorageCapabilities, error) {
	return ports.StorageCapabilities{
		ConditionalCreate: true,
		BatchDelete:       true,
		UserMetadata:      true,
		RangeReads:        true,
	}, nil
}

func (m *MockObjectStore) Put(ctx context.Context, req ports.PutObjectRequest) (ports.StoredObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanKey := strings.TrimPrefix(filepath.Clean("/"+req.Key), "/")
	if existing, exists := m.objects[cleanKey]; exists {
		if req.IfNotExists {
			return ports.StoredObject{
				Key:          cleanKey,
				Size:         int64(len(existing.data)),
				ETag:         existing.etag,
				LastModified: existing.lastModified,
				Metadata:     existing.metadata,
			}, nil
		}
		// Check WORM Object Lock
		if existing.legalHold || (!existing.lockExpires.IsZero() && time.Now().Before(existing.lockExpires)) {
			return ports.StoredObject{}, fmt.Errorf("WORM object lock violation: object %s is immutable until %s", cleanKey, existing.lockExpires)
		}
	}

	var data []byte
	if req.Body != nil {
		var err error
		data, err = io.ReadAll(req.Body)
		if err != nil {
			return ports.StoredObject{}, err
		}
	}

	h := sha256.Sum256(data)
	etag := hex.EncodeToString(h[:])
	now := time.Now().UTC()

	var lockExpiry time.Time
	mode := ""
	legalHold := false
	if req.Metadata != nil {
		if v, ok := req.Metadata["dbvault-lock-mode"]; ok {
			mode = v
		}
		if v, ok := req.Metadata["dbvault-lock-until"]; ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				lockExpiry = t
			}
		}
		if v, ok := req.Metadata["dbvault-legal-hold"]; ok && v == "true" {
			legalHold = true
		}
	}

	m.objects[cleanKey] = storedItem{
		data:         data,
		etag:         etag,
		lastModified: now,
		metadata:     req.Metadata,
		lockExpires:  lockExpiry,
		legalHold:    legalHold,
		mode:         mode,
	}

	return ports.StoredObject{
		Key:          cleanKey,
		Size:         int64(len(data)),
		ETag:         etag,
		LastModified: now,
		Metadata:     req.Metadata,
	}, nil
}

func (m *MockObjectStore) Get(ctx context.Context, req ports.GetObjectRequest) (ports.ObjectReader, ports.StoredObject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cleanKey := strings.TrimPrefix(filepath.Clean("/"+req.Key), "/")
	item, ok := m.objects[cleanKey]
	if !ok {
		return nil, ports.StoredObject{}, fmt.Errorf("object not found: %s", cleanKey)
	}

	return io.NopCloser(bytes.NewReader(item.data)), ports.StoredObject{
		Key:          cleanKey,
		Size:         int64(len(item.data)),
		ETag:         item.etag,
		LastModified: item.lastModified,
		Metadata:     item.metadata,
	}, nil
}

func (m *MockObjectStore) Head(ctx context.Context, key string) (ports.StoredObject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cleanKey := strings.TrimPrefix(filepath.Clean("/"+key), "/")
	item, ok := m.objects[cleanKey]
	if !ok {
		return ports.StoredObject{}, fmt.Errorf("object not found: %s", cleanKey)
	}

	return ports.StoredObject{
		Key:          cleanKey,
		Size:         int64(len(item.data)),
		ETag:         item.etag,
		LastModified: item.lastModified,
		Metadata:     item.metadata,
	}, nil
}

func (m *MockObjectStore) List(ctx context.Context, req ports.ListObjectsRequest) (ports.ListObjectsResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prefix := strings.TrimPrefix(filepath.Clean("/"+req.Prefix), "/")
	if prefix == "." {
		prefix = ""
	}

	var res []ports.StoredObject
	for k, v := range m.objects {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			res = append(res, ports.StoredObject{
				Key:          k,
				Size:         int64(len(v.data)),
				ETag:         v.etag,
				LastModified: v.lastModified,
				Metadata:     v.metadata,
			})
		}
	}

	return ports.ListObjectsResult{Objects: res}, nil
}

func (m *MockObjectStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanKey := strings.TrimPrefix(filepath.Clean("/"+key), "/")
	item, ok := m.objects[cleanKey]
	if !ok {
		return nil
	}
	if item.legalHold || (!item.lockExpires.IsZero() && time.Now().Before(item.lockExpires)) {
		return fmt.Errorf("WORM object lock violation: cannot delete locked object %s", cleanKey)
	}
	delete(m.objects, cleanKey)
	return nil
}

// MutateChunk flips bits in an object to simulate byte-level corruption.
func (m *MockObjectStore) MutateChunk(key string, byteOffset int, bitMask byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanKey := strings.TrimPrefix(filepath.Clean("/"+key), "/")
	item, ok := m.objects[cleanKey]
	if !ok {
		return fmt.Errorf("object not found to mutate: %s", cleanKey)
	}
	if byteOffset >= len(item.data) {
		byteOffset = len(item.data) - 1
	}
	if byteOffset < 0 {
		byteOffset = 0
	}
	corrupted := make([]byte, len(item.data))
	copy(corrupted, item.data)
	corrupted[byteOffset] ^= bitMask
	item.data = corrupted
	m.objects[cleanKey] = item
	return nil
}

// Helper functions for end-to-end tests

// CheckPostgresConnection checks if live PostgreSQL is reachable.
func CheckPostgresConnection() bool {
	cmd := exec.Command("psql", "-h", "127.0.0.1", "-p", "5432", "-U", "postgres", "-c", "SELECT 1;")
	cmd.Env = append(os.Environ(), "PGPASSWORD=Awodumila")
	return cmd.Run() == nil
}

// CheckMySQLConnection checks if live MySQL is reachable.
func CheckMySQLConnection() bool {
	cmd := exec.Command("mysql", "-h", "127.0.0.1", "-P", "3306", "-u", "root", "-e", "SELECT 1;")
	cmd.Env = append(os.Environ(), "MYSQL_PWD=Awodumila")
	return cmd.Run() == nil
}

// RunPostgresQuery runs a query against live PostgreSQL with standard password.
func RunPostgresQuery(dbName, query string) (string, error) {
	args := []string{"-h", "127.0.0.1", "-p", "5432", "-U", "postgres"}
	if dbName != "" {
		args = append(args, "-d", dbName)
	}
	args = append(args, "-t", "-A", "-c", query)
	cmd := exec.Command("psql", args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD=Awodumila")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// RunMySQLQuery runs a query against live MySQL with standard password.
func RunMySQLQuery(dbName, query string) (string, error) {
	args := []string{"-h", "127.0.0.1", "-P", "3306", "-u", "root"}
	if dbName != "" {
		args = append(args, "-D", dbName)
	}
	args = append(args, "-s", "-N", "-e", query)
	cmd := exec.Command("mysql", args...)
	cmd.Env = append(os.Environ(), "MYSQL_PWD=Awodumila")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// BuildCompleteBackupSet executes chunking, encryption, Merkle computation, Ed25519 signing, and store upload.
func (env *TestEnv) BuildCompleteBackupSet(sourceID, snapID string, rawData []byte) (*mf.SnapshotManifest, []byte, []byte, error) {
	chunkSize := 1024 * 1024
	if len(rawData) < chunkSize {
		chunkSize = len(rawData)
	}
	if chunkSize == 0 {
		chunkSize = 1
	}

	plainChunks := []chunking.PlainChunk{}
	for i := 0; i < len(rawData); i += chunkSize {
		end := i + chunkSize
		if end > len(rawData) {
			end = len(rawData)
		}
		data := rawData[i:end]
		plainChunks = append(plainChunks, chunking.PlainChunk{
			Sequence:      len(plainChunks),
			PageStart:     int64(i / 4096),
			PageCount:     len(data) / 4096,
			PlaintextSize: int64(len(data)),
			Data:          data,
		})
	}

	dedupKey, err := keyderive.Derive(env.MasterKey, keyderive.DomainDedup)
	if err != nil {
		return nil, nil, nil, err
	}
	aeadKey, err := keyderive.Derive(env.MasterKey, keyderive.DomainAEAD)
	if err != nil {
		return nil, nil, nil, err
	}

	enc, err := aead.New(aeadKey[:])
	if err != nil {
		return nil, nil, nil, err
	}

	chunkIDs := make([]string, len(plainChunks))
	manifestChunks := make([]mf.ChunkInfo, len(plainChunks))

	for i, ch := range plainChunks {
		cid := digest.ChunkID(dedupKey[:], 4096, ch.Data)
		chunkIDs[i] = cid

		ciphertext, err := enc.EncryptChunk(context.Background(), cid, ch.Data)
		if err != nil {
			return nil, nil, nil, err
		}

		objKey := fmt.Sprintf("%s/chunks/%s.dbv", sourceID, cid)
		_, err = env.Store.Put(context.Background(), ports.PutObjectRequest{
			Key:  objKey,
			Body: bytes.NewReader(ciphertext),
		})
		if err != nil {
			return nil, nil, nil, err
		}

		manifestChunks[i] = mf.ChunkInfo{
			Sequence:      i,
			ChunkID:       cid,
			ObjectKey:     objKey,
			PageStart:     ch.PageStart,
			PageCount:     ch.PageCount,
			PlaintextSize: ch.PlaintextSize,
			StoredSize:    int64(len(ciphertext)),
			Compression:   "none",
			KeyID:         "local-key",
		}
	}

	rootDigest := digest.RootDigest(dedupKey[:], plainChunks, chunkIDs, int64(len(rawData)), 4096)

	manifest := &mf.SnapshotManifest{
		Format:        "dbvault-manifest-v3",
		FormatVersion: 3,
		SnapshotID:    snapID,
		SourceID:      sourceID,
		Database: mf.DatabaseInfo{
			Engine:      "sqlite",
			LogicalSize: int64(len(rawData)),
			PageSize:    4096,
		},
		Snapshot: mf.SnapshotInfo{
			Mode:       "logical",
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			RootDigest: rootDigest,
		},
		Chunks: manifestChunks,
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, nil, nil, err
	}

	manifestKey := fmt.Sprintf("%s/snapshots/%s/manifest.json", sourceID, snapID)
	_, err = env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  manifestKey,
		Body: bytes.NewReader(manifestBytes),
	})
	if err != nil {
		return nil, nil, nil, err
	}

	sigBytes, err := env.Signer.Sign(manifestBytes)
	if err != nil {
		return nil, nil, nil, err
	}

	sigKey := fmt.Sprintf("%s/snapshots/%s/manifest.sig", sourceID, snapID)
	_, err = env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  sigKey,
		Body: bytes.NewReader(sigBytes),
	})
	if err != nil {
		return nil, nil, nil, err
	}

	completeKey := fmt.Sprintf("%s/snapshots/%s/complete.json", sourceID, snapID)
	completeBytes := []byte(`{"status":"COMMITTED","committed_at":"2026-08-30T19:00:00Z"}`)
	_, err = env.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  completeKey,
		Body: bytes.NewReader(completeBytes),
	})
	if err != nil {
		return nil, nil, nil, err
	}

	return manifest, manifestBytes, sigBytes, nil
}
