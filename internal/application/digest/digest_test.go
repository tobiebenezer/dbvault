package digest

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/dbvault/dbvault/internal/application/chunking"
	"github.com/dbvault/dbvault/internal/domain"
)

func TestChunkID_DeterminismAndDomainSeparation(t *testing.T) {
	key := []byte("dedup-key-32-bytes-entropy-ok!!!")
	data1 := []byte("page content alpha")
	data2 := []byte("page content beta")

	id1 := ChunkID(key, 4096, data1)
	id1Again := ChunkID(key, 4096, data1)
	id2 := ChunkID(key, 4096, data2)
	id1DifferentPageSize := ChunkID(key, 8192, data1)

	if id1 != id1Again {
		t.Fatalf("ChunkID must be strictly deterministic")
	}
	if id1 == id2 {
		t.Fatalf("different data must produce different chunk IDs")
	}
	if id1 == id1DifferentPageSize {
		t.Fatalf("different page size must produce different chunk ID due to domain separation")
	}
	if len(id1) != 64 {
		t.Fatalf("expected 64-char hex SHA256 string, got %d", len(id1))
	}
}

func TestRootDigest_Properties(t *testing.T) {
	key := []byte("dedup-key-32-bytes-entropy-ok!!!")
	c1 := chunking.PlainChunk{Index: 0, Sequence: 0, PlaintextSize: 10, Data: []byte("0123456789")}
	c2 := chunking.PlainChunk{Index: 1, Sequence: 1, PlaintextSize: 10, Data: []byte("abcdefghij")}
	id1 := ChunkID(key, 4096, c1.Data)
	id2 := ChunkID(key, 4096, c2.Data)

	// 1. Deterministic
	r1 := RootDigest(key, []chunking.PlainChunk{c1, c2}, []string{id1, id2}, 20, 4096)
	r2 := RootDigest(key, []chunking.PlainChunk{c1, c2}, []string{id1, id2}, 20, 4096)
	if r1 != r2 {
		t.Fatalf("RootDigest must be strictly deterministic")
	}

	// 2. Order sensitivity
	rReversed := RootDigest(key, []chunking.PlainChunk{c2, c1}, []string{id2, id1}, 20, 4096)
	if r1 == rReversed {
		t.Fatalf("RootDigest must be order sensitive")
	}

	// 3. Single-bit change sensitivity
	c1Mod := chunking.PlainChunk{Index: 0, Sequence: 0, PlaintextSize: 10, Data: []byte("0123456788")}
	id1Mod := ChunkID(key, 4096, c1Mod.Data)
	rMod := RootDigest(key, []chunking.PlainChunk{c1Mod, c2}, []string{id1Mod, id2}, 20, 4096)
	if r1 == rMod {
		t.Fatalf("RootDigest must change on modified data")
	}

	// 4. Empty set
	rEmpty := RootDigest(key, []chunking.PlainChunk{}, []string{}, 0, 4096)
	if len(rEmpty) != 64 {
		t.Fatalf("expected valid 64-char hex digest for empty set, got %s", rEmpty)
	}
}

func TestMerkleTree_EmptyAndSingleLeaf(t *testing.T) {
	key := []byte("merkle-secret-key-32-bytes-test!")

	// 1. Empty tree
	emptyTree, err := BuildTree(key, nil)
	if err != nil {
		t.Fatalf("empty tree creation failed: %v", err)
	}
	if emptyTree.LeafCount != 0 || emptyTree.RootHash == "" {
		t.Fatalf("invalid empty tree: %+v", emptyTree)
	}

	// Generating proof for empty tree must fail
	if _, err := GenerateProof(emptyTree, 0); !errors.Is(err, domain.ErrEmptyMerkleTree) {
		t.Fatalf("expected ErrEmptyMerkleTree, got %v", err)
	}

	// 2. Single leaf tree
	leafData := [][]byte{[]byte("sole database chunk")}
	singleTree, err := BuildTree(key, leafData)
	if err != nil {
		t.Fatalf("single leaf tree creation failed: %v", err)
	}
	if singleTree.LeafCount != 1 || singleTree.RootHash == "" {
		t.Fatalf("invalid single leaf tree: %+v", singleTree)
	}

	proof, err := GenerateProof(singleTree, 0)
	if err != nil {
		t.Fatalf("failed to generate proof for single leaf: %v", err)
	}
	if len(proof.Path) != 0 {
		t.Fatalf("expected 0 sibling steps for single-leaf tree, got %d", len(proof.Path))
	}
	if !VerifyInclusionProof(proof, singleTree.RootHash, key) {
		t.Fatal("single leaf proof verification failed")
	}
}

func TestMerkleTree_EvenAndOddLeafCounts(t *testing.T) {
	key := []byte("merkle-secret-key-32-bytes-test!")

	testCounts := []int{2, 3, 4, 5, 7, 8, 15, 16, 31, 32}

	for _, count := range testCounts {
		t.Run(fmt.Sprintf("Leaves_%d", count), func(t *testing.T) {
			leaves := make([][]byte, count)
			for i := 0; i < count; i++ {
				leaves[i] = []byte(fmt.Sprintf("chunk-page-data-%04d", i))
			}

			tree, err := BuildTree(key, leaves)
			if err != nil {
				t.Fatalf("failed to build tree with %d leaves: %v", count, err)
			}
			if tree.LeafCount != count {
				t.Fatalf("expected %d leaves, got %d", count, tree.LeafCount)
			}
			if tree.RootHash == "" {
				t.Fatal("root hash cannot be empty")
			}

			// Generate and verify inclusion proofs for EVERY leaf in the tree
			for i := 0; i < count; i++ {
				proof, err := GenerateProof(tree, i)
				if err != nil {
					t.Fatalf("failed to generate proof for leaf %d: %v", i, err)
				}
				if proof.LeafIndex != i {
					t.Fatalf("expected leaf index %d, got %d", i, proof.LeafIndex)
				}
				if proof.RootHash != tree.RootHash {
					t.Fatalf("proof root hash mismatch: %s vs %s", proof.RootHash, tree.RootHash)
				}

				// Valid proof verification
				if !VerifyInclusionProof(proof, tree.RootHash, key) {
					t.Fatalf("proof verification failed for leaf %d (count %d)", i, count)
				}

				// Proof using struct method directly
				if !proof.Verify(tree.RootHash, key) {
					t.Fatalf("proof.Verify failed for leaf %d", i)
				}

				// Verification with wrong root must fail
				if VerifyInclusionProof(proof, "bad-root-hash", key) {
					t.Fatalf("expected verification failure for bad root hash on leaf %d", i)
				}

				// Tampered leaf hash must fail
				tamperedProof := *proof
				tamperedProof.LeafHash = HashLeaf(key, []byte("tampered data"))
				if VerifyInclusionProof(&tamperedProof, tree.RootHash, key) {
					t.Fatalf("expected verification failure for tampered leaf hash on leaf %d", i)
				}
			}

			// Out of bounds leaf index
			if _, err := GenerateProof(tree, count); !errors.Is(err, domain.ErrLeafOutOfBounds) {
				t.Fatalf("expected ErrLeafOutOfBounds for index %d, got %v", count, err)
			}
			if _, err := GenerateProof(tree, -1); !errors.Is(err, domain.ErrLeafOutOfBounds) {
				t.Fatalf("expected ErrLeafOutOfBounds for negative index, got %v", err)
			}
		})
	}
}

func TestMerkleTree_FromChunks(t *testing.T) {
	key := []byte("merkle-secret-key-32-bytes-test!")
	chunks := []chunking.PlainChunk{
		{Index: 0, Sequence: 0, PlaintextSize: 4096, Data: bytes.Repeat([]byte{1}, 4096)},
		{Index: 1, Sequence: 1, PlaintextSize: 4096, Data: bytes.Repeat([]byte{2}, 4096)},
		{Index: 2, Sequence: 2, PlaintextSize: 4096, Data: bytes.Repeat([]byte{3}, 4096)},
	}

	tree, err := BuildTreeFromChunks(key, 4096, chunks)
	if err != nil {
		t.Fatalf("BuildTreeFromChunks failed: %v", err)
	}

	if tree.LeafCount != 3 {
		t.Fatalf("expected 3 leaves, got %d", tree.LeafCount)
	}
	if tree.LogicalSize != 3*4096 {
		t.Fatalf("expected logical size 12288, got %d", tree.LogicalSize)
	}

	for i := 0; i < 3; i++ {
		proof, err := GenerateProof(tree, i)
		if err != nil {
			t.Fatalf("proof gen failed for chunk %d: %v", i, err)
		}
		if !VerifyInclusionProof(proof, tree.RootHash, key) {
			t.Fatalf("proof verify failed for chunk %d", i)
		}
	}
}

func TestMerkleTree_DeterminismAcrossInstances(t *testing.T) {
	key := make([]byte, 32)
	io.ReadFull(rand.Reader, key)

	leafData := make([][]byte, 9)
	for i := range leafData {
		leafData[i] = []byte(fmt.Sprintf("deterministic-leaf-%d", i))
	}

	tree1, err := BuildTree(key, leafData)
	if err != nil {
		t.Fatal(err)
	}

	tree2, err := BuildTree(key, leafData)
	if err != nil {
		t.Fatal(err)
	}

	if tree1.RootHash != tree2.RootHash {
		t.Fatalf("tree root hashes must match across instances: %s vs %s", tree1.RootHash, tree2.RootHash)
	}
}
