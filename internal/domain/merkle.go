package domain

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var (
	ErrEmptyMerkleTree    = errors.New("cannot build merkle tree from empty leaf set")
	ErrInvalidMerkleProof = errors.New("merkle inclusion proof verification failed")
	ErrLeafOutOfBounds    = errors.New("merkle leaf index out of bounds")
)

type MerkleProofPosition string

const (
	ProofPositionLeft  MerkleProofPosition = "left"
	ProofPositionRight MerkleProofPosition = "right"
)

// MerkleProofStep represents a single sibling hash and its position relative to the current branch.
type MerkleProofStep struct {
	Hash     string              `json:"hash"`
	Position MerkleProofPosition `json:"position"` // "left" if sibling is left, "right" if sibling is right
}

// MerkleProof contains the cryptographic inclusion proof for a specific leaf in the Merkle tree.
type MerkleProof struct {
	LeafIndex int               `json:"leaf_index"`
	LeafHash  string            `json:"leaf_hash"`
	Path      []MerkleProofStep `json:"path"`
	RootHash  string            `json:"root_hash"`
}

// Verify validates this inclusion proof against an expected root hash.
func (p *MerkleProof) Verify(expectedRoot string, key []byte) bool {
	if p == nil || p.LeafHash == "" || (p.RootHash == "" && expectedRoot == "") {
		return false
	}
	targetRoot := expectedRoot
	if targetRoot == "" {
		targetRoot = p.RootHash
	}

	currentHash := p.LeafHash
	for _, step := range p.Path {
		var combined []byte
		if step.Position == ProofPositionLeft {
			// Sibling is on the left: hash(0x01 || sibling || current)
			combined = append([]byte{0x01}, []byte(step.Hash)...)
			combined = append(combined, []byte(currentHash)...)
		} else {
			// Sibling is on the right: hash(0x01 || current || sibling)
			combined = append([]byte{0x01}, []byte(currentHash)...)
			combined = append(combined, []byte(step.Hash)...)
		}

		if len(key) > 0 {
			h := hmac.New(sha256.New, key)
			h.Write(combined)
			currentHash = hex.EncodeToString(h.Sum(nil))
		} else {
			h := sha256.Sum256(combined)
			currentHash = hex.EncodeToString(h[:])
		}
	}

	return hmac.Equal([]byte(currentHash), []byte(targetRoot)) || bytes.Equal([]byte(currentHash), []byte(targetRoot))
}

// MerkleNode represents an individual internal node or leaf in the binary Merkle tree.
type MerkleNode struct {
	Hash      string      `json:"hash"`
	IsLeaf    bool        `json:"is_leaf"`
	LeafIndex int         `json:"leaf_index,omitempty"`
	ChunkID   string      `json:"chunk_id,omitempty"`
	Left      *MerkleNode `json:"-"`
	Right     *MerkleNode `json:"-"`
}

// MerkleTree represents a complete deterministic hierarchical binary Merkle tree.
type MerkleTree struct {
	Root        *MerkleNode   `json:"root"`
	RootHash    string        `json:"root_hash"`
	Leaves      []*MerkleNode `json:"leaves"`
	LeafCount   int           `json:"leaf_count"`
	PageSize    int           `json:"page_size,omitempty"`
	LogicalSize int64         `json:"logical_size,omitempty"`
}
