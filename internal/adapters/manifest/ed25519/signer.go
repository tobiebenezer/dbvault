package ed25519

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type Signer struct {
	KeyID   string
	Private ed25519.PrivateKey
	Public  ed25519.PublicKey
}
type Envelope struct {
	Format        string `json:"format"`
	FormatVersion int    `json:"format_version"`
	Algorithm     string `json:"algorithm"`
	KeyID         string `json:"key_id"`
	Signature     string `json:"signature"`
}

func Generate(keyID string) (Signer, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Signer{}, err
	}
	return Signer{KeyID: keyID, Private: priv, Public: pub}, nil
}
func New(keyID string, priv ed25519.PrivateKey, pub ed25519.PublicKey) Signer {
	return Signer{KeyID: keyID, Private: priv, Public: pub}
}
func (s Signer) Sign(payload []byte) ([]byte, error) {
	if len(s.Private) == 0 {
		return nil, fmt.Errorf("missing private signing key")
	}
	env := Envelope{Format: "dbvault-manifest-signature", FormatVersion: 1, Algorithm: "ed25519", KeyID: s.KeyID, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(s.Private, payload))}
	return json.MarshalIndent(env, "", "  ")
}
func (s Signer) Verify(payload, sig []byte) error {
	var env Envelope
	if err := json.Unmarshal(sig, &env); err != nil {
		return err
	}
	raw, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil {
		return err
	}
	pub := s.Public
	if len(pub) == 0 && len(s.Private) > 0 {
		pub = s.Private.Public().(ed25519.PublicKey)
	}
	if !ed25519.Verify(pub, payload, raw) {
		return fmt.Errorf("invalid manifest signature")
	}
	return nil
}
