package aead

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"testing"
)

func TestHKDFKeyDerivationAndEnvelope(t *testing.T) {
	rootKey := bytes.Repeat([]byte{0x42}, 32)

	// 1. Test deterministic derivation for different versions
	k1 := DeriveKey(rootKey, "1")
	k2 := DeriveKey(rootKey, "2")
	k1Again := DeriveKey(rootKey, "1")

	if bytes.Equal(k1, k2) {
		t.Fatalf("k1 and k2 must be distinct")
	}
	if !bytes.Equal(k1, k1Again) {
		t.Fatalf("HKDF derivation must be strictly deterministic")
	}

	// 2. Test encryption and decryption with versioned derivation
	enc1, err := NewWithDerivation(rootKey, "1")
	if err != nil {
		t.Fatal(err)
	}
	enc2, err := NewWithDerivation(rootKey, "2")
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("secret database record data payload")
	sealed1, err := enc1.EncryptChunk(context.Background(), "chunk-abc", plaintext)
	if err != nil {
		t.Fatal(err)
	}

	// Correct version decrypts
	decrypted, err := enc1.DecryptChunk(context.Background(), "chunk-abc", sealed1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted payload mismatch")
	}

	// Different version fails decryption
	if _, err := enc2.DecryptChunk(context.Background(), "chunk-abc", sealed1); err == nil {
		t.Fatalf("expected decryption failure with mismatched version key")
	}

	// 3. Test Envelope Encryption
	envPlaintext := []byte(`{"keyring_version":2,"keys":{"v1":"...","v2":"..."}}`)
	envSealed, err := EncryptEnvelope(rootKey, envPlaintext)
	if err != nil {
		t.Fatal(err)
	}
	envDecrypted, err := DecryptEnvelope(rootKey, envSealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(envDecrypted, envPlaintext) {
		t.Fatalf("envelope decryption mismatch")
	}
}

func TestAEAD_LegacyDerivationFallback(t *testing.T) {
	rootKey := bytes.Repeat([]byte{0x42}, 32)
	enc, err := NewWithDerivation(rootKey, "1")
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("payload sealed with the pre-HKDF key derivation")

	legacyMac := hmac.New(sha256.New, rootKey)
	legacyMac.Write([]byte("dbvault-chunk-key-derivation-v:1"))
	legacyEnc, err := New(legacyMac.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}

	legacySealed, err := legacyEnc.EncryptChunk(context.Background(), "chunk-legacy", plaintext)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := enc.DecryptChunk(context.Background(), "chunk-legacy", legacySealed)
	if err != nil {
		t.Fatalf("legacy-sealed chunk must decrypt via fallback: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("legacy fallback payload mismatch")
	}

	envPlaintext := []byte(`{"keyring_version":1,"keys":{"v1":"..."}}`)
	legacyEnvMac := hmac.New(sha256.New, rootKey)
	legacyEnvMac.Write([]byte("dbvault-chunk-key-derivation-v:keyring-envelope"))
	legacyEnvEnc, err := New(legacyEnvMac.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}
	legacyEnvSealed, err := legacyEnvEnc.EncryptChunk(context.Background(), "envelope", envPlaintext)
	if err != nil {
		t.Fatal(err)
	}
	envDecrypted, err := DecryptEnvelope(rootKey, legacyEnvSealed)
	if err != nil {
		t.Fatalf("legacy keyring envelope must decrypt via fallback: %v", err)
	}
	if !bytes.Equal(envDecrypted, envPlaintext) {
		t.Fatalf("legacy envelope fallback payload mismatch")
	}

	corrupted := make([]byte, len(legacySealed))
	copy(corrupted, legacySealed)
	corrupted[len(corrupted)-3] ^= 0xFF
	if _, err := enc.DecryptChunk(context.Background(), "chunk-legacy", corrupted); err == nil {
		t.Fatal("expected failure for corrupted legacy ciphertext under both derivations")
	}

	shortRoot := bytes.Repeat([]byte{0x11}, 16)
	shortEnc, err := NewWithDerivation(shortRoot, "1")
	if err != nil {
		t.Fatal(err)
	}
	shortSealed, err := shortEnc.EncryptChunk(context.Background(), "chunk-short", plaintext)
	if err != nil {
		t.Fatal(err)
	}
	shortDecrypted, err := shortEnc.DecryptChunk(context.Background(), "chunk-short", shortSealed)
	if err != nil {
		t.Fatalf("sub-32-byte root key round trip must keep working: %v", err)
	}
	if !bytes.Equal(shortDecrypted, plaintext) {
		t.Fatalf("sub-32-byte root key payload mismatch")
	}
}

func TestAEAD_KeyValidation(t *testing.T) {
	// Invalid key lengths (< 32 bytes or > 32 bytes)
	shortKey := make([]byte, 16)
	if _, err := New(shortKey); err == nil {
		t.Fatal("expected error for 16-byte key, got nil")
	}

	longKey := make([]byte, 64)
	if _, err := New(longKey); err == nil {
		t.Fatal("expected error for 64-byte key, got nil")
	}

	if _, err := NewWithDerivation(nil, "1"); err == nil {
		t.Fatal("expected error for empty root key, got nil")
	}
}

func TestAEAD_PayloadSizes(t *testing.T) {
	key := make([]byte, 32)
	io.ReadFull(rand.Reader, key)
	enc, err := New(key)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	testCases := []struct {
		name string
		size int
	}{
		{"ZeroByte", 0},
		{"SingleByte", 1},
		{"PageSize4K", 4096},
		{"PageSize8K", 8192},
		{"PageSize16K", 16384},
		{"MultiMegabyte", 2 * 1024 * 1024},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, tc.size)
			if tc.size > 0 {
				io.ReadFull(rand.Reader, data)
			}
			chunkID := "chunk-" + tc.name
			ciphertext, err := enc.EncryptChunk(context.Background(), chunkID, data)
			if err != nil {
				t.Fatalf("encryption failed for %s: %v", tc.name, err)
			}

			// Minimum ciphertext length: 4-byte header + 12-byte nonce + 16-byte GCM tag = 32 bytes
			if len(ciphertext) < 32 {
				t.Fatalf("ciphertext too short: %d bytes", len(ciphertext))
			}

			decrypted, err := enc.DecryptChunk(context.Background(), chunkID, ciphertext)
			if err != nil {
				t.Fatalf("decryption failed for %s: %v", tc.name, err)
			}
			if !bytes.Equal(decrypted, data) {
				t.Fatalf("payload mismatch for %s", tc.name)
			}
		})
	}
}

func TestAEAD_CorruptedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	io.ReadFull(rand.Reader, key)
	enc, err := New(key)
	if err != nil {
		t.Fatal(err)
	}

	chunkID := "chunk-corrupt-test"
	data := []byte("integrity sensitive database page payload")
	ciphertext, err := enc.EncryptChunk(context.Background(), chunkID, data)
	if err != nil {
		t.Fatal(err)
	}

	// Bit-flip in the middle of ciphertext
	corrupted := make([]byte, len(ciphertext))
	copy(corrupted, ciphertext)
	corrupted[len(corrupted)-5] ^= 0xFF

	if _, err := enc.DecryptChunk(context.Background(), chunkID, corrupted); err == nil {
		t.Fatal("expected decryption failure for corrupted ciphertext payload, got nil")
	}

	// Bit-flip in the nonce
	corruptedNonce := make([]byte, len(ciphertext))
	copy(corruptedNonce, ciphertext)
	corruptedNonce[5] ^= 0x01

	if _, err := enc.DecryptChunk(context.Background(), chunkID, corruptedNonce); err == nil {
		t.Fatal("expected decryption failure for corrupted nonce, got nil")
	}
}

func TestAEAD_TruncatedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	io.ReadFull(rand.Reader, key)
	enc, err := New(key)
	if err != nil {
		t.Fatal(err)
	}

	chunkID := "chunk-trunc"
	data := []byte("payload to truncate")
	ciphertext, err := enc.EncryptChunk(context.Background(), chunkID, data)
	if err != nil {
		t.Fatal(err)
	}

	// < 4 bytes
	if _, err := enc.DecryptChunk(context.Background(), chunkID, ciphertext[:3]); err == nil {
		t.Fatal("expected failure on < 4 bytes")
	}

	// Truncated nonce
	if _, err := enc.DecryptChunk(context.Background(), chunkID, ciphertext[:10]); err == nil {
		t.Fatal("expected failure on truncated nonce")
	}

	// Truncated GCM auth tag
	if _, err := enc.DecryptChunk(context.Background(), chunkID, ciphertext[:len(ciphertext)-5]); err == nil {
		t.Fatal("expected failure on truncated auth tag")
	}
}

func TestAEAD_AADMismatch(t *testing.T) {
	key := make([]byte, 32)
	io.ReadFull(rand.Reader, key)
	enc, err := New(key)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("database page chunk")
	ciphertext, err := enc.EncryptChunk(context.Background(), "chunk-001", data)
	if err != nil {
		t.Fatal(err)
	}

	// Decrypting with wrong chunkID must fail due to AAD binding
	if _, err := enc.DecryptChunk(context.Background(), "chunk-002", ciphertext); err == nil {
		t.Fatal("expected AAD mismatch failure when decrypting with different chunk ID, got nil")
	}
}

func BenchmarkAEAD_EncryptChunk(b *testing.B) {
	key := make([]byte, 32)
	io.ReadFull(rand.Reader, key)
	enc, _ := New(key)
	data := make([]byte, 64*1024) // 64KB chunk
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = enc.EncryptChunk(ctx, "benchmark-chunk", data)
	}
}
