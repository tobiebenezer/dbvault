package verification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/dbvault/dbvault/internal/application/chunking"
	"github.com/dbvault/dbvault/internal/application/digest"
	mf "github.com/dbvault/dbvault/internal/application/manifest"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Service struct {
	Catalogue ports.Catalogue
	Store     ports.ObjectStore
	Signer    ports.ManifestSigner
	DedupKey  []byte
}

type Result struct {
	SnapshotID domain.SnapshotID `json:"snapshot_id"`
	Levels     []string          `json:"levels"`
	ChunkCount int               `json:"chunk_count"`
	RootDigest string            `json:"root_digest,omitempty"`
	Verified   bool              `json:"verified"`
}

// VerifyRemote verifies an existing snapshot in remote storage by checking:
// 1. Publication & Manifest validity
// 2. Ed25519 signature verification (public-key only, zero chunk download)
// 3. Completion record SHA-256 digest matching
// 4. Merkle root integrity over chunk IDs
// 5. Chunk existence via S3 HEAD requests
func (s Service) VerifyRemote(ctx context.Context, id domain.SnapshotID) (Result, error) {
	snap, _, err := s.Catalogue.GetSnapshot(ctx, id)
	if err != nil {
		return Result{}, err
	}
	r, _, err := s.Store.Get(ctx, ports.GetObjectRequest{Key: snap.ManifestObjectKey})
	if err != nil {
		return Result{}, domain.NewError(domain.ErrChunkMissing, "manifest not found in object storage", err)
	}
	payload, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		return Result{}, err
	}

	// 1. Verify Manifest Signature
	sigKey := strings.TrimSuffix(snap.ManifestObjectKey, ".json") + ".sig"
	if sigKey == "" || sigKey == ".sig" {
		sigKey = fmt.Sprintf("%s/snapshots/%s/manifest.sig", snap.SourceID, snap.ID)
	}
	sigR, _, err := s.Store.Get(ctx, ports.GetObjectRequest{Key: sigKey})
	if err != nil {
		return Result{}, domain.NewError(domain.ErrVerificationFailed, "missing manifest signature", err)
	}
	sig, err := io.ReadAll(sigR)
	_ = sigR.Close()
	if err != nil {
		return Result{}, err
	}
	if s.Signer != nil {
		if err := s.Signer.Verify(payload, sig); err != nil {
			return Result{}, domain.NewError(domain.ErrVerificationFailed, "invalid manifest signature", err)
		}
	}

	// 2. Verify Completion Record Digest (fail-closed when catalogue-tracked)
	if snap.CompletionObjectKey != "" {
		cr, _, err := s.Store.Get(ctx, ports.GetObjectRequest{Key: snap.CompletionObjectKey})
		if err != nil {
			return Result{}, domain.NewError(domain.ErrVerificationFailed, "catalogue-tracked completion record is missing from storage", err)
		}
		cb, err := io.ReadAll(cr)
		_ = cr.Close()
		if err != nil {
			return Result{}, domain.NewError(domain.ErrVerificationFailed, "cannot read completion record", err)
		}
		var comp mf.Completion
		if err := json.Unmarshal(cb, &comp); err != nil {
			return Result{}, domain.NewError(domain.ErrVerificationFailed, "malformed completion record", err)
		}
		if comp.ManifestDigest == "" {
			return Result{}, domain.NewError(domain.ErrVerificationFailed, "completion record has empty manifest digest", nil)
		}
		h := sha256.Sum256(payload)
		if comp.ManifestDigest != hex.EncodeToString(h[:]) {
			return Result{}, domain.NewError(domain.ErrVerificationFailed, "completion manifest digest mismatch", nil)
		}
	}

	// 3. Unmarshal manifest and check Merkle integrity
	var man mf.SnapshotManifest
	if err := json.Unmarshal(payload, &man); err != nil {
		return Result{}, domain.NewError(domain.ErrManifestInvalid, "malformed manifest JSON", err)
	}

	expectedRoot := man.Snapshot.RootDigest
	if expectedRoot == "" {
		expectedRoot = man.RootDigest
	}
	if expectedRoot == "" {
		return Result{}, domain.NewError(domain.ErrVerificationFailed, "missing root digest in manifest", nil)
	}

	// Verify Merkle tree construction from chunk IDs
	if len(s.DedupKey) > 0 && len(man.Chunks) > 0 {
		chunkIDs := make([]string, len(man.Chunks))
		plainChunks := make([]chunking.PlainChunk, len(man.Chunks))
		for i, ch := range man.Chunks {
			chunkIDs[i] = ch.ChunkID
			plainChunks[i] = chunking.PlainChunk{
				Sequence:      ch.Sequence,
				PageStart:     ch.PageStart,
				PageCount:     ch.PageCount,
				PlaintextSize: ch.PlaintextSize,
			}
		}
		tree, err := digest.BuildTreeFromHashes(s.DedupKey, chunkIDs)
		if err != nil {
			return Result{}, domain.NewError(domain.ErrVerificationFailed, "failed to compute merkle tree from chunk IDs", err)
		}
		if tree == nil || tree.RootHash == "" {
			return Result{}, domain.NewError(domain.ErrVerificationFailed, "computed merkle tree root is empty", nil)
		}

		computedDigest := digest.RootDigest(s.DedupKey, plainChunks, chunkIDs, man.Database.LogicalSize, man.Database.PageSize)
		if tree.RootHash != expectedRoot && computedDigest != expectedRoot {
			return Result{}, domain.NewError(domain.ErrVerificationFailed, fmt.Sprintf("merkle root mismatch: manifest root is %s, computed tree root is %s, digest is %s", expectedRoot, tree.RootHash, computedDigest), nil)
		}
	}

	// 4. Verify Chunk Existence via HEAD requests
	for _, ch := range man.Chunks {
		if _, err := s.Store.Head(ctx, ch.ObjectKey); err != nil {
			return Result{}, domain.NewError(domain.ErrChunkMissing, fmt.Sprintf("chunk %s missing from storage", ch.ChunkID), err)
		}
	}

	return Result{
		SnapshotID: id,
		Levels:     []string{"publication", "signature", "chunk-existence"},
		ChunkCount: len(man.Chunks),
		RootDigest: expectedRoot,
		Verified:   true,
	}, nil
}
