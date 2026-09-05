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

type restoreTestFixture struct {
	Store      *mockMemStore
	Cat        *memory.Catalogue
	Signer     ed25519signer.Signer
	Encryptor  *aead.Encryptor
	Compressor ports.Compressor
	DedupKey   []byte
	Service    *Service
	SnapID     domain.SnapshotID
	SourceID   domain.SourceID
	PageData   []byte
	ChunkID    string
	ChunkKey   string
	ManKey     string
	SigKey     string
	CompKey    string
	Manifest   mf.SnapshotManifest
}

func newRestoreTestFixture(t *testing.T, snapName string) *restoreTestFixture {
	t.Helper()
	store := newMemStore()
	cat := memory.New()
	signer, err := ed25519signer.Generate("key-" + snapName)
	if err != nil {
		t.Fatalf("failed to generate signer: %v", err)
	}
	encKey := bytes.Repeat([]byte{0x42}, 32)
	enc, err := aead.New(encKey)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}
	comp := gzipc.New(6)

	svc := &Service{
		Catalogue:  cat,
		Store:      store,
		Compressor: comp,
		Encryptor:  enc,
		Signer:     signer,
		DedupKey:   encKey,
	}

	snapID := domain.SnapshotID("snap_" + snapName)
	sourceID := domain.SourceID("src_" + snapName)

	// Build a valid 4096-byte SQLite page
	header := []byte("SQLite format 3\x00")
	pageData := append(header, bytes.Repeat([]byte{0xaa}, 4096-len(header))...)
	cid := digest.ChunkID(encKey, 4096, pageData)
	chunkKey := fmt.Sprintf("%s/chunks/%s.dvchunk", sourceID, cid)

	// Compress and encrypt
	var compBuf bytes.Buffer
	if err := comp.Compress(context.Background(), bytes.NewReader(pageData), &compBuf); err != nil {
		t.Fatalf("compress failed: %v", err)
	}
	encrypted, err := enc.EncryptChunk(context.Background(), cid, compBuf.Bytes())
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	store.Put(context.Background(), ports.PutObjectRequest{Key: chunkKey, Body: bytes.NewReader(encrypted)})

	plainChunks := []chunking.PlainChunk{
		{Index: 0, Sequence: 0, PageStart: 0, PageCount: 1, PlaintextSize: 4096, Data: pageData},
	}
	rootDigest := digest.RootDigest(encKey, plainChunks, []string{cid}, 4096, 4096)

	man := mf.SnapshotManifest{
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

	manBytes, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest failed: %v", err)
	}
	manKey := fmt.Sprintf("%s/snapshots/%s/manifest.json", sourceID, snapID)
	store.Put(context.Background(), ports.PutObjectRequest{Key: manKey, Body: bytes.NewReader(manBytes)})

	sigBytes, err := signer.Sign(manBytes)
	if err != nil {
		t.Fatalf("sign manifest failed: %v", err)
	}
	sigKey := fmt.Sprintf("%s/snapshots/%s/manifest.sig", sourceID, snapID)
	store.Put(context.Background(), ports.PutObjectRequest{Key: sigKey, Body: bytes.NewReader(sigBytes)})

	h := sha256.Sum256(manBytes)
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
		ManifestObjectKey:   manKey,
		CompletionObjectKey: compKey,
		Status:              domain.SnapshotCommitted,
		RootDigest:          rootDigest,
	}, nil, nil)

	return &restoreTestFixture{
		Store:      store,
		Cat:        cat,
		Signer:     signer,
		Encryptor:  enc,
		Compressor: comp,
		DedupKey:   encKey,
		Service:    svc,
		SnapID:     snapID,
		SourceID:   sourceID,
		PageData:   pageData,
		ChunkID:    cid,
		ChunkKey:   chunkKey,
		ManKey:     manKey,
		SigKey:     sigKey,
		CompKey:    compKey,
		Manifest:   man,
	}
}

// TestStress_PreRestore_MissingSignatureRejection verifies rejection when signature object is absent.
func TestStress_PreRestore_MissingSignatureRejection(t *testing.T) {
	f := newRestoreTestFixture(t, "miss_sig")
	f.Store.Delete(context.Background(), f.SigKey)

	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "output.db")

	err := f.Service.Restore(context.Background(), f.SnapID, targetPath, false)
	if err == nil {
		t.Fatal("expected restore failure on missing signature, got nil")
	}

	if _, err := os.Stat(targetPath); err == nil {
		t.Fatalf("target file %s must NOT exist after signature failure", targetPath)
	}
}

// TestStress_PreRestore_TamperedSignatureRejection verifies rejection on signature bit-flip.
func TestStress_PreRestore_TamperedSignatureRejection(t *testing.T) {
	f := newRestoreTestFixture(t, "bad_sig")
	f.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  f.SigKey,
		Body: bytes.NewReader([]byte(`{"format":"dbvault-manifest-signature","format_version":1,"algorithm":"ed25519","key_id":"k","signature":"deadbeef"}`)),
	})

	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "output.db")

	err := f.Service.Restore(context.Background(), f.SnapID, targetPath, false)
	if err == nil {
		t.Fatal("expected restore failure on invalid signature, got nil")
	}

	if _, err := os.Stat(targetPath); err == nil {
		t.Fatalf("target file %s must NOT exist after signature failure", targetPath)
	}
}

// TestStress_PreRestore_CompletionDigestMismatchRejection verifies rejection if complete.json digest does not match manifest.
func TestStress_PreRestore_CompletionDigestMismatchRejection(t *testing.T) {
	f := newRestoreTestFixture(t, "bad_comp")
	badComp := mf.Completion{
		Format:         "dbvault-completion",
		FormatVersion:  1,
		SnapshotID:     string(f.SnapID),
		ManifestDigest: "0000000000000000000000000000000000000000000000000000000000000000",
		CommittedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	cb, _ := json.Marshal(badComp)
	f.Store.Put(context.Background(), ports.PutObjectRequest{Key: f.CompKey, Body: bytes.NewReader(cb)})

	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "output.db")

	err := f.Service.Restore(context.Background(), f.SnapID, targetPath, false)
	if err == nil {
		t.Fatal("expected restore failure on completion digest mismatch, got nil")
	}

	if _, err := os.Stat(targetPath); err == nil {
		t.Fatalf("target file %s must NOT exist after completion mismatch", targetPath)
	}
}

// TestStress_PreRestore_MalformedManifestJSON verifies rejection on truncated JSON manifest.
func TestStress_PreRestore_MalformedManifestJSON(t *testing.T) {
	f := newRestoreTestFixture(t, "bad_json")
	f.Store.Put(context.Background(), ports.PutObjectRequest{
		Key:  f.ManKey,
		Body: bytes.NewReader([]byte(`{"format":"dbvault-manifest-v3", "snapshot_id":`)),
	})
	sig, _ := f.Signer.Sign([]byte(`{"format":"dbvault-manifest-v3", "snapshot_id":`))
	f.Store.Put(context.Background(), ports.PutObjectRequest{Key: f.SigKey, Body: bytes.NewReader(sig)})

	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "output.db")

	err := f.Service.Restore(context.Background(), f.SnapID, targetPath, false)
	if err == nil {
		t.Fatal("expected restore failure on malformed manifest JSON, got nil")
	}

	if _, err := os.Stat(targetPath); err == nil {
		t.Fatalf("target file %s must NOT exist after JSON parse failure", targetPath)
	}
}

// TestStress_PreRestore_CorruptedChunkCiphertext verifies rejection when AES-GCM decryption fails.
func TestStress_PreRestore_CorruptedChunkCiphertext(t *testing.T) {
	f := newRestoreTestFixture(t, "bad_cipher")
	r, obj, err := f.Store.Get(context.Background(), ports.GetObjectRequest{Key: f.ChunkKey})
	if err != nil {
		t.Fatalf("failed to get chunk: %v", err)
	}
	data, _ := io.ReadAll(r)
	_ = r.Close()
	data[len(data)-1] ^= 0xff
	f.Store.Put(context.Background(), ports.PutObjectRequest{Key: f.ChunkKey, Body: bytes.NewReader(data), Size: obj.Size})

	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "output.db")

	err = f.Service.Restore(context.Background(), f.SnapID, targetPath, false)
	if err == nil {
		t.Fatal("expected restore failure on corrupted chunk ciphertext, got nil")
	}

	if _, statErr := os.Stat(targetPath); statErr == nil {
		t.Fatalf("target file %s must NOT exist after decryption failure", targetPath)
	}

	tmpPath := targetPath + ".dbvault-restore-tmp"
	if _, statErr := os.Stat(tmpPath); statErr == nil {
		t.Fatalf("temporary restore file %s must be cleaned up on error", tmpPath)
	}
}

// TestStress_PreRestore_ChunkPlaintextHashMismatch verifies chunk hash validation against ChunkID.
func TestStress_PreRestore_ChunkPlaintextHashMismatch(t *testing.T) {
	f := newRestoreTestFixture(t, "chunk_hash_mismatch")

	differentData := bytes.Repeat([]byte{0xee}, 4096)
	var compBuf bytes.Buffer
	f.Compressor.Compress(context.Background(), bytes.NewReader(differentData), &compBuf)
	encData, _ := f.Encryptor.EncryptChunk(context.Background(), f.ChunkID, compBuf.Bytes())
	f.Store.Put(context.Background(), ports.PutObjectRequest{Key: f.ChunkKey, Body: bytes.NewReader(encData)})

	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "output.db")

	err := f.Service.Restore(context.Background(), f.SnapID, targetPath, false)
	if err == nil {
		t.Fatal("expected restore failure on chunk hash mismatch, got nil")
	}

	if _, statErr := os.Stat(targetPath); statErr == nil {
		t.Fatalf("target file %s must NOT exist after chunk hash mismatch", targetPath)
	}
}

// TestStress_PreRestore_ExistingTargetReplacedSafely verifies replace=false rejects and replace=true succeeds.
func TestStress_PreRestore_ExistingTargetReplacedSafely(t *testing.T) {
	f := newRestoreTestFixture(t, "replace_flow")

	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "output.db")

	os.WriteFile(targetPath, []byte("pre-existing database file"), 0600)

	err := f.Service.Restore(context.Background(), f.SnapID, targetPath, false)
	if err == nil {
		t.Fatal("expected restore error when target exists and replace=false")
	}
	existingContent, _ := os.ReadFile(targetPath)
	if string(existingContent) != "pre-existing database file" {
		t.Fatalf("existing file was altered when replace=false: %s", string(existingContent))
	}

	err = f.Service.Restore(context.Background(), f.SnapID, targetPath, true)
	if err != nil {
		t.Fatalf("expected restore to succeed with replace=true, got: %v", err)
	}

	restoredContent, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}
	if !bytes.Equal(restoredContent, f.PageData) {
		t.Fatal("restored content does not match expected page data")
	}
}

// TestStress_PreRestore_TamperedMerkleRootRejection tests whether a signed manifest with a tampered RootDigest is rejected.
func TestStress_PreRestore_TamperedMerkleRootRejection(t *testing.T) {
	f := newRestoreTestFixture(t, "tampered_root")

	// Mutate the RootDigest in the manifest to a bogus value, but sign it properly with completion
	f.Manifest.RootDigest = "0000000000000000000000000000000000000000000000000000000000000000"
	f.Manifest.Snapshot.RootDigest = "0000000000000000000000000000000000000000000000000000000000000000"

	manBytes, _ := json.MarshalIndent(f.Manifest, "", "  ")
	f.Store.Put(context.Background(), ports.PutObjectRequest{Key: f.ManKey, Body: bytes.NewReader(manBytes)})

	sigBytes, _ := f.Signer.Sign(manBytes)
	f.Store.Put(context.Background(), ports.PutObjectRequest{Key: f.SigKey, Body: bytes.NewReader(sigBytes)})

	h := sha256.Sum256(manBytes)
	compRec := mf.Completion{
		Format:         "dbvault-completion",
		FormatVersion:  1,
		SnapshotID:     string(f.SnapID),
		ManifestDigest: hex.EncodeToString(h[:]),
		CommittedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	compBytes, _ := json.Marshal(compRec)
	f.Store.Put(context.Background(), ports.PutObjectRequest{Key: f.CompKey, Body: bytes.NewReader(compBytes)})

	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "output.db")

	err := f.Service.Restore(context.Background(), f.SnapID, targetPath, false)
	if err == nil {
		t.Fatal("expected restore failure on tampered RootDigest, got nil")
	}

	if _, err := os.Stat(targetPath); err == nil {
		t.Fatalf("target file %s must NOT exist after root digest tampering rejection", targetPath)
	}
}
