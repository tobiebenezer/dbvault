//go:build !restricted

package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/dbvault/dbvault/internal/adapters/encryption/aead"
	"github.com/dbvault/dbvault/internal/application/chunking"
	"github.com/dbvault/dbvault/internal/application/digest"
	mf "github.com/dbvault/dbvault/internal/application/manifest"
	"github.com/dbvault/dbvault/internal/domain"
)

// TestTier5_Crypto_EndToEndAdversarialPipeline tests full snapshot encryption,
// Merkle tree generation, Ed25519 signing, and corruptions across the entire pipeline.
func TestTier5_Crypto_EndToEndAdversarialPipeline(t *testing.T) {
	ctx := context.Background()
	env := NewTestEnv(t)

	// 1. Generate 17 chunks (odd count) of arbitrary database data
	const chunkCount = 17
	const chunkSize = 4096
	chunks := make([]chunking.PlainChunk, chunkCount)
	chunkDataList := make([][]byte, chunkCount)

	for i := 0; i < chunkCount; i++ {
		data := make([]byte, chunkSize)
		io.ReadFull(rand.Reader, data)
		chunks[i] = chunking.PlainChunk{
			Index:         i,
			Sequence:      i,
			PlaintextSize: int64(chunkSize),
			Data:          data,
		}
		chunkDataList[i] = data
	}

	// 2. Encrypt all chunks using AEAD with derived snapshot key
	enc, err := aead.NewWithDerivation(env.MasterKey, "snap-001")
	if err != nil {
		t.Fatalf("failed to create derived encryptor: %v", err)
	}

	encryptedChunks := make([][]byte, chunkCount)
	chunkIDs := make([]string, chunkCount)
	for i, ch := range chunks {
		cid := digest.ChunkID(env.MasterKey, chunkSize, ch.Data)
		chunkIDs[i] = cid
		ct, err := enc.EncryptChunk(ctx, cid, ch.Data)
		if err != nil {
			t.Fatalf("failed to encrypt chunk %d: %v", i, err)
		}
		encryptedChunks[i] = ct
	}

	// 3. Construct Merkle tree from chunks and hashes
	tree, err := digest.BuildTreeFromHashes(env.MasterKey, chunkIDs)
	if err != nil {
		t.Fatalf("failed to build merkle tree: %v", err)
	}
	if tree.LeafCount != chunkCount {
		t.Fatalf("expected leaf count %d, got %d", chunkCount, tree.LeafCount)
	}

	// 4. Generate manifest and sign with Ed25519
	manifest := mf.SnapshotManifest{
		Format:        "dbvault-snapshot-manifest",
		FormatVersion: 1,
		SnapshotID:    "snap-001",
		SourceID:      "src-postgres-prod",
		RootDigest:    tree.RootHash,
		Snapshot: mf.SnapshotInfo{
			RootDigest: tree.RootHash,
		},
		Chunks: make([]mf.ChunkInfo, chunkCount),
	}
	for i, cid := range chunkIDs {
		manifest.Chunks[i] = mf.ChunkInfo{
			Index:         i,
			Sequence:      i,
			ChunkID:       cid,
			PlaintextSize: int64(chunkSize),
			StoredSize:    int64(len(encryptedChunks[i])),
			ObjectKey:     fmt.Sprintf("chunks/%s", cid),
		}
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	sigBytes, err := env.Signer.Sign(manifestBytes)
	if err != nil {
		t.Fatalf("failed to sign manifest: %v", err)
	}

	// 5. Verification: Signature, Root Digest, and Chunk Decryption
	// a. Signature verification
	if err := env.Signer.Verify(manifestBytes, sigBytes); err != nil {
		t.Fatalf("valid manifest signature verification failed: %v", err)
	}

	// b. Merkle Inclusion Proofs for each chunk
	for i := 0; i < chunkCount; i++ {
		proof, err := digest.GenerateProof(tree, i)
		if err != nil {
			t.Fatalf("failed to generate inclusion proof for chunk %d: %v", i, err)
		}
		if !digest.VerifyInclusionProof(proof, manifest.RootDigest, env.MasterKey) {
			t.Fatalf("valid inclusion proof failed for chunk %d", i)
		}
	}

	// c. Decrypt all chunks and verify byte equality
	for i, ch := range chunks {
		pt, err := enc.DecryptChunk(ctx, chunkIDs[i], encryptedChunks[i])
		if err != nil {
			t.Fatalf("decryption failed for chunk %d: %v", i, err)
		}
		if !bytes.Equal(pt, ch.Data) {
			t.Fatalf("decrypted chunk %d data mismatch", i)
		}
	}

	// 6. ADVERSARIAL INJECTIONS

	// Injection A: Bit flip in ciphertext of chunk 7
	tamperedCT := make([]byte, len(encryptedChunks[7]))
	copy(tamperedCT, encryptedChunks[7])
	tamperedCT[20] ^= 0x01
	if _, err := enc.DecryptChunk(ctx, chunkIDs[7], tamperedCT); err == nil {
		t.Fatal("expected decryption failure for bit-flipped chunk ciphertext, got nil")
	}

	// Injection B: Chunk ID swapping (AAD mismatch)
	if _, err := enc.DecryptChunk(ctx, chunkIDs[0], encryptedChunks[1]); err == nil {
		t.Fatal("expected decryption failure when decrypting chunk 1 with chunk 0 ID")
	}

	// Injection C: Manifest signature tampering
	tamperedSig := make([]byte, len(sigBytes))
	copy(tamperedSig, sigBytes)
	tamperedSig[len(tamperedSig)-15] ^= 0x55
	if err := env.Signer.Verify(manifestBytes, tamperedSig); err == nil {
		t.Fatal("expected verification failure for tampered signature envelope")
	}

	// Injection D: Manifest payload tampering
	tamperedManifestBytes := bytes.Replace(manifestBytes, []byte(chunkIDs[0]), []byte("forged-chunk-id-0000000000000000"), 1)
	if err := env.Signer.Verify(tamperedManifestBytes, sigBytes); err == nil {
		t.Fatal("expected verification failure for tampered manifest payload")
	}

	// Injection E: Sibling alteration in Merkle proof
	proof3, _ := digest.GenerateProof(tree, 3)
	if len(proof3.Path) > 0 {
		tamperedProof := *proof3
		corruptedPath := make([]domain.MerkleProofStep, len(proof3.Path))
		copy(corruptedPath, proof3.Path)
		corruptedPath[0].Hash = "corrupted-sibling-hash"
		tamperedProof.Path = corruptedPath
		if digest.VerifyInclusionProof(&tamperedProof, manifest.RootDigest, env.MasterKey) {
			t.Fatal("expected verification failure for corrupted sibling proof step")
		}
	}

	// Injection F: Wrong root digest
	proof4, _ := digest.GenerateProof(tree, 4)
	if digest.VerifyInclusionProof(proof4, "fake-root-digest-0000000000000000", env.MasterKey) {
		t.Fatal("expected verification failure for fake root digest")
	}
}

// TestTier5_Crypto_KeyDerivationIsolation tests that derived keys for different snapshots
// cannot decrypt each other's chunks even if chunks have identical content.
func TestTier5_Crypto_KeyDerivationIsolation(t *testing.T) {
	masterKey := make([]byte, 32)
	io.ReadFull(rand.Reader, masterKey)

	encSnap1, err := aead.NewWithDerivation(masterKey, "snap-2026-01-01")
	if err != nil {
		t.Fatal(err)
	}
	encSnap2, err := aead.NewWithDerivation(masterKey, "snap-2026-01-02")
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("identical database page across snapshot versions")
	chunkID := "chunk-shared-id"

	ct1, err := encSnap1.EncryptChunk(context.Background(), chunkID, data)
	if err != nil {
		t.Fatal(err)
	}

	// Snap2 key MUST NOT decrypt Snap1 ciphertext
	if _, err := encSnap2.DecryptChunk(context.Background(), chunkID, ct1); err == nil {
		t.Fatal("snapshot 2 key successfully decrypted snapshot 1 chunk ciphertext!")
	}
}
