package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/adapters/catalogue/memory"
	gzipc "github.com/dbvault/dbvault/internal/adapters/compression/gzip"
	"github.com/dbvault/dbvault/internal/adapters/encryption/aead"
	ed25519signer "github.com/dbvault/dbvault/internal/adapters/manifest/ed25519"
	"github.com/dbvault/dbvault/internal/adapters/scratch"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type mockIDGen struct {
	mu  sync.Mutex
	seq int
}

func (m *mockIDGen) NewID(prefix string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), m.seq)
}

type fixedClock struct {
	t time.Time
}

func (c fixedClock) Now() time.Time { return c.t }

type mockArtifact struct {
	path string
	size int64
	meta ports.SnapshotMetadata
}

func (a *mockArtifact) Path() string                     { return a.path }
func (a *mockArtifact) Size() int64                      { return a.size }
func (a *mockArtifact) Metadata() ports.SnapshotMetadata { return a.meta }
func (a *mockArtifact) Close() error                     { return nil }

type mockSourceDriver struct {
	artifact *mockArtifact
}

func (m *mockSourceDriver) Name() string { return "mock-sqlite" }
func (m *mockSourceDriver) Validate(ctx context.Context, source domain.Source) error {
	return nil
}
func (m *mockSourceDriver) Inspect(ctx context.Context, source domain.Source) (domain.SourceInspection, error) {
	return domain.SourceInspection{EngineVersion: "3.40.0", PageSize: 4096, PageCount: 4, FileSize: 16384}, nil
}
func (m *mockSourceDriver) CreateSnapshot(ctx context.Context, req ports.SnapshotRequest) (ports.SnapshotArtifact, error) {
	return m.artifact, nil
}

type memoryStoreWithCounts struct {
	mu         sync.Mutex
	objects    map[string][]byte
	headCounts map[string]int
	putCounts  map[string]int
}

func newMemStoreWithCounts() *memoryStoreWithCounts {
	return &memoryStoreWithCounts{
		objects:    make(map[string][]byte),
		headCounts: make(map[string]int),
		putCounts:  make(map[string]int),
	}
}

func (m *memoryStoreWithCounts) Name() string                       { return "mem-counts" }
func (m *memoryStoreWithCounts) Validate(ctx context.Context) error { return nil }
func (m *memoryStoreWithCounts) Capabilities(ctx context.Context) (ports.StorageCapabilities, error) {
	return ports.StorageCapabilities{MultipartUpload: true, UserMetadata: true, RangeReads: true}, nil
}
func (m *memoryStoreWithCounts) Put(ctx context.Context, req ports.PutObjectRequest) (ports.StoredObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.putCounts[req.Key]++
	var data []byte
	if req.Body != nil {
		data, _ = io.ReadAll(req.Body)
	}
	m.objects[req.Key] = data
	return ports.StoredObject{Key: req.Key, Size: int64(len(data)), LastModified: time.Now().UTC()}, nil
}
func (m *memoryStoreWithCounts) Get(ctx context.Context, req ports.GetObjectRequest) (ports.ObjectReader, ports.StoredObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.objects[req.Key]
	if !ok {
		return nil, ports.StoredObject{}, domain.NewError(domain.ErrChunkMissing, "not found", nil)
	}
	return io.NopCloser(bytes.NewReader(data)), ports.StoredObject{Key: req.Key, Size: int64(len(data))}, nil
}
func (m *memoryStoreWithCounts) Head(ctx context.Context, key string) (ports.StoredObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.headCounts[key]++
	data, ok := m.objects[key]
	if !ok {
		return ports.StoredObject{}, domain.NewError(domain.ErrChunkMissing, "not found", nil)
	}
	return ports.StoredObject{Key: key, Size: int64(len(data))}, nil
}
func (m *memoryStoreWithCounts) List(ctx context.Context, req ports.ListObjectsRequest) (ports.ListObjectsResult, error) {
	return ports.ListObjectsResult{}, nil
}
func (m *memoryStoreWithCounts) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

// TestStress_CAS_ChunkDeduplicationAndReuse tests chunk deduplication across successive backup runs.
func TestStress_CAS_ChunkDeduplicationAndReuse(t *testing.T) {
	store := newMemStoreWithCounts()
	cat := memory.New()
	signer, _ := ed25519signer.Generate("key-cas-test")
	encKey := bytes.Repeat([]byte{0x33}, 32)
	enc, _ := aead.New(encKey)
	comp := gzipc.New(6)
	tempDir := t.TempDir()
	scratchMgr := scratch.New(tempDir)
	clock := fixedClock{t: time.Now().UTC()}
	idGen := &mockIDGen{}

	repo := domain.Repository{
		ID:   "repo_s3_cas",
		Name: "S3 CAS Repository",
		Encryption: domain.EncryptionPolicy{
			Algorithm: "aes-256-gcm",
			KeyID:     "v1",
		},
	}

	// Create a 16KB mock SQLite database (4 pages of 4096 bytes)
	dbFile1 := filepath.Join(tempDir, "db1.sqlite")
	page0 := append([]byte("SQLite format 3\x00"), bytes.Repeat([]byte{0x11}, 4096-16)...)
	page1 := bytes.Repeat([]byte{0x22}, 4096)
	page2 := bytes.Repeat([]byte{0x33}, 4096)
	page3 := bytes.Repeat([]byte{0x44}, 4096)
	db1Bytes := append(page0, append(page1, append(page2, page3...)...)...)
	os.WriteFile(dbFile1, db1Bytes, 0600)

	art1 := &mockArtifact{
		path: dbFile1,
		size: int64(len(db1Bytes)),
		meta: ports.SnapshotMetadata{
			EngineVersion: "3.40.0",
			PageSize:      4096,
			PageCount:     4,
			SchemaDigest:  "schema_hash_1",
		},
	}

	sourceDriver1 := &mockSourceDriver{artifact: art1}

	svc := &Service{
		Catalogue:        cat,
		Source:           sourceDriver1,
		Store:            store,
		Scratch:          scratchMgr,
		Compressor:       comp,
		Encryptor:        enc,
		Signer:           signer,
		Clock:            clock,
		IDs:              idGen,
		DedupKey:         encKey,
		Repository:       repo,
		TargetChunkBytes: 4096,
	}

	cmd1 := Command{
		Source: domain.Source{
			ID:     "src_app_prod",
			Name:   "Production App DB",
			Driver: "sqlite",
		},
		Trigger: domain.TriggerManual,
	}

	// 1. First backup run: all 4 chunks must be newly uploaded
	res1, err := svc.Create(context.Background(), cmd1)
	if err != nil {
		t.Fatalf("first backup run failed: %v", err)
	}
	if res1.Duplicate {
		t.Fatal("first run should not be marked duplicate")
	}
	if res1.Snapshot.ChunkCount != 4 {
		t.Fatalf("expected 4 chunks, got %d", res1.Snapshot.ChunkCount)
	}

	// Count chunk uploads in store
	chunkUploadsAfterRun1 := 0
	for k := range store.objects {
		if filepath.Ext(k) == ".dvchunk" {
			chunkUploadsAfterRun1++
		}
	}
	if chunkUploadsAfterRun1 != 4 {
		t.Fatalf("expected 4 chunk objects in store after run 1, got %d", chunkUploadsAfterRun1)
	}

	// 2. Second backup run with 1 page modified: page 2 changed, pages 0, 1, 3 identical
	dbFile2 := filepath.Join(tempDir, "db2.sqlite")
	page2Modified := bytes.Repeat([]byte{0x99}, 4096)
	db2Bytes := append(page0, append(page1, append(page2Modified, page3...)...)...)
	os.WriteFile(dbFile2, db2Bytes, 0600)

	art2 := &mockArtifact{
		path: dbFile2,
		size: int64(len(db2Bytes)),
		meta: ports.SnapshotMetadata{
			EngineVersion: "3.40.0",
			PageSize:      4096,
			PageCount:     4,
			SchemaDigest:  "schema_hash_2",
		},
	}
	sourceDriver1.artifact = art2

	res2, err := svc.Create(context.Background(), cmd1)
	if err != nil {
		t.Fatalf("second backup run failed: %v", err)
	}
	if res2.Duplicate {
		t.Fatal("second run has modified page, must not be marked duplicate")
	}
	if res2.Run.ReusedBytes != 3*4096 {
		t.Fatalf("expected 12288 reused bytes (3 chunks), got %d", res2.Run.ReusedBytes)
	}

	chunkUploadsAfterRun2 := 0
	for k := range store.objects {
		if filepath.Ext(k) == ".dvchunk" {
			chunkUploadsAfterRun2++
		}
	}
	// Total unique chunk objects should now be 4 + 1 = 5
	if chunkUploadsAfterRun2 != 5 {
		t.Fatalf("expected 5 total chunk objects in store after run 2, got %d", chunkUploadsAfterRun2)
	}
}

// TestStress_CAS_MultiTenantKeyPathFormatting verifies proper key isolation between tenants.
func TestStress_CAS_MultiTenantKeyPathFormatting(t *testing.T) {
	keyA := chunkObjectKey("tenant_alpha", "repo1", "v1", "a1b2c3d4e5f600112233445566778899")
	keyB := chunkObjectKey("tenant_beta", "repo1", "v1", "a1b2c3d4e5f600112233445566778899")

	if keyA == keyB {
		t.Fatalf("tenant keys must be isolated: keyA=%s, keyB=%s", keyA, keyB)
	}

	expectedA := "tenant_alpha/chunks/v1/a1/b2/a1b2c3d4e5f600112233445566778899.dvchunk"
	expectedB := "tenant_beta/chunks/v1/a1/b2/a1b2c3d4e5f600112233445566778899.dvchunk"

	if keyA != expectedA {
		t.Fatalf("expected keyA %s, got %s", expectedA, keyA)
	}
	if keyB != expectedB {
		t.Fatalf("expected keyB %s, got %s", expectedB, keyB)
	}
}
