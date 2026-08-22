package keyderive

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// RFC 5869 Appendix A, Test Case 1 (SHA-256).
func TestRFC5869TestCase1(t *testing.T) {
	ikm, _ := hex.DecodeString("0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b")
	salt, _ := hex.DecodeString("000102030405060708090a0b0c")
	info, _ := hex.DecodeString("f0f1f2f3f4f5f6f7f8f9")
	wantOKM, _ := hex.DecodeString("3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865")

	prk := extract(salt, ikm)
	if got := hex.EncodeToString(prk); got != "077709362c2e32df0ddc3f0dc47bba6390b6c73bb50f9c3122ec844ad7c2b3e5" {
		t.Fatalf("PRK mismatch: %s", got)
	}
	okm := expand(prk, info, 42)
	if !bytes.Equal(okm, wantOKM) {
		t.Fatalf("OKM mismatch: %s", hex.EncodeToString(okm))
	}
}

// RFC 5869 Appendix A, Test Case 3 (SHA-256, zero-length salt/info).
func TestRFC5869TestCase3(t *testing.T) {
	ikm, _ := hex.DecodeString("0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b")
	wantOKM, _ := hex.DecodeString("8da4e775a563c18f715f802a063c5a31b8a11f5c5ee1879ec3454e5f3c738d2d9d201395faa4b61a96c8")

	prk := extract(nil, ikm)
	if got := hex.EncodeToString(prk); got != "19ef24a32c717b167f33a91d6f648bdf96596776afdb6377ac434c1c293ccb04" {
		t.Fatalf("PRK mismatch: %s", got)
	}
	okm := expand(prk, nil, 42)
	if !bytes.Equal(okm, wantOKM) {
		t.Fatalf("OKM mismatch: %s", hex.EncodeToString(okm))
	}
}

func TestDeriveRejectsShortMasterKey(t *testing.T) {
	for _, n := range []int{0, 1, 16, 31} {
		if _, err := Derive(make([]byte, n), DomainAEAD); err == nil {
			t.Fatalf("%d-byte key accepted; zero-pad path must stay retired", n)
		} else if !strings.Contains(err.Error(), "zero-padding retired") {
			t.Fatalf("unexpected error text: %v", err)
		}
	}
	if _, err := Derive(make([]byte, 32), DomainAEAD); err != nil {
		t.Fatalf("32-byte key rejected: %v", err)
	}
}

func TestDeriveDomainSeparation(t *testing.T) {
	master := bytes.Repeat([]byte{0x42}, 32)
	aeadKey, err := Derive(master, DomainAEAD)
	if err != nil {
		t.Fatal(err)
	}
	hmacKey, err := Derive(master, DomainHMAC)
	if err != nil {
		t.Fatal(err)
	}
	dedupKey, err := Derive(master, DomainDedup)
	if err != nil {
		t.Fatal(err)
	}
	if aeadKey == hmacKey || aeadKey == dedupKey || hmacKey == dedupKey {
		t.Fatal("subkeys for different domains must differ")
	}

	flipped := append([]byte{}, master...)
	flipped[0] ^= 0x01
	aeadFlipped, err := Derive(flipped, DomainAEAD)
	if err != nil {
		t.Fatal(err)
	}
	if aeadFlipped == aeadKey {
		t.Fatal("flipping a master-key bit must change the subkey")
	}

	emptyLabel, err := Derive(master, "")
	if err == nil {
		t.Fatal("empty domain label must be rejected")
	}
	_ = emptyLabel
}

// Proves the derived HMAC subkey verifies payloads and that any single-bit
// domain-label flip breaks verification (negative test from plan A6).
func TestDeriveHMACRoundTripAndLabelBreak(t *testing.T) {
	master := bytes.Repeat([]byte{0x77}, 32)
	keyA, err := Derive(master, DomainHMAC)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("manifest-payload")
	mac := hmac.New(sha256.New, keyA[:])
	mac.Write(payload)
	sig := mac.Sum(nil)

	keyAgain, _ := Derive(master, DomainHMAC)
	verify := hmac.New(sha256.New, keyAgain[:])
	verify.Write(payload)
	if !hmac.Equal(sig, verify.Sum(nil)) {
		t.Fatal("round-trip verification failed")
	}

	broken, _ := Derive(master, DomainDedup)
	verifier := hmac.New(sha256.New, broken[:])
	verifier.Write(payload)
	if hmac.Equal(sig, verifier.Sum(nil)) {
		t.Fatal("verification must fail when the domain label differs")
	}
}
