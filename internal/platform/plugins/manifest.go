package plugins

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
)

type Permission struct {
	Name      string   `json:"name"`
	Resources []string `json:"resources,omitempty"`
}
type Manifest struct {
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	APIVersion  int          `json:"api_version"`
	Category    string       `json:"category"`
	Executable  string       `json:"executable"`
	Permissions []Permission `json:"permissions"`
	KeyID       string       `json:"key_id"`
	Signature   string       `json:"signature"`
}

func Verify(manifest Manifest, public ed25519.PublicKey) error {
	if manifest.APIVersion != 1 {
		return errors.New("unsupported plugin API")
	}
	if manifest.Name == "" || manifest.Executable == "" {
		return errors.New("invalid plugin manifest")
	}
	sig, err := base64.StdEncoding.DecodeString(manifest.Signature)
	if err != nil {
		return err
	}
	manifest.Signature = ""
	payload, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if !ed25519.Verify(public, payload, sig) {
		return errors.New("invalid plugin signature")
	}
	return nil
}
