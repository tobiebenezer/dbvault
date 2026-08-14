package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestReleaseManifestSignatureAndChecksum(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	artifact := Artifact{Name: "dbvault", OS: "linux", Arch: "amd64", SHA256: Digest([]byte("binary")), Size: 6}
	m, err := Sign(Manifest{Version: "1.0.0", Artifacts: []Artifact{artifact}}, priv)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(m, pub); err != nil {
		t.Fatal(err)
	}
	if err := VerifyArtifact(artifact, []byte("binary")); err != nil {
		t.Fatal(err)
	}
	if err := VerifyArtifact(artifact, []byte("tampered")); err == nil {
		t.Fatal("tampered artifact accepted")
	}
}
