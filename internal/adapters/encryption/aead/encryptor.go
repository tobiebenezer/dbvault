package aead

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
)

type Encryptor struct{ key []byte }

// DeriveKey derives a version-scoped encryption key deterministically from a root master key using HKDF-HMAC-SHA256.
func DeriveKey(rootKey []byte, version string) []byte {
	if version == "" {
		version = "1"
	}
	h := hmac.New(sha256.New, rootKey)
	h.Write([]byte("dbvault-chunk-key-derivation-v:" + version))
	return h.Sum(nil)
}

func New(key []byte) (*Encryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("AES-256-GCM key must be 32 bytes")
	}
	return &Encryptor{key: key}, nil
}

// NewWithDerivation creates an encryptor using a key derived from the root key for a specific snapshot version.
func NewWithDerivation(rootKey []byte, version string) (*Encryptor, error) {
	if len(rootKey) == 0 {
		return nil, fmt.Errorf("root master key cannot be empty")
	}
	derived := DeriveKey(rootKey, version)
	return New(derived)
}

func (e *Encryptor) EncryptChunk(ctx context.Context, chunkID string, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	aad := []byte("dbvault-chunk-v1:" + chunkID)
	sealed := g.Seal(nil, nonce, plaintext, aad)
	out := make([]byte, 4+len(nonce)+len(sealed))
	binary.BigEndian.PutUint32(out[:4], uint32(len(nonce)))
	copy(out[4:], nonce)
	copy(out[4+len(nonce):], sealed)
	return out, nil
}

func (e *Encryptor) DecryptChunk(ctx context.Context, chunkID string, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 4 {
		return nil, fmt.Errorf("ciphertext too short")
	}
	n := int(binary.BigEndian.Uint32(ciphertext[:4]))
	if len(ciphertext) < 4+n {
		return nil, fmt.Errorf("ciphertext nonce truncated")
	}
	nonce := ciphertext[4 : 4+n]
	body := ciphertext[4+n:]
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	aad := []byte("dbvault-chunk-v1:" + chunkID)
	return g.Open(nil, nonce, body, aad)
}

// EncryptEnvelope encrypts arbitrary metadata (such as historical keyring records) under the Root Master Key.
func EncryptEnvelope(rootKey []byte, plaintext []byte) ([]byte, error) {
	derived := DeriveKey(rootKey, "keyring-envelope")
	enc, err := New(derived)
	if err != nil {
		return nil, err
	}
	return enc.EncryptChunk(context.Background(), "envelope", plaintext)
}

// DecryptEnvelope decrypts an encrypted metadata envelope using the Root Master Key.
func DecryptEnvelope(rootKey []byte, ciphertext []byte) ([]byte, error) {
	derived := DeriveKey(rootKey, "keyring-envelope")
	enc, err := New(derived)
	if err != nil {
		return nil, err
	}
	return enc.DecryptChunk(context.Background(), "envelope", ciphertext)
}
