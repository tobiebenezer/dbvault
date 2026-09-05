package verification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// TestStress_Verification_TamperedRootDigest_BypassesCheck demonstrates that VerifyRemote fails to validate the Merkle root.
func TestStress_Verification_TamperedRootDigest_BypassesCheck(t *testing.T) {
	store := newMockStore()
	cat := memory.New()
	signer, err := ed25519signer.Generate("key-test-stress")
	if err != nil {
		t.Fatal(err)
	}

	snapID := domain.SnapshotID("snap_tampered_merkle_root")
	sourceID := domain.SourceID("src_tamper")
	dedupKey := []byte("dedup-key-32-bytes-long-ok-12345")

	chunk1Data := []byte("page chunk 1 raw contents")
	chunk2Data := []byte("page chunk 2 raw contents")
	cid1 := digest.ChunkID(dedupKey, 4096, chunk1Data)
	cid2 := digest.ChunkID(dedupKey, 4096, chunk2Data)
	chunk1Key := fmt.Sprintf("%s/chunks/%s.dvchunk", sourceID, cid1)
	chunk2Key := fmt.Sprintf("%s/chunks/%s.dvchunk", sourceID, cid2)

	store.Put(context.Background(), ports.PutObjectRequest{Key: chunk1Key, Body: bytes.NewReader([]byte("enc1"))})
	store.Put(context.Background(), ports.PutObjectRequest{Key: chunk2Key, Body: bytes.NewReader([]byte("enc2"))})

	plainChunks := []chunking.PlainChunk{
		{Index: 0, Sequence: 0, PlaintextSize: int64(len(chunk1Data)), Data: chunk1Data},
		{Index: 1, Sequence: 1, PlaintextSize: int64(len(chunk2Data)), Data: chunk2Data},
	}
	correctRoot := digest.RootDigest(dedupKey, plainChunks, []string{cid1, cid2}, int64(len(chunk1Data)+len(chunk2Data)), 4096)

	// Tampered root digest in manifest
	bogusRoot := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

	manifest := mf.SnapshotManifest{
		Format:        "dbvault-manifest-v3",
		FormatVersion: 3,
		SnapshotID:    string(snapID),
		SourceID:      string(sourceID),
		RootDigest:    bogusRoot, // TAMPERED!
		Database: mf.DatabaseInfo{
			Engine:      "sqlite",
			PageSize:    4096,
			LogicalSize: int64(len(chunk1Data) + len(chunk2Data)),
		},
		Snapshot: mf.SnapshotInfo{
			RootDigest: bogusRoot, // TAMPERED!
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		},
		Chunks: []mf.ChunkInfo{
			{Sequence: 0, ChunkID: cid1, ObjectKey: chunk1Key, PlaintextSize: int64(len(chunk1Data))},
			{Sequence: 1, ChunkID: cid2, ObjectKey: chunk2Key, PlaintextSize: int64(len(chunk2Data))},
		},
	}

	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	manifestKey := fmt.Sprintf("%s/snapshots/%s/manifest.json", sourceID, snapID)
	store.Put(context.Background(), ports.PutObjectRequest{Key: manifestKey, Body: bytes.NewReader(manifestBytes)})

	sigBytes, _ := signer.Sign(manifestBytes)
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

	cat.CreateSnapshot(context.Background(), domain.Snapshot{
		ID:                  snapID,
		SourceID:            sourceID,
		ManifestObjectKey:   manifestKey,
		CompletionObjectKey: compKey,
		Status:              domain.SnapshotCommitted,
		RootDigest:          correctRoot, // Catalogue has the real root
	}, nil, nil)

	svc := Service{
		Catalogue: cat,
		Store:     store,
		Signer:    signer,
		DedupKey:  dedupKey,
	}

	res, err := svc.VerifyRemote(context.Background(), snapID)
	if err == nil {
		t.Fatalf("expected VerifyRemote to fail for tampered root digest, got verified=%v, res=%+v", res.Verified, res)
	}
	if res.Verified {
		t.Fatal("expected res.Verified to be false when root digest is tampered")
	}
}
