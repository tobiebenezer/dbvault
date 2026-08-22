// Package keyderive splits a repository master key into independent
// purpose-scoped subkeys (RFC 5869 HKDF-SHA256) so that no single key
// material serves encryption, signing and dedup at once.
//
// Implemented by hand on top of crypto/hmac + crypto/sha256 only, keeping
// the restricted dependency-free build green.
package keyderive

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
)

// Domain labels provide HKDF `info` separation between subkey purposes.
const (
	DomainAEAD  = "dbvault/aead-v1"
	DomainHMAC  = "dbvault/hmac-v1"
	DomainDedup = "dbvault/dedup-v1"

	keyLen = 32
)

// Derive returns the 32-byte subkey for the given domain label.
// Master keys shorter than 32 bytes are rejected: legacy zero-padding is
// no longer performed for any newly derived material.
func Derive(master []byte, domain string) ([32]byte, error) {
	var out [32]byte
	if len(master) < keyLen {
		return out, fmt.Errorf("master key is %d bytes; at least %d bytes of entropy required (zero-padding retired)", len(master), keyLen)
	}
	if domain == "" {
		return out, errors.New("derivation domain label must not be empty")
	}
	salt := Fingerprint(master)
	prk := extract(salt, master)
	okm := expand(prk, []byte(domain), keyLen)
	copy(out[:], okm)
	return out, nil
}

// Fingerprint returns the hex SHA-256 fingerprint prefix used as the HKDF salt.
func Fingerprint(master []byte) []byte {
	h := sha256.Sum256(master)
	return h[:]
}

func extract(salt, ikm []byte) []byte {
	if len(salt) == 0 {
		salt = make([]byte, sha256.Size)
	}
	mac := hmac.New(sha256.New, salt)
	mac.Write(ikm)
	return mac.Sum(nil)
}

func expand(prk, info []byte, length int) []byte {
	okm := make([]byte, 0, length)
	var previous []byte
	counter := byte(1)
	for len(okm) < length {
		mac := hmac.New(sha256.New, prk)
		mac.Write(previous)
		mac.Write(info)
		mac.Write([]byte{counter})
		previous = mac.Sum(nil)
		okm = append(okm, previous...)
		counter++
	}
	return okm[:length]
}
