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

	"github.com/dbvault/dbvault/internal/platform/keyderive"
)

type Encryptor struct {
	key     []byte
	aead    cipher.AEAD
	rootKey []byte
	version string
}

// DeriveKey derives a version-scoped encryption key deterministically from a root master key using HKDF-HMAC-SHA256.
func DeriveKey(rootKey []byte, version string) []byte {
	if version == "" {
		version = "1"
	}
	domain := "dbvault/aead-v1:" + version
	if len(rootKey) >= 32 {
		derived, err := keyderive.Derive(rootKey, domain)
		if err == nil {
			out := make([]byte, 32)
			copy(out, derived[:])
			return out
		}
	}
	return legacyDeriveKey(rootKey, version)
}

// legacyDeriveKey reproduces the pre-HKDF derivation so that historical
// snapshots and keyring envelopes written before the HKDF switch remain
// decryptable via a fallback retry in DecryptChunk.
func legacyDeriveKey(rootKey []byte, version string) []byte {
	if version == "" {
		version = "1"
	}
	h := hmac.New(sha256.New, rootKey)
	h.Write([]byte("dbvault-chunk-key-derivation-v:" + version))
	return h.Sum(nil)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func New(key []byte) (*Encryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("AES-256-GCM key must be 32 bytes")
	}
	g, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	return &Encryptor{key: keyCopy, aead: g}, nil
}

// NewWithDerivation creates an encryptor using a key derived from the root key for a specific snapshot version.
func NewWithDerivation(rootKey []byte, version string) (*Encryptor, error) {
	if len(rootKey) == 0 {
		return nil, fmt.Errorf("root master key cannot be empty")
	}
	e, err := New(DeriveKey(rootKey, version))
	if err != nil {
		return nil, err
	}
	rootCopy := make([]byte, len(rootKey))
	copy(rootCopy, rootKey)
	e.rootKey = rootCopy
	e.version = version
	return e, nil
}

func (e *Encryptor) EncryptChunk(ctx context.Context, chunkID string, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	aad := []byte("dbvault-chunk-v1:" + chunkID)
	sealed := e.aead.Seal(nil, nonce, plaintext, aad)
	out := make([]byte, 4+len(nonce)+len(sealed))
	binary.BigEndian.PutUint32(out[:4], uint32(len(nonce)))
	copy(out[4:], nonce)
	copy(out[4+len(nonce):], sealed)
	return out, nil
}

func (e *Encryptor) DecryptChunk(ctx context.Context, chunkID string, ciphertext []byte) ([]byte, error) {
	plaintext, err := openWith(e.aead, chunkID, ciphertext)
	if err == nil {
		return plaintext, nil
	}
	if len(e.rootKey) >= 32 {
		if legacyAEAD, legacyErr := newGCM(legacyDeriveKey(e.rootKey, e.version)); legacyErr == nil {
			if legacyPlain, legacyOpenErr := openWith(legacyAEAD, chunkID, ciphertext); legacyOpenErr == nil {
				return legacyPlain, nil
			}
		}
	}
	return nil, err
}

func openWith(g cipher.AEAD, chunkID string, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 4 {
		return nil, fmt.Errorf("ciphertext too short")
	}
	n := int(binary.BigEndian.Uint32(ciphertext[:4]))
	if n <= 0 || n != g.NonceSize() || len(ciphertext) < 4+n+g.Overhead() {
		return nil, fmt.Errorf("ciphertext nonce or payload truncated")
	}
	nonce := ciphertext[4 : 4+n]
	body := ciphertext[4+n:]
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
	enc, err := NewWithDerivation(rootKey, "keyring-envelope")
	if err != nil {
		return nil, err
	}
	return enc.DecryptChunk(context.Background(), "envelope", ciphertext)
}
