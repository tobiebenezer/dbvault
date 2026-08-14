package local

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSecretStoreRoundTripAndEncryption(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	path := filepath.Join(t.TempDir(), "secrets.json")
	store, err := New(path, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("r2_secret", []byte("very-secret")); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("r2_secret")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "very-secret" {
		t.Fatalf("got %q", got)
	}
	raw, _ := os.ReadFile(path)
	if bytes.Contains(raw, []byte("very-secret")) {
		t.Fatal("plaintext secret persisted")
	}
}
