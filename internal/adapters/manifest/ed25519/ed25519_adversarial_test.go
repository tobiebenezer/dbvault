package ed25519

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
)

// TestEd25519_Adversarial_MalformedPublicKeys tests all non-32 byte public key lengths
// to ensure Verify and VerifyKey strictly reject them with ErrInvalidKeyLength and never panic.
func TestEd25519_Adversarial_MalformedPublicKeys(t *testing.T) {
	signer, err := Generate("key-adv-001")
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte(`{"manifest_id":"man-001","root_hash":"0123456789abcdef"}`)
	sigBytes, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}

	invalidLengths := []int{0, 1, 2, 7, 8, 15, 16, 24, 31, 33, 48, 64, 128, 256}
	for _, sz := range invalidLengths {
		pub := make([]byte, sz)
		io.ReadFull(rand.Reader, pub)

		// 1. Signer struct verification
		vSigner := Signer{KeyID: "test", Public: ed25519.PublicKey(pub)}
		err := vSigner.Verify(payload, sigBytes)
		if !errors.Is(err, ErrInvalidKeyLength) {
			t.Fatalf("Signer.Verify with %d-byte public key: expected ErrInvalidKeyLength, got %v", sz, err)
		}

		// 2. VerifyKey helper
		ok, err := VerifyKey(pub, payload, sigBytes)
		if ok || !errors.Is(err, ErrInvalidKeyLength) {
			t.Fatalf("VerifyKey with %d-byte public key: expected false, ErrInvalidKeyLength, got ok=%v, err=%v", sz, ok, err)
		}
	}

	// Nil public key
	vSigner := Signer{KeyID: "test", Public: nil}
	if err := vSigner.Verify(payload, sigBytes); !errors.Is(err, ErrInvalidKeyLength) {
		t.Fatalf("Signer.Verify with nil public key: expected ErrInvalidKeyLength, got %v", err)
	}
	if ok, err := VerifyKey(nil, payload, sigBytes); ok || !errors.Is(err, ErrInvalidKeyLength) {
		t.Fatalf("VerifyKey with nil public key: expected false, ErrInvalidKeyLength, got ok=%v, err=%v", ok, err)
	}
}

// TestEd25519_Adversarial_SignatureBitFlips exhaustively flips every bit in the 64-byte Ed25519 signature
// to ensure every bit corruption triggers ErrInvalidSignature.
func TestEd25519_Adversarial_SignatureBitFlips(t *testing.T) {
	signer, err := Generate("key-adv-002")
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte(`{"snapshot":"snap_production_2026_08_30","database":"prod_pg"}`)
	sigBytes, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}

	// Unmarshal envelope to get raw signature bytes
	var env Envelope
	if err := json.Unmarshal(sigBytes, &env); err != nil {
		t.Fatal(err)
	}
	rawSig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil || len(rawSig) != ed25519.SignatureSize {
		t.Fatalf("expected %d-byte raw signature, got %d", ed25519.SignatureSize, len(rawSig))
	}

	// Flip every bit of the 64-byte raw signature
	corruptedRaw := make([]byte, len(rawSig))
	for byteIdx := 0; byteIdx < len(rawSig); byteIdx++ {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			copy(corruptedRaw, rawSig)
			corruptedRaw[byteIdx] ^= 1 << bitIdx

			corruptedEnv := env
			corruptedEnv.Signature = base64.StdEncoding.EncodeToString(corruptedRaw)
			corruptedSigBytes, _ := json.Marshal(corruptedEnv)

			err := signer.Verify(payload, corruptedSigBytes)
			if err == nil {
				t.Fatalf("FATAL: Ed25519 accepted tampered signature! Byte %d, Bit %d flipped", byteIdx, bitIdx)
			}
			if !errors.Is(err, ErrInvalidSignature) {
				t.Fatalf("expected ErrInvalidSignature, got: %v", err)
			}
		}
	}
}

// TestEd25519_Adversarial_WrongPublicKey tests verification against completely different public keys.
func TestEd25519_Adversarial_WrongPublicKey(t *testing.T) {
	signer1, _ := Generate("key-1")
	signer2, _ := Generate("key-2")

	payload := []byte(`{"secret_state":"verified_snapshot_merkle_root"}`)
	sig1, err := signer1.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}

	// Verifying with signer2's key MUST fail
	verifier2 := New("key-2", nil, signer2.Public)
	err = verifier2.Verify(payload, sig1)
	if err == nil {
		t.Fatal("expected verification failure with different public key")
	}
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}

	// All-zero 32-byte public key MUST fail
	zeroPub := make([]byte, 32)
	ok, err := VerifyKey(zeroPub, payload, sig1)
	if ok || err == nil {
		t.Fatal("expected verification failure for all-zero public key")
	}
}

// TestEd25519_Adversarial_EnvelopeTampering tests various corruptions of the signature envelope structure.
func TestEd25519_Adversarial_EnvelopeTampering(t *testing.T) {
	signer, _ := Generate("key-env-adv")
	payload := []byte("critical manifest payload")
	validSig, _ := signer.Sign(payload)

	var baseEnv Envelope
	_ = json.Unmarshal(validSig, &baseEnv)

	tamperCases := []struct {
		name     string
		modifier func(env *Envelope) []byte
	}{
		{
			name: "UnsupportedFormat",
			modifier: func(e *Envelope) []byte {
				clone := *e
				clone.Format = "bogus-signature-format"
				b, _ := json.Marshal(clone)
				return b
			},
		},
		{
			name: "UnsupportedAlgorithm_RSA",
			modifier: func(e *Envelope) []byte {
				clone := *e
				clone.Algorithm = "rsa4096"
				b, _ := json.Marshal(clone)
				return b
			},
		},
		{
			name: "UnsupportedAlgorithm_ECDSA",
			modifier: func(e *Envelope) []byte {
				clone := *e
				clone.Algorithm = "ecdsa-p256"
				b, _ := json.Marshal(clone)
				return b
			},
		},
		{
			name: "MalformedBase64_InvalidChars",
			modifier: func(e *Envelope) []byte {
				clone := *e
				clone.Signature = "ThisIsNotValidBase64!@#$%%^&*()_+"
				b, _ := json.Marshal(clone)
				return b
			},
		},
		{
			name: "MalformedBase64_TruncatedLength",
			modifier: func(e *Envelope) []byte {
				clone := *e
				clone.Signature = base64.StdEncoding.EncodeToString([]byte("short-sig"))
				b, _ := json.Marshal(clone)
				return b
			},
		},
		{
			name: "TruncatedJSON",
			modifier: func(e *Envelope) []byte {
				return []byte(`{"format":"dbvault-manifest-signature","signature":`)
			},
		},
		{
			name: "EmptyJSON",
			modifier: func(e *Envelope) []byte {
				return []byte(`{}`)
			},
		},
		{
			name: "NullJSON",
			modifier: func(e *Envelope) []byte {
				return []byte(`null`)
			},
		},
		{
			name: "RawStringNotJSON",
			modifier: func(e *Envelope) []byte {
				return []byte(`raw-text-not-json`)
			},
		},
	}

	for _, tc := range tamperCases {
		t.Run(tc.name, func(t *testing.T) {
			corruptedSig := tc.modifier(&baseEnv)
			err := signer.Verify(payload, corruptedSig)
			if err == nil {
				t.Fatalf("expected error for case %s, got nil", tc.name)
			}
		})
	}
}

// TestEd25519_Adversarial_PayloadAlterations tests subtle alterations to the payload
// to ensure exact byte-for-byte authenticity.
func TestEd25519_Adversarial_PayloadAlterations(t *testing.T) {
	signer, _ := Generate("key-payload")
	payload := []byte(`{"manifest":"v1","sources":["pg","mysql"],"chunks":100}`)
	sig, _ := signer.Sign(payload)

	alterations := [][]byte{
		[]byte(`{"manifest":"v1","sources":["pg","mysql"],"chunks":101}`),
		[]byte(`{"manifest":"v1","sources":["pg","mysql"],"chunks":100} `), // trailing space
		[]byte(` {"manifest":"v1","sources":["pg","mysql"],"chunks":100}`), // leading space
		[]byte(`{"manifest":"v2","sources":["pg","mysql"],"chunks":100}`),
		payload[:len(payload)-1], // truncated 1 byte
		append(payload, 0x00),    // null byte appended
		[]byte{},                 // empty payload
	}

	for i, alt := range alterations {
		t.Run(fmt.Sprintf("Alteration_%d", i), func(t *testing.T) {
			err := signer.Verify(alt, sig)
			if err == nil {
				t.Fatalf("expected verification failure for altered payload variant %d", i)
			}
		})
	}
}

// TestEd25519_Adversarial_ConcurrentSignVerify tests heavy concurrent signing and verification.
func TestEd25519_Adversarial_ConcurrentSignVerify(t *testing.T) {
	signer, err := Generate("key-concurrent")
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 15
	const iterations = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(gID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				payload := []byte(fmt.Sprintf("concurrent-payload-%d-%d", gID, i))
				sig, err := signer.Sign(payload)
				if err != nil {
					t.Errorf("sign failed: %v", err)
					return
				}
				if err := signer.Verify(payload, sig); err != nil {
					t.Errorf("verify failed: %v", err)
					return
				}
			}
		}(g)
	}

	wg.Wait()
}
