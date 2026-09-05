package verification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/adapters/catalogue/memory"
	ed25519signer "github.com/dbvault/dbvault/internal/adapters/manifest/ed25519"
	"github.com/dbvault/dbvault/internal/application/chunking"
	"github.com/dbvault/dbvault/internal/application/digest"
	mf "github.com/dbvault/dbvault/internal/application/manifest"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

// mockMemoryStore implements ports.ObjectStore in-memory for unit testing
type mockMemoryStore struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

func newMockStore() *mockMemoryStore {
	return &mockMemoryStore{objects: make(map[string][]byte)}
}

func (m *mockMemoryStore) Name() string                       { return "mock-memory" }
func (m *mockMemoryStore) Validate(ctx context.Context) error { return nil }
func (m *mockMemoryStore) Capabilities(ctx context.Context) (ports.StorageCapabilities, error) {
	return ports.StorageCapabilities{RangeReads: true}, nil
}
func (m *mockMemoryStore) Put(ctx context.Context, req ports.PutObjectRequest) (ports.StoredObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var data []byte
	if req.Body != nil {
		data, _ = io.ReadAll(req.Body)
	}
	m.objects[req.Key] = data
	return ports.StoredObject{Key: req.Key, Size: int64(len(data)), LastModified: time.Now().UTC()}, nil
}
func (m *mockMemoryStore) Get(ctx context.Context, req ports.GetObjectRequest) (ports.ObjectReader, ports.StoredObject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.objects[req.Key]
	if !ok {
		return nil, ports.StoredObject{}, fmt.Errorf("object not found: %s", req.Key)
	}
	return io.NopCloser(bytes.NewReader(data)), ports.StoredObject{Key: req.Key, Size: int64(len(data))}, nil
}
func (m *mockMemoryStore) Head(ctx context.Context, key string) (ports.StoredObject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.objects[key]
	if !ok {
		return ports.StoredObject{}, fmt.Errorf("object not found: %s", key)
	}
	return ports.StoredObject{Key: key, Size: int64(len(data))}, nil
}
func (m *mockMemoryStore) List(ctx context.Context, req ports.ListObjectsRequest) (ports.ListObjectsResult, error) {
	return ports.ListObjectsResult{}, nil
}
func (m *mockMemoryStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

func setupTestSnapshot(t *testing.T, store *mockMemoryStore, cat *memory.Catalogue, signer ed25519signer.Signer, snapID domain.SnapshotID, sourceID domain.SourceID) {
	dedupKey := []byte("dedup-key-32-bytes-long-ok-12345")
	chunk1Data := []byte("page chunk 1 raw contents")
	chunk2Data := []byte("page chunk 2 raw contents")

	cid1 := digest.ChunkID(dedupKey, 4096, chunk1Data)
	cid2 := digest.ChunkID(dedupKey, 4096, chunk2Data)

	chunk1Key := fmt.Sprintf("%s/chunks/%s.dvchunk", sourceID, cid1)
	chunk2Key := fmt.Sprintf("%s/chunks/%s.dvchunk", sourceID, cid2)

	// Put encrypted chunk objects in storage
	store.Put(context.Background(), ports.PutObjectRequest{Key: chunk1Key, Body: bytes.NewReader([]byte("encrypted-payload-1"))})
	store.Put(context.Background(), ports.PutObjectRequest{Key: chunk2Key, Body: bytes.NewReader([]byte("encrypted-payload-2"))})

	plainChunks := []chunking.PlainChunk{
		{Index: 0, Sequence: 0, PlaintextSize: int64(len(chunk1Data)), Data: chunk1Data},
		{Index: 1, Sequence: 1, PlaintextSize: int64(len(chunk2Data)), Data: chunk2Data},
	}
	rootDigest := digest.RootDigest(dedupKey, plainChunks, []string{cid1, cid2}, int64(len(chunk1Data)+len(chunk2Data)), 4096)

	manifest := mf.SnapshotManifest{
		Format:        "dbvault-manifest-v3",
		FormatVersion: 3,
		SnapshotID:    string(snapID),
		SourceID:      string(sourceID),
		RootDigest:    rootDigest,
		Database: mf.DatabaseInfo{
			Engine:      "sqlite",
			PageSize:    4096,
			LogicalSize: int64(len(chunk1Data) + len(chunk2Data)),
		},
		Snapshot: mf.SnapshotInfo{
			RootDigest: rootDigest,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		},
		Chunks: []mf.ChunkInfo{
			{Sequence: 0, ChunkID: cid1, ObjectKey: chunk1Key, PlaintextSize: int64(len(chunk1Data))},
			{Sequence: 1, ChunkID: cid2, ObjectKey: chunk2Key, PlaintextSize: int64(len(chunk2Data))},
		},
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	manifestKey := fmt.Sprintf("%s/snapshots/%s/manifest.json", sourceID, snapID)
	store.Put(context.Background(), ports.PutObjectRequest{Key: manifestKey, Body: bytes.NewReader(manifestBytes)})

	sigBytes, err := signer.Sign(manifestBytes)
	if err != nil {
		t.Fatalf("failed to sign manifest: %v", err)
	}
	sigKey := fmt.Sprintf("%s/snapshots/%s/manifest.sig", sourceID, snapID)
	store.Put(context.Background(), ports.PutObjectRequest{Key: sigKey, Body: bytes.NewReader(sigBytes)})

	h := sha256.Sum256(manifestBytes)
	comp := mf.Completion{
		Format:         "dbvault-completion",
		FormatVersion:  1,
		SnapshotID:     string(snapID),
		ManifestDigest: hex.EncodeToString(h[:]),
		CommittedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	compBytes, _ := json.Marshal(comp)
	compKey := fmt.Sprintf("%s/snapshots/%s/complete.json", sourceID, snapID)
	store.Put(context.Background(), ports.PutObjectRequest{Key: compKey, Body: bytes.NewReader(compBytes)})

	snap := domain.Snapshot{
		ID:                  snapID,
		SourceID:            sourceID,
		ManifestObjectKey:   manifestKey,
		CompletionObjectKey: compKey,
		Status:              domain.SnapshotCommitted,
		RootDigest:          rootDigest,
	}
	cat.CreateSnapshot(context.Background(), snap, nil, nil)
}

func TestVerification_RemoteVerificationSuccess(t *testing.T) {
	store := newMockStore()
	cat := memory.New()
	signer, err := ed25519signer.Generate("key-test-verify")
	if err != nil {
		t.Fatal(err)
	}

	snapID := domain.SnapshotID("snap_success_01")
	sourceID := domain.SourceID("src_01")
	setupTestSnapshot(t, store, cat, signer, snapID, sourceID)

	// Create verification service using only public key (zero knowledge of private key, zero chunk downloads)
	verifier := ed25519signer.New("key-test-verify", nil, signer.Public)
	svc := Service{
		Catalogue: cat,
		Store:     store,
		Signer:    verifier,
		DedupKey:  []byte("dedup-key-32-bytes-long-ok-12345"),
	}

	result, err := svc.VerifyRemote(context.Background(), snapID)
	if err != nil {
		t.Fatalf("remote verification failed: %v", err)
	}
	if !result.Verified {
		t.Fatal("expected result.Verified to be true")
	}
	if result.ChunkCount != 2 {
		t.Fatalf("expected chunk count 2, got %d", result.ChunkCount)
	}
}

func TestVerification_MissingOrTamperedSignatureFails(t *testing.T) {
	store := newMockStore()
	cat := memory.New()
	signer, _ := ed25519signer.Generate("key-valid")
	snapID := domain.SnapshotID("snap_tamper_sig")
	sourceID := domain.SourceID("src_tamper")
	setupTestSnapshot(t, store, cat, signer, snapID, sourceID)

	// Tamper the signature in storage
	sigKey := fmt.Sprintf("%s/snapshots/%s/manifest.sig", sourceID, snapID)
	store.Put(context.Background(), ports.PutObjectRequest{Key: sigKey, Body: bytes.NewReader([]byte(`{"format":"dbvault-manifest-signature","format_version":1,"algorithm":"ed25519","key_id":"k","signature":"AAAA"}`))})

	svc := Service{
		Catalogue: cat,
		Store:     store,
		Signer:    signer,
	}

	_, err := svc.VerifyRemote(context.Background(), snapID)
	if err == nil {
		t.Fatal("expected verification failure for tampered signature, got nil")
	}
}

func TestVerification_MissingChunkFails(t *testing.T) {
	store := newMockStore()
	cat := memory.New()
	signer, _ := ed25519signer.Generate("key-missing-chunk")
	snapID := domain.SnapshotID("snap_missing_chunk")
	sourceID := domain.SourceID("src_chunk")
	setupTestSnapshot(t, store, cat, signer, snapID, sourceID)

	// Delete one of the chunk files in object storage
	for k := range store.objects {
		if len(k) > 10 && k[len(k)-8:] == ".dvchunk" {
			store.Delete(context.Background(), k)
			break
		}
	}

	svc := Service{
		Catalogue: cat,
		Store:     store,
		Signer:    signer,
	}

	_, err := svc.VerifyRemote(context.Background(), snapID)
	if err == nil {
		t.Fatal("expected verification failure for missing chunk in storage, got nil")
	}
}

func TestVerification_FailClosedOnTrackedCompletionRecord(t *testing.T) {
	store := newMockStore()
	cat := memory.New()
	signer, _ := ed25519signer.Generate("key-failclosed")
	snapID := domain.SnapshotID("snap_fc_verify")
	sourceID := domain.SourceID("src_fc_verify")
	setupTestSnapshot(t, store, cat, signer, snapID, sourceID)

	// Case 1: catalogue-tracked completion record removed from storage.
	compKey := fmt.Sprintf("%s/snapshots/%s/complete.json", sourceID, snapID)
	store.Delete(context.Background(), compKey)

	svc := Service{Catalogue: cat, Store: store, Signer: signer}
	_, err := svc.VerifyRemote(context.Background(), snapID)
	if err == nil || !strings.Contains(err.Error(), "completion record is missing") {
		t.Fatalf("expected fail-closed error for missing completion record, got %v", err)
	}

	// Case 2: record present but with an empty manifest digest.
	store.Put(context.Background(), ports.PutObjectRequest{Key: compKey, Body: bytes.NewReader([]byte(`{"format":"dbvault-completion","format_version":1,"snapshot_id":"snap_fc_verify"}`))})
	_, err = svc.VerifyRemote(context.Background(), snapID)
	if err == nil || !strings.Contains(err.Error(), "empty manifest digest") {
		t.Fatalf("expected fail-closed error for empty completion digest, got %v", err)
	}
}
