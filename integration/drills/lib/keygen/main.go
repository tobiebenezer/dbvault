// Command keygen writes key material in the exact on-disk layout expected by
// internal/adapters/keys/file: a base64-encoded 32-byte encryption key at
// <dir>/<repo>-<activeKey>.key plus a matching ed25519 signing pair. This is
// drill-harness plumbing only; real deployments should provision key material
// through their own secret management.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: keygen <keys-dir> <repository-id> <active-key>")
		os.Exit(2)
	}
	dir, repoID, activeKey := os.Args[1], os.Args[2], os.Args[3]
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fail(err)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		fail(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fail(err)
	}
	write(filepath.Join(dir, repoID+"-"+activeKey+".key"), raw)
	write(filepath.Join(dir, repoID+"-signing-private.key"), priv)
	write(filepath.Join(dir, repoID+"-signing-public.key"), pub)
	fmt.Printf("[drill] key material written under %s\n", dir)
}

func write(path string, b []byte) {
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(b)+"\n"), 0o600); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "keygen:", err)
	os.Exit(1)
}
