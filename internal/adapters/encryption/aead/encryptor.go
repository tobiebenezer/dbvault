package aead

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
)

type Encryptor struct{ key []byte }

func New(key []byte) (*Encryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("AES-256-GCM key must be 32 bytes")
	}
	return &Encryptor{key: key}, nil
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
