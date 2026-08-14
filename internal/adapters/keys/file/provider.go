package file

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

type Provider struct{ Directory string }
type Material struct {
	ID     string
	Secret []byte
}
type SigningPair struct {
	ID      string
	Private ed25519.PrivateKey
	Public  ed25519.PublicKey
}

func New(dir string) Provider { return Provider{Directory: dir} }
func (p Provider) Generate(repositoryID, keyID string) error {
	if err := os.MkdirAll(p.Directory, 0700); err != nil {
		return err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	if err := writeB64(filepath.Join(p.Directory, repositoryID+"-"+keyID+".key"), key); err != nil {
		return err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := writeB64(filepath.Join(p.Directory, repositoryID+"-signing-private.key"), priv); err != nil {
		return err
	}
	return writeB64(filepath.Join(p.Directory, repositoryID+"-signing-public.key"), pub)
}
func (p Provider) EncryptionKey(repositoryID, keyID string) (Material, error) {
	b, err := readB64(filepath.Join(p.Directory, repositoryID+"-"+keyID+".key"))
	if err != nil {
		return Material{}, err
	}
	if len(b) != 32 {
		return Material{}, fmt.Errorf("encryption key must be 32 bytes")
	}
	return Material{ID: keyID, Secret: b}, nil
}
func (p Provider) Signing(repositoryID string) (SigningPair, error) {
	priv, err := readB64(filepath.Join(p.Directory, repositoryID+"-signing-private.key"))
	if err != nil {
		return SigningPair{}, err
	}
	pub, err := readB64(filepath.Join(p.Directory, repositoryID+"-signing-public.key"))
	if err != nil {
		return SigningPair{}, err
	}
	if len(priv) != ed25519.PrivateKeySize {
		return SigningPair{}, fmt.Errorf("invalid ed25519 private key length")
	}
	if len(pub) != ed25519.PublicKeySize {
		return SigningPair{}, fmt.Errorf("invalid ed25519 public key length")
	}
	privKey := ed25519.PrivateKey(priv)
	if !bytesEqual(privKey.Public().(ed25519.PublicKey), ed25519.PublicKey(pub)) {
		return SigningPair{}, fmt.Errorf("signing public key does not match private key")
	}
	return SigningPair{ID: "signing-1", Private: privKey, Public: ed25519.PublicKey(pub)}, nil
}
func writeB64(path string, b []byte) error {
	return os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(b)+"\n"), 0600)
}
func readB64(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("key file %s must not be a symlink", path)
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("key file %s permissions are too broad", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(string(bytesTrimSpace(b)))
}
func bytesTrimSpace(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ' || b[len(b)-1] == '\t') {
		b = b[:len(b)-1]
	}
	for len(b) > 0 && (b[0] == '\n' || b[0] == '\r' || b[0] == ' ' || b[0] == '\t') {
		b = b[1:]
	}
	return b
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
