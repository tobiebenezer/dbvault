package digest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/dbvault/dbvault/internal/application/chunking"
	"github.com/dbvault/dbvault/internal/domain"
)

const (
	DomainLeafPrefix     byte = 0x00
	DomainInternalPrefix byte = 0x01
)

// HashLeaf computes the deterministic leaf hash over input data with domain separation (0x00).
func HashLeaf(key []byte, data []byte) string {
	payload := append([]byte{DomainLeafPrefix}, data...)
	if len(key) > 0 {
		h := hmac.New(sha256.New, key)
		h.Write(payload)
		return hex.EncodeToString(h.Sum(nil))
	}
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}

// HashInternalNode computes the deterministic internal parent node hash over two children (0x01 || left || right).
func HashInternalNode(key []byte, leftHash, rightHash string) string {
	payload := append([]byte{DomainInternalPrefix}, []byte(leftHash)...)
	payload = append(payload, []byte(rightHash)...)
	if len(key) > 0 {
		h := hmac.New(sha256.New, key)
		h.Write(payload)
		return hex.EncodeToString(h.Sum(nil))
	}
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}

// BuildTree builds a binary Merkle tree from raw leaf byte slices.
func BuildTree(key []byte, leafData [][]byte) (*domain.MerkleTree, error) {
	if len(leafData) == 0 {
		emptyHash := HashLeaf(key, []byte{})
		return &domain.MerkleTree{
			Root: &domain.MerkleNode{
				Hash:   emptyHash,
				IsLeaf: true,
			},
			RootHash:  emptyHash,
			Leaves:    nil,
			LeafCount: 0,
		}, nil
	}

	leafNodes := make([]*domain.MerkleNode, len(leafData))
	for i, data := range leafData {
		h := HashLeaf(key, data)
		leafNodes[i] = &domain.MerkleNode{
			Hash:      h,
			IsLeaf:    true,
			LeafIndex: i,
		}
	}

	return buildTreeFromNodes(key, leafNodes)
}

// BuildTreeFromHashes constructs a binary Merkle tree from pre-computed leaf hashes / chunk IDs.
func BuildTreeFromHashes(key []byte, leafHashes []string) (*domain.MerkleTree, error) {
	if len(leafHashes) == 0 {
		emptyHash := HashLeaf(key, []byte{})
		return &domain.MerkleTree{
			Root: &domain.MerkleNode{
				Hash:   emptyHash,
				IsLeaf: true,
			},
			RootHash:  emptyHash,
			Leaves:    nil,
			LeafCount: 0,
		}, nil
	}

	leafNodes := make([]*domain.MerkleNode, len(leafHashes))
	for i, h := range leafHashes {
		leafNodes[i] = &domain.MerkleNode{
			Hash:      h,
			IsLeaf:    true,
			LeafIndex: i,
			ChunkID:   h,
		}
	}

	return buildTreeFromNodes(key, leafNodes)
}

// BuildTreeFromChunks constructs a binary Merkle tree from database page chunks.
func BuildTreeFromChunks(key []byte, pageSize int, chunks []chunking.PlainChunk) (*domain.MerkleTree, error) {
	if len(chunks) == 0 {
		emptyHash := HashLeaf(key, []byte{})
		return &domain.MerkleTree{
			Root: &domain.MerkleNode{
				Hash:   emptyHash,
				IsLeaf: true,
			},
			RootHash:  emptyHash,
			Leaves:    nil,
			LeafCount: 0,
			PageSize:  pageSize,
		}, nil
	}

	leafNodes := make([]*domain.MerkleNode, len(chunks))
	var totalLogicalSize int64
	for i, ch := range chunks {
		cid := ChunkID(key, pageSize, ch.Data)
		leafNodes[i] = &domain.MerkleNode{
			Hash:      cid,
			IsLeaf:    true,
			LeafIndex: i,
			ChunkID:   cid,
		}
		totalLogicalSize += ch.PlaintextSize
	}

	tree, err := buildTreeFromNodes(key, leafNodes)
	if err != nil {
		return nil, err
	}
	tree.PageSize = pageSize
	tree.LogicalSize = totalLogicalSize
	return tree, nil
}

// buildTreeFromNodes constructs the hierarchical tree levels from bottom leaves up to the root.
func buildTreeFromNodes(key []byte, leaves []*domain.MerkleNode) (*domain.MerkleTree, error) {
	if len(leaves) == 0 {
		return nil, domain.ErrEmptyMerkleTree
	}

	currentLevel := leaves
	for len(currentLevel) > 1 {
		var nextLevel []*domain.MerkleNode
		for i := 0; i < len(currentLevel); i += 2 {
			if i+1 < len(currentLevel) {
				left := currentLevel[i]
				right := currentLevel[i+1]
				parentHash := HashInternalNode(key, left.Hash, right.Hash)
				parent := &domain.MerkleNode{
					Hash:   parentHash,
					IsLeaf: false,
					Left:   left,
					Right:  right,
				}
				nextLevel = append(nextLevel, parent)
			} else {
				// Odd node promotion: promote the single child to the next level
				nextLevel = append(nextLevel, currentLevel[i])
			}
		}
		currentLevel = nextLevel
	}

	rootNode := currentLevel[0]
	return &domain.MerkleTree{
		Root:      rootNode,
		RootHash:  rootNode.Hash,
		Leaves:    leaves,
		LeafCount: len(leaves),
	}, nil
}

// GenerateProof generates an O(log N) inclusion proof for the leaf at leafIndex.
func GenerateProof(tree *domain.MerkleTree, leafIndex int) (*domain.MerkleProof, error) {
	if tree == nil || tree.LeafCount == 0 {
		return nil, domain.ErrEmptyMerkleTree
	}
	if leafIndex < 0 || leafIndex >= tree.LeafCount {
		return nil, fmt.Errorf("%w: index %d, total %d", domain.ErrLeafOutOfBounds, leafIndex, tree.LeafCount)
	}

	targetLeaf := tree.Leaves[leafIndex]
	var path []domain.MerkleProofStep

	// Collect siblings by searching from root down to targetLeaf
	found := collectProofPath(tree.Root, targetLeaf, &path)
	if !found {
		return nil, fmt.Errorf("leaf at index %d not found in tree", leafIndex)
	}

	// Reverse path so it goes bottom-up (from leaf to root)
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return &domain.MerkleProof{
		LeafIndex: leafIndex,
		LeafHash:  targetLeaf.Hash,
		Path:      path,
		RootHash:  tree.RootHash,
	}, nil
}

func collectProofPath(current *domain.MerkleNode, target *domain.MerkleNode, path *[]domain.MerkleProofStep) bool {
	if current == nil {
		return false
	}
	if current == target {
		return true
	}
	if current.IsLeaf {
		return false
	}

	// Check if target is in the left subtree
	if current.Left != nil && nodeContains(current.Left, target) {
		if current.Right != nil {
			*path = append(*path, domain.MerkleProofStep{
				Hash:     current.Right.Hash,
				Position: domain.ProofPositionRight, // Sibling is on right
			})
		}
		return collectProofPath(current.Left, target, path)
	}

	// Check if target is in the right subtree
	if current.Right != nil && nodeContains(current.Right, target) {
		if current.Left != nil {
			*path = append(*path, domain.MerkleProofStep{
				Hash:     current.Left.Hash,
				Position: domain.ProofPositionLeft, // Sibling is on left
			})
		}
		return collectProofPath(current.Right, target, path)
	}

	return false
}

func nodeContains(parent *domain.MerkleNode, target *domain.MerkleNode) bool {
	if parent == nil {
		return false
	}
	if parent == target {
		return true
	}
	if parent.IsLeaf {
		return false
	}
	return nodeContains(parent.Left, target) || nodeContains(parent.Right, target)
}

// VerifyInclusionProof verifies an inclusion proof against an expected root hash.
func VerifyInclusionProof(proof *domain.MerkleProof, expectedRoot string, key []byte) bool {
	if proof == nil {
		return false
	}
	return proof.Verify(expectedRoot, key)
}
