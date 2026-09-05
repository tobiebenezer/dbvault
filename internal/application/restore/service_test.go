package restore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/adapters/catalogue/memory"
	gzipc "github.com/dbvault/dbvault/internal/adapters/compression/gzip"
	"github.com/dbvault/dbvault/internal/adapters/encryption/aead"
	ed25519signer "github.com/dbvault/dbvault/internal/adapters/manifest/ed25519"
	"github.com/dbvault/dbvault/internal/application/chunking"
	"github.com/dbvault/dbvault/internal/application/digest"
	mf "github.com/dbvault/dbvault/internal/application/manifest"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type mockMemStore struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

func newMemStore() *mockMemStore {
	return &mockMemStore{objects: make(map[string][]byte)}
}

func (m *mockMemStore) Name() string                       { return "mock-mem" }
func (m *mockMemStore) Validate(ctx context.Context) error { return nil }
func (m *mockMemStore) Capabilities(ctx context.Context) (ports.StorageCapabilities, error) {
	return ports.StorageCapabilities{RangeReads: true}, nil
}
func (m *mockMemStore) Put(ctx context.Context, req ports.PutObjectRequest) (ports.StoredObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var data []byte
	if req.Body != nil {
		data, _ = io.ReadAll(req.Body)
	}
	m.objects[req.Key] = data
	return ports.StoredObject{Key: req.Key, Size: int64(len(data)), LastModified: time.Now().UTC()}, nil
}
func (m *mockMemStore) Get(ctx context.Context, req ports.GetObjectRequest) (ports.ObjectReader, ports.StoredObject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.objects[req.Key]
	if !ok {
		return nil, ports.StoredObject{}, fmt.Errorf("object not found: %s", req.Key)
	}
	return io.NopCloser(bytes.NewReader(data)), ports.StoredObject{Key: req.Key, Size: int64(len(data))}, nil
}
func (m *mockMemStore) Head(ctx context.Context, key string) (ports.StoredObject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.objects[key]
	if !ok {
		return ports.StoredObject{}, fmt.Errorf("object not found: %s", key)
	}
	return ports.StoredObject{Key: key, Size: int64(len(data))}, nil
}
func (m *mockMemStore) List(ctx context.Context, req ports.ListObjectsRequest) (ports.ListObjectsResult, error) {
	return ports.ListObjectsResult{}, nil
}
func (m *mockMemStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

func TestRestore_PreFlightValidationRejection(t *testing.T) {
	store := newMemStore()
	cat := memory.New()
	signer, _ := ed25519signer.Generate("key-restore")
	encKey := bytes.Repeat([]byte{0x77}, 32)
	enc, _ := aead.New(encKey)
	comp := gzipc.New(6)

	svc := &Service{
		Catalogue:  cat,
		Store:      store,
		Compressor: comp,
		Encryptor:  enc,
		Signer:     signer,
		DedupKey:   encKey,
	}

	snapID := domain.SnapshotID("snap_restore_invalid")
	sourceID := domain.SourceID("src_restore")

	// Create manifest with invalid signature
	manifest := mf.SnapshotManifest{
		Format:        "dbvault-manifest-v3",
		FormatVersion: 3,
		SnapshotID:    string(snapID),
		SourceID:      string(sourceID),
		Database: mf.DatabaseInfo{
			Engine:      "sqlite",
			PageSize:    4096,
			LogicalSize: 4096,
		},
	}
	manifestBytes, _ := json.Marshal(manifest)
	manifestKey := fmt.Sprintf("%s/snapshots/%s/manifest.json", sourceID, snapID)
	store.Put(context.Background(), ports.PutObjectRequest{Key: manifestKey, Body: bytes.NewReader(manifestBytes)})

	// Put corrupted signature
	sigKey := fmt.Sprintf("%s/snapshots/%s/manifest.sig", sourceID, snapID)
	store.Put(context.Background(), ports.PutObjectRequest{Key: sigKey, Body: bytes.NewReader([]byte(`{"format":"dbvault-manifest-signature","format_version":1,"algorithm":"ed25519","key_id":"k","signature":"AAAA"}`))})

	cat.CreateSnapshot(context.Background(), domain.Snapshot{
		ID:                snapID,
		SourceID:          sourceID,
		ManifestObjectKey: manifestKey,
	}, nil, nil)

	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "restored.db")

	err := svc.Restore(context.Background(), snapID, targetPath, false)
	if err == nil {
		t.Fatal("expected restore to fail before disk write due to signature verification failure, got nil")
	}

	// Verify target was NOT created on disk
	if _, statErr := os.Stat(targetPath); statErr == nil {
		t.Fatal("target file must NOT exist on disk after pre-flight validation failure")
	}
}

func TestRestore_SuccessfulEndToEndRestore(t *testing.T) {
	store := newMemStore()
	cat := memory.New()
	signer, _ := ed25519signer.Generate("key-restore-ok")
	encKey := bytes.Repeat([]byte{0x55}, 32)
	enc, _ := aead.New(encKey)
	comp := gzipc.New(6)

	svc := &Service{
		Catalogue:  cat,
		Store:      store,
		Compressor: comp,
		Encryptor:  enc,
		Signer:     signer,
		DedupKey:   encKey,
	}

	snapID := domain.SnapshotID("snap_restore_ok")
	sourceID := domain.SourceID("src_ok")

	// Generate SQLite page with valid header
	sqliteHeader := []byte("SQLite format 3\x00")
	pageData := append(sqliteHeader, bytes.Repeat([]byte{0x00}, 4096-len(sqliteHeader))...)

	cid := digest.ChunkID(encKey, 4096, pageData)
	chunkKey := fmt.Sprintf("%s/chunks/%s.dvchunk", sourceID, cid)

	// Compress then encrypt chunk
	var compBuf bytes.Buffer
	comp.Compress(context.Background(), bytes.NewReader(pageData), &compBuf)
	encrypted, err := enc.EncryptChunk(context.Background(), cid, compBuf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	store.Put(context.Background(), ports.PutObjectRequest{Key: chunkKey, Body: bytes.NewReader(encrypted)})

	plainChunks := []chunking.PlainChunk{
		{Index: 0, Sequence: 0, PageStart: 0, PageCount: 1, PlaintextSize: 4096, Data: pageData},
	}
	rootDigest := digest.RootDigest(encKey, plainChunks, []string{cid}, 4096, 4096)

	manifest := mf.SnapshotManifest{
		Format:        "dbvault-manifest-v3",
		FormatVersion: 3,
		SnapshotID:    string(snapID),
		SourceID:      string(sourceID),
		RootDigest:    rootDigest,
		Database: mf.DatabaseInfo{
			Engine:      "sqlite",
			PageSize:    4096,
			LogicalSize: 4096,
		},
		Snapshot: mf.SnapshotInfo{
			RootDigest: rootDigest,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		},
		Chunks: []mf.ChunkInfo{
			{Sequence: 0, ChunkID: cid, ObjectKey: chunkKey, PageStart: 0, PageCount: 1, PlaintextSize: 4096},
		},
	}

	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	manifestKey := fmt.Sprintf("%s/snapshots/%s/manifest.json", sourceID, snapID)
	store.Put(context.Background(), ports.PutObjectRequest{Key: manifestKey, Body: bytes.NewReader(manifestBytes)})

	sigBytes, _ := signer.Sign(manifestBytes)
	sigKey := fmt.Sprintf("%s/snapshots/%s/manifest.sig", sourceID, snapID)
	store.Put(context.Background(), ports.PutObjectRequest{Key: sigKey, Body: bytes.NewReader(sigBytes)})

	h := sha256.Sum256(manifestBytes)
	compRec := mf.Completion{
		Format:         "dbvault-completion",
		FormatVersion:  1,
		SnapshotID:     string(snapID),
		ManifestDigest: hex.EncodeToString(h[:]),
		CommittedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	compBytes, _ := json.Marshal(compRec)
	compKey := fmt.Sprintf("%s/snapshots/%s/complete.json", sourceID, snapID)
	store.Put(context.Background(), ports.PutObjectRequest{Key: compKey, Body: bytes.NewReader(compBytes)})

	cat.CreateSnapshot(context.Background(), domain.Snapshot{
		ID:                  snapID,
		SourceID:            sourceID,
		ManifestObjectKey:   manifestKey,
		CompletionObjectKey: compKey,
		Status:              domain.SnapshotCommitted,
	}, nil, nil)

	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "restored.db")

	err = svc.Restore(context.Background(), snapID, targetPath, false)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// Verify restored file contents and SQLite header
	restoredData, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}
	if !bytes.Equal(restoredData, pageData) {
		t.Fatalf("restored data does not match original page payload")
	}
}

func TestRestore_FailClosedOnTrackedCompletionRecord(t *testing.T) {
	store := newMemStore()
	cat := memory.New()
	signer, _ := ed25519signer.Generate("key-failclosed")
	svc := &Service{Catalogue: cat, Store: store, Signer: signer}

	sourceID := domain.SourceID("src_fc")
	manifestKeyFor := func(snapID domain.SnapshotID) string {
		return fmt.Sprintf("%s/snapshots/%s/manifest.json", sourceID, snapID)
	}

	// Case 1: catalogue tracks a completion record that is absent in storage.
	snapID := domain.SnapshotID("snap_fc_missing")
	manifestKey := manifestKeyFor(snapID)
	compKey := fmt.Sprintf("%s/snapshots/%s/complete.json", sourceID, snapID)
	manifestBytes := []byte(`{"format":"dbvault-snapshot","format_version":1,"snapshot_id":"snap_fc_missing"}`)
	store.Put(context.Background(), ports.PutObjectRequest{Key: manifestKey, Body: bytes.NewReader(manifestBytes)})
	sigBytes, _ := signer.Sign(manifestBytes)
	store.Put(context.Background(), ports.PutObjectRequest{Key: strings.TrimSuffix(manifestKey, ".json") + ".sig", Body: bytes.NewReader(sigBytes)})
	cat.CreateSnapshot(context.Background(), domain.Snapshot{
		ID:                  snapID,
		SourceID:            sourceID,
		ManifestObjectKey:   manifestKey,
		CompletionObjectKey: compKey,
		Status:              domain.SnapshotCommitted,
	}, nil, nil)

	err := svc.Restore(context.Background(), snapID, filepath.Join(t.TempDir(), "out.db"), false)
	if err == nil || !strings.Contains(err.Error(), "completion record is missing") {
		t.Fatalf("expected fail-closed error for missing completion record, got %v", err)
	}

	// Case 2: record exists but has an empty manifest digest.
	snapID2 := domain.SnapshotID("snap_fc_empty")
	manifestKey2 := manifestKeyFor(snapID2)
	compKey2 := fmt.Sprintf("%s/snapshots/%s/complete.json", sourceID, snapID2)
	manifestBytes2 := []byte(`{"format":"dbvault-snapshot","format_version":1,"snapshot_id":"snap_fc_empty"}`)
	store.Put(context.Background(), ports.PutObjectRequest{Key: manifestKey2, Body: bytes.NewReader(manifestBytes2)})
	sigBytes2, _ := signer.Sign(manifestBytes2)
	store.Put(context.Background(), ports.PutObjectRequest{Key: strings.TrimSuffix(manifestKey2, ".json") + ".sig", Body: bytes.NewReader(sigBytes2)})
	store.Put(context.Background(), ports.PutObjectRequest{Key: compKey2, Body: bytes.NewReader([]byte(`{"format":"dbvault-completion","format_version":1,"snapshot_id":"snap_fc_empty"}`))})
	cat.CreateSnapshot(context.Background(), domain.Snapshot{
		ID:                  snapID2,
		SourceID:            sourceID,
		ManifestObjectKey:   manifestKey2,
		CompletionObjectKey: compKey2,
		Status:              domain.SnapshotCommitted,
	}, nil, nil)

	err2 := svc.Restore(context.Background(), snapID2, filepath.Join(t.TempDir(), "out.db"), false)
	if err2 == nil || !strings.Contains(err2.Error(), "empty manifest digest") {
		t.Fatalf("expected fail-closed error for empty completion digest, got %v", err2)
	}
}
