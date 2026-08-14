package release

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
)

type Artifact struct {
	Name   string `json:"name"`
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	Format    string     `json:"format"`
	Version   string     `json:"version"`
	Artifacts []Artifact `json:"artifacts"`
	KeyID     string     `json:"key_id"`
	Signature string     `json:"signature"`
}

func Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func Sign(m Manifest, private ed25519.PrivateKey) (Manifest, error) {
	m.Format = "dbvault-release-manifest"
	m.Signature = ""
	payload, err := json.Marshal(m)
	if err != nil {
		return m, err
	}
	m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, payload))
	return m, nil
}

func Verify(m Manifest, public ed25519.PublicKey) error {
	if m.Format != "dbvault-release-manifest" {
		return errors.New("invalid release manifest format")
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return err
	}
	m.Signature = ""
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if !ed25519.Verify(public, payload, sig) {
		return errors.New("invalid release manifest signature")
	}
	return nil
}

func VerifyArtifact(a Artifact, content []byte) error {
	if Digest(content) != a.SHA256 {
		return errors.New("artifact checksum mismatch")
	}
	if a.Size != 0 && a.Size != int64(len(content)) {
		return errors.New("artifact size mismatch")
	}
	return nil
}
