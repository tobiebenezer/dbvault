package aead

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"testing"
)

// TestAEAD_Adversarial_BitFlipExhaustive tests flipping every bit in a ciphertext
// across different payload sizes to ensure every single bit modification is detected and rejected.
func TestAEAD_Adversarial_BitFlipExhaustive(t *testing.T) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatal(err)
	}
	enc, err := New(key)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	payloads := [][]byte{
		[]byte("A"),
		[]byte("short secret database record"),
		bytes.Repeat([]byte{0x5A}, 512),
		bytes.Repeat([]byte{0xA5}, 4096),
	}

	ctx := context.Background()

	for _, payload := range payloads {
		chunkID := fmt.Sprintf("chunk-%d", len(payload))
		ciphertext, err := enc.EncryptChunk(ctx, chunkID, payload)
		if err != nil {
			t.Fatalf("encrypt chunk failed: %v", err)
		}

		// Verify original decrypts
		decrypted, err := enc.DecryptChunk(ctx, chunkID, ciphertext)
		if err != nil || !bytes.Equal(decrypted, payload) {
			t.Fatalf("baseline decryption failed for payload len %d", len(payload))
		}

		// Exhaustively flip every bit of the ciphertext
		corrupted := make([]byte, len(ciphertext))
		for byteIdx := 0; byteIdx < len(ciphertext); byteIdx++ {
			for bitIdx := 0; bitIdx < 8; bitIdx++ {
				copy(corrupted, ciphertext)
				corrupted[byteIdx] ^= 1 << bitIdx

				_, err := enc.DecryptChunk(ctx, chunkID, corrupted)
				if err == nil {
					t.Fatalf("FATAL: DecryptChunk accepted tampered ciphertext! Byte %d, Bit %d flipped (payload size %d)", byteIdx, bitIdx, len(payload))
				}
			}
		}
	}
}

// TestAEAD_Adversarial_TruncationAndBoundaries tests all possible truncation prefix lengths
// from 0 bytes up to full length - 1.
func TestAEAD_Adversarial_TruncationAndBoundaries(t *testing.T) {
	key := make([]byte, 32)
	io.ReadFull(rand.Reader, key)
	enc, err := New(key)
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("boundary truncation test payload 1234567890")
	chunkID := "chunk-trunc-adversarial"
	ciphertext, err := enc.EncryptChunk(context.Background(), chunkID, payload)
	if err != nil {
		t.Fatal(err)
	}

	// Test all truncations from 0 to len(ciphertext)-1
	for cutLen := 0; cutLen < len(ciphertext); cutLen++ {
		truncated := ciphertext[:cutLen]
		_, err := enc.DecryptChunk(context.Background(), chunkID, truncated)
		if err == nil {
			t.Fatalf("expected error for truncated ciphertext of length %d, got nil", cutLen)
		}
	}
}

// TestAEAD_Adversarial_NonceHeaderTampering tests corrupting the 4-byte nonce length header.
func TestAEAD_Adversarial_NonceHeaderTampering(t *testing.T) {
	key := make([]byte, 32)
	io.ReadFull(rand.Reader, key)
	enc, err := New(key)
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("nonce header corruption test")
	chunkID := "chunk-nonce-tamper"
	ciphertext, err := enc.EncryptChunk(context.Background(), chunkID, payload)
	if err != nil {
		t.Fatal(err)
	}

	invalidNonceLengths := []uint32{0, 1, 8, 11, 13, 16, 32, 64, 100, 1000, 0xFFFFFFFF, 0x80000000}
	for _, fakeLen := range invalidNonceLengths {
		corrupted := make([]byte, len(ciphertext))
		copy(corrupted, ciphertext)
		corrupted[0] = byte(fakeLen >> 24)
		corrupted[1] = byte(fakeLen >> 16)
		corrupted[2] = byte(fakeLen >> 8)
		corrupted[3] = byte(fakeLen)

		_, err := enc.DecryptChunk(context.Background(), chunkID, corrupted)
		if err == nil {
			t.Fatalf("expected error for corrupted nonce length %d, got nil", fakeLen)
		}
	}
}

// TestAEAD_Adversarial_InvalidKeyLengths tests invalid key sizes against New and DeriveKey.
func TestAEAD_Adversarial_InvalidKeyLengths(t *testing.T) {
	invalidSizes := []int{0, 1, 7, 8, 15, 16, 24, 31, 33, 48, 64, 128, 256, 1024}
	for _, sz := range invalidSizes {
		k := make([]byte, sz)
		_, err := New(k)
		if err == nil {
			t.Fatalf("New() should reject key of size %d bytes", sz)
		}
	}

	// Sub-32 byte root keys with DeriveKey should still produce 32-byte keys without panicking
	for _, sz := range []int{1, 8, 16, 31} {
		k := make([]byte, sz)
		derived := DeriveKey(k, "1")
		if len(derived) != 32 {
			t.Fatalf("DeriveKey with %d-byte root key returned %d bytes, expected 32", sz, len(derived))
		}
	}
}

// TestAEAD_Adversarial_AADVariations tests AAD cross-binding and edge case strings.
func TestAEAD_Adversarial_AADVariations(t *testing.T) {
	key := make([]byte, 32)
	io.ReadFull(rand.Reader, key)
	enc, err := New(key)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	payload := []byte("database page chunk sensitive data")

	testAADs := []string{
		"",
		"chunk-1",
		"chunk-2",
		"chunk-10",
		"chunk-01",
		"chunk-1\x00",
		"dbvault-chunk-v1:chunk-1",
		"../path/traversal",
		"unicode-🚀-chunk",
		string(bytes.Repeat([]byte("very-long-aad-chunk-identifier-"), 100)),
	}

	// Encrypt under each AAD
	ciphertexts := make([][]byte, len(testAADs))
	for i, aad := range testAADs {
		ct, err := enc.EncryptChunk(ctx, aad, payload)
		if err != nil {
			t.Fatalf("encrypt failed for AAD %q: %v", aad, err)
		}
		ciphertexts[i] = ct
	}

	// Cross-decrypt: decrypting under any other AAD MUST fail
	for i, aadI := range testAADs {
		for j, aadJ := range testAADs {
			dec, err := enc.DecryptChunk(ctx, aadJ, ciphertexts[i])
			if i == j {
				if err != nil || !bytes.Equal(dec, payload) {
					t.Fatalf("valid decryption failed for AAD %q: %v", aadI, err)
				}
			} else {
				if err == nil {
					t.Fatalf("cross-AAD decryption succeeded! AAD %q decrypted ciphertext from AAD %q", aadJ, aadI)
				}
			}
		}
	}
}

// TestAEAD_Adversarial_ConcurrentStress tests concurrent encryption and decryption
// under heavy multi-goroutine load.
func TestAEAD_Adversarial_ConcurrentStress(t *testing.T) {
	key := make([]byte, 32)
	io.ReadFull(rand.Reader, key)
	enc, err := New(key)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 20
	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(gID int) {
			defer wg.Done()
			ctx := context.Background()
			for iter := 0; iter < iterations; iter++ {
				data := make([]byte, (gID*100)+iter+1)
				chunkID := fmt.Sprintf("chunk-%d-%d", gID, iter)
				ct, err := enc.EncryptChunk(ctx, chunkID, data)
				if err != nil {
					t.Errorf("concurrent encrypt failed: %v", err)
					return
				}
				pt, err := enc.DecryptChunk(ctx, chunkID, ct)
				if err != nil {
					t.Errorf("concurrent decrypt failed: %v", err)
					return
				}
				if !bytes.Equal(pt, data) {
					t.Errorf("concurrent decrypt mismatch")
					return
				}
			}
		}(g)
	}

	wg.Wait()
}
