package aead

import (
	"bytes"
	"context"
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
