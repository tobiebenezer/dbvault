package ed25519

import (
	"crypto/ed25519"
	"errors"
	"testing"
)

func TestEd25519_GenerateAndSignVerify(t *testing.T) {
	signer, err := Generate("key-001")
	if err != nil {
		t.Fatalf("failed to generate signer: %v", err)
	}

	if signer.KeyID != "key-001" {
		t.Fatalf("expected key ID key-001, got %s", signer.KeyID)
	}
	if len(signer.Public) != ed25519.PublicKeySize {
		t.Fatalf("expected 32-byte public key, got %d", len(signer.Public))
	}
	if len(signer.Private) != ed25519.PrivateKeySize {
		t.Fatalf("expected 64-byte private key, got %d", len(signer.Private))
	}

	payload := []byte(`{"snapshot_id":"snap_test_001","root_digest":"abcdef"}`)
	sigBytes, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	// Verify using same signer instance
	if err := signer.Verify(payload, sigBytes); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	// Verify using public key only instance (e.g. read-only verifier / emergency restore)
	verifier := New("key-001", nil, signer.Public)
	if err := verifier.Verify(payload, sigBytes); err != nil {
		t.Fatalf("verifier with public key only failed: %v", err)
	}

	// Verify using standalone helper
	ok, err := VerifyKey(signer.Public, payload, sigBytes)
	if err != nil || !ok {
		t.Fatalf("VerifyKey helper failed: ok=%v, err=%v", ok, err)
	}
}

func TestEd25519_TamperedPayloadAndSignature(t *testing.T) {
	signer, err := Generate("key-002")
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte(`{"snapshot_id":"snap_valid"}`)
	sigBytes, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Tampered payload
	tamperedPayload := []byte(`{"snapshot_id":"snap_tampered"}`)
	if err := signer.Verify(tamperedPayload, sigBytes); err == nil {
		t.Fatal("expected verification failure for tampered payload, got nil")
	}

	// 2. Tampered signature envelope
	tamperedSig := make([]byte, len(sigBytes))
	copy(tamperedSig, sigBytes)
	tamperedSig[len(tamperedSig)-10] ^= 0xFF
	if err := signer.Verify(payload, tamperedSig); err == nil {
		t.Fatal("expected verification failure for tampered signature, got nil")
	}

	// 3. Different key verification fails
	otherSigner, _ := Generate("key-other")
	otherVerifier := New("key-other", nil, otherSigner.Public)
	if err := otherVerifier.Verify(payload, sigBytes); err == nil {
		t.Fatal("expected verification failure when verified with different public key")
	}
}

func TestEd25519_MalformedPublicKeyGuards(t *testing.T) {
	payload := []byte("test payload")
	sig := []byte(`{"format":"dbvault-manifest-signature","format_version":1,"algorithm":"ed25519","key_id":"k1","signature":"AQIDBA=="}`)

	// Short public key (e.g. 16 bytes) - MUST NOT PANIC
	shortPub := make([]byte, 16)
	shortSigner := Signer{Public: shortPub}
	err := shortSigner.Verify(payload, sig)
	if !errors.Is(err, ErrInvalidKeyLength) {
		t.Fatalf("expected ErrInvalidKeyLength for 16-byte key, got: %v", err)
	}

	// Empty public key with nil private key - MUST NOT PANIC
	emptySigner := Signer{}
	err = emptySigner.Verify(payload, sig)
	if !errors.Is(err, ErrInvalidKeyLength) {
		t.Fatalf("expected ErrInvalidKeyLength for empty key, got: %v", err)
	}

	// Overly long public key (e.g. 64 bytes) - MUST NOT PANIC
	longPub := make([]byte, 64)
	longSigner := Signer{Public: longPub}
	err = longSigner.Verify(payload, sig)
	if !errors.Is(err, ErrInvalidKeyLength) {
		t.Fatalf("expected ErrInvalidKeyLength for 64-byte key, got: %v", err)
	}

	// Standalone VerifyKey guard
	ok, err := VerifyKey(shortPub, payload, sig)
	if ok || !errors.Is(err, ErrInvalidKeyLength) {
		t.Fatalf("expected VerifyKey to return false, ErrInvalidKeyLength for malformed key, got ok=%v, err=%v", ok, err)
	}
}

func TestEd25519_EnvelopeValidation(t *testing.T) {
	signer, _ := Generate("key-env")
	payload := []byte("test payload")

	// Invalid format
	badFormatSig := []byte(`{"format":"invalid-format","format_version":1,"algorithm":"ed25519","key_id":"k1","signature":"AQIDBA=="}`)
	if err := signer.Verify(payload, badFormatSig); err == nil {
		t.Fatal("expected error on invalid format")
	}

	// Invalid algorithm
	badAlgoSig := []byte(`{"format":"dbvault-manifest-signature","format_version":1,"algorithm":"rsa4096","key_id":"k1","signature":"AQIDBA=="}`)
	if err := signer.Verify(payload, badAlgoSig); err == nil {
		t.Fatal("expected error on invalid algorithm")
	}

	// Invalid JSON
	if err := signer.Verify(payload, []byte("not-json")); err == nil {
		t.Fatal("expected error on non-json signature")
	}

	// Missing private key for signing
	emptyPrivateSigner := Signer{KeyID: "k-nopriv"}
	if _, err := emptyPrivateSigner.Sign(payload); !errors.Is(err, ErrMissingPrivateKey) {
		t.Fatalf("expected ErrMissingPrivateKey, got: %v", err)
	}
}
