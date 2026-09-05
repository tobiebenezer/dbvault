package digest

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/dbvault/dbvault/internal/domain"
)

// TestMerkleTree_Adversarial_ExhaustiveProofVerification tests tree construction and
// inclusion proof generation/verification across various power-of-two, odd, even, and prime leaf counts.
func TestMerkleTree_Adversarial_ExhaustiveProofVerification(t *testing.T) {
	key := []byte("secret-merkle-adversarial-key-32")

	treeSizes := []int{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 16, 17, 23, 31, 32, 33,
		63, 64, 65, 100, 127, 128, 129, 255, 256, 257, 500, 512, 513,
	}

	for _, size := range treeSizes {
		t.Run(fmt.Sprintf("Size_%d", size), func(t *testing.T) {
			leaves := make([][]byte, size)
			for i := 0; i < size; i++ {
				leaves[i] = []byte(fmt.Sprintf("database-chunk-payload-leaf-%d-size-%d", i, size))
			}

			tree, err := BuildTree(key, leaves)
			if err != nil {
				t.Fatalf("failed to build tree of size %d: %v", size, err)
			}
			if tree.LeafCount != size {
				t.Fatalf("expected LeafCount %d, got %d", size, tree.LeafCount)
			}
			if tree.RootHash == "" {
				t.Fatal("expected non-empty RootHash")
			}

			// Verify inclusion proof for every single leaf
			for i := 0; i < size; i++ {
				proof, err := GenerateProof(tree, i)
				if err != nil {
					t.Fatalf("failed to generate proof for leaf %d (size %d): %v", i, size, err)
				}
				if proof.LeafIndex != i {
					t.Fatalf("proof leaf index mismatch: %d != %d", proof.LeafIndex, i)
				}
				if proof.RootHash != tree.RootHash {
					t.Fatalf("proof root hash mismatch: %s != %s", proof.RootHash, tree.RootHash)
				}

				// 1. Valid verification must succeed
				if !VerifyInclusionProof(proof, tree.RootHash, key) {
					t.Fatalf("valid proof verification failed for leaf %d in tree of size %d", i, size)
				}
				if !proof.Verify(tree.RootHash, key) {
					t.Fatalf("proof.Verify method failed for leaf %d", i)
				}

				// 2. Verification with wrong root must fail
				if VerifyInclusionProof(proof, "invalid_root_digest_0000000000000000", key) {
					t.Fatalf("verification succeeded with wrong root for leaf %d", i)
				}

				// 3. Verification with tampered leaf hash must fail
				tamperedLeafProof := *proof
				tamperedLeafProof.LeafHash = HashLeaf(key, []byte("adversarially-corrupted-chunk"))
				if VerifyInclusionProof(&tamperedLeafProof, tree.RootHash, key) {
					t.Fatalf("verification succeeded with tampered leaf hash for leaf %d", i)
				}

				// 4. Verification with corrupted sibling hashes or inverted positions
				for stepIdx := range proof.Path {
					// Tamper sibling hash
					corruptedPath := make([]domain.MerkleProofStep, len(proof.Path))
					copy(corruptedPath, proof.Path)
					corruptedPath[stepIdx].Hash = "bad_sibling_hash_abcdef1234567890"

					tamperedStepProof := *proof
					tamperedStepProof.Path = corruptedPath
					if VerifyInclusionProof(&tamperedStepProof, tree.RootHash, key) {
						t.Fatalf("verification succeeded with corrupted sibling hash at step %d for leaf %d", stepIdx, i)
					}

					// Invert sibling position (left <-> right)
					invertedPath := make([]domain.MerkleProofStep, len(proof.Path))
					copy(invertedPath, proof.Path)
					if invertedPath[stepIdx].Position == domain.ProofPositionLeft {
						invertedPath[stepIdx].Position = domain.ProofPositionRight
					} else {
						invertedPath[stepIdx].Position = domain.ProofPositionLeft
					}

					tamperedPosProof := *proof
					tamperedPosProof.Path = invertedPath
					if VerifyInclusionProof(&tamperedPosProof, tree.RootHash, key) {
						t.Fatalf("verification succeeded with inverted sibling position at step %d for leaf %d", stepIdx, i)
					}
				}
			}

			// Out of bounds proof requests
			for _, badIdx := range []int{-100, -1, size, size + 1, size + 1000} {
				if _, err := GenerateProof(tree, badIdx); !errors.Is(err, domain.ErrLeafOutOfBounds) {
					t.Fatalf("expected ErrLeafOutOfBounds for index %d on size %d, got %v", badIdx, size, err)
				}
			}
		})
	}
}

// TestMerkleTree_Adversarial_KeyBindingAndHMAC tests that HMAC key changes invalidate proof verification.
func TestMerkleTree_Adversarial_KeyBindingAndHMAC(t *testing.T) {
	key1 := []byte("merkle-secret-key-1-32-bytes!!!")
	key2 := []byte("merkle-secret-key-2-32-bytes!!!")

	leaves := [][]byte{
		[]byte("chunk-alpha"),
		[]byte("chunk-beta"),
		[]byte("chunk-gamma"),
	}

	tree1, err := BuildTree(key1, leaves)
	if err != nil {
		t.Fatal(err)
	}

	tree2, err := BuildTree(key2, leaves)
	if err != nil {
		t.Fatal(err)
	}

	// Different keys MUST yield different root hashes for identical leaves
	if tree1.RootHash == tree2.RootHash {
		t.Fatalf("different HMAC keys must produce different root hashes")
	}

	proof1, err := GenerateProof(tree1, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Verifying proof1 with key2 MUST fail
	if VerifyInclusionProof(proof1, tree1.RootHash, key2) {
		t.Fatal("proof verified with incorrect HMAC key!")
	}

	// Verifying unkeyed tree proof
	unkeyedTree, _ := BuildTree(nil, leaves)
	if unkeyedTree.RootHash == tree1.RootHash {
		t.Fatal("unkeyed tree root hash collided with keyed tree root hash")
	}

	unkeyedProof, _ := GenerateProof(unkeyedTree, 1)
	if !VerifyInclusionProof(unkeyedProof, unkeyedTree.RootHash, nil) {
		t.Fatal("unkeyed proof verification failed")
	}
	if VerifyInclusionProof(unkeyedProof, unkeyedTree.RootHash, key1) {
		t.Fatal("unkeyed proof verified against keyed expectation!")
	}
}

// TestMerkleTree_Adversarial_DomainSeparationSecondPreimage tests that domain separation
// (0x00 for leaves vs 0x01 for internal nodes) prevents second-preimage attacks.
func TestMerkleTree_Adversarial_DomainSeparationSecondPreimage(t *testing.T) {
	key := []byte("merkle-domain-separation-key-32!")

	leafData := []byte("test-leaf-payload")
	leafHash := HashLeaf(key, leafData)

	// An internal node computed over the same payload without domain prefix MUST NOT equal leafHash
	internalHash := HashInternalNode(key, string(leafData), "")
	if leafHash == internalHash {
		t.Fatal("domain separation collision: leaf hash equals internal node hash")
	}
}

// TestMerkleTree_Adversarial_OrderSensitivity tests that reordering leaves changes root hash.
func TestMerkleTree_Adversarial_OrderSensitivity(t *testing.T) {
	key := []byte("merkle-order-sensitivity-key-32!")

	leavesA := [][]byte{[]byte("A"), []byte("B"), []byte("C"), []byte("D")}
	leavesB := [][]byte{[]byte("B"), []byte("A"), []byte("C"), []byte("D")}
	leavesC := [][]byte{[]byte("A"), []byte("B"), []byte("D"), []byte("C")}

	treeA, _ := BuildTree(key, leavesA)
	treeB, _ := BuildTree(key, leavesB)
	treeC, _ := BuildTree(key, leavesC)

	if treeA.RootHash == treeB.RootHash {
		t.Fatal("root hash did not change when swapping leaf 0 and 1")
	}
	if treeA.RootHash == treeC.RootHash {
		t.Fatal("root hash did not change when swapping leaf 2 and 3")
	}
}

// TestMerkleTree_Adversarial_TreeFromHashesEquivalence tests that BuildTreeFromHashes
// produces the identical root and proofs as BuildTree.
func TestMerkleTree_Adversarial_TreeFromHashesEquivalence(t *testing.T) {
	key := []byte("merkle-equiv-key-32-bytes-long!!")
	rawLeaves := make([][]byte, 19)
	hashes := make([]string, 19)

	for i := range rawLeaves {
		rawLeaves[i] = []byte(fmt.Sprintf("raw-chunk-content-%03d", i))
		hashes[i] = HashLeaf(key, rawLeaves[i])
	}

	tree1, err := BuildTree(key, rawLeaves)
	if err != nil {
		t.Fatal(err)
	}

	tree2, err := BuildTreeFromHashes(key, hashes)
	if err != nil {
		t.Fatal(err)
	}

	if tree1.RootHash != tree2.RootHash {
		t.Fatalf("BuildTree vs BuildTreeFromHashes root mismatch: %s vs %s", tree1.RootHash, tree2.RootHash)
	}

	// Verify all proofs from tree2
	for i := 0; i < 19; i++ {
		proof, err := GenerateProof(tree2, i)
		if err != nil {
			t.Fatalf("failed proof gen on tree2 leaf %d: %v", i, err)
		}
		if !VerifyInclusionProof(proof, tree1.RootHash, key) {
			t.Fatalf("tree2 proof failed to verify against tree1 root for leaf %d", i)
		}
	}
}

// TestMerkleTree_Adversarial_RandomFuzzing performs randomized fuzz tests
// with unpredictable chunk sizes and keys.
func TestMerkleTree_Adversarial_RandomFuzzing(t *testing.T) {
	for trial := 0; trial < 10; trial++ {
		key := make([]byte, 32)
		io.ReadFull(rand.Reader, key)

		numLeaves := 1 + trial*7 // 1, 8, 15, 22, 29, 36, ...
		leaves := make([][]byte, numLeaves)
		for i := 0; i < numLeaves; i++ {
			sz := 16 + (i*31)%512
			leaves[i] = make([]byte, sz)
			io.ReadFull(rand.Reader, leaves[i])
		}

		tree, err := BuildTree(key, leaves)
		if err != nil {
			t.Fatalf("trial %d: BuildTree failed: %v", trial, err)
		}

		// Spot check 3 random leaves or all if < 3
		indicesToCheck := []int{0, numLeaves / 2, numLeaves - 1}
		for _, idx := range indicesToCheck {
			proof, err := GenerateProof(tree, idx)
			if err != nil {
				t.Fatalf("trial %d: proof gen failed for idx %d: %v", trial, idx, err)
			}
			if !VerifyInclusionProof(proof, tree.RootHash, key) {
				t.Fatalf("trial %d: proof verify failed for idx %d", trial, idx)
			}
		}
	}
}
