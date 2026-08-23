package doctor

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

const validConfig = `version: 1
server:
  data_directory: %s
  scratch_directory: %s/scratch
catalogue:
  driver: sqlite
  path: %s/catalogue.sqlite
secret_providers:
  - id: local-files
    driver: file
    file:
      root: %s/keys
destinations:
  - id: local
    driver: filesystem
    filesystem:
      root: %s/repository
repositories:
  - id: repo
    mode: single
    primary:
      destination: local
    encryption:
      key_provider: local-files
      active_key: k1
    signing:
      key_provider: local-files
      key_id: signing-local
    compression:
      algorithm: zstd
      level: 6
      minimum_savings_percent: 5
    chunking:
      sqlite:
        strategy: page-aligned
        target_size: 64KiB
    retention:
      keep_last: 3
sources:
  - id: src
    engine: sqlite
    repository: repo
    enabled: true
    sqlite:
      path: %s/source.db
`

// writeKeyMaterial seeds the file key provider exactly as `dbvault key
// generate` would, so bootstrap can load encryption and signing material.
func writeKeyMaterial(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	enc := make([]byte, 32)
	if _, err := rand.Read(enc); err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"repo-k1.key":              enc,
		"repo-signing-private.key": priv,
		"repo-signing-public.key":  pub,
	}
	for name, raw := range files {
		b64 := base64.StdEncoding.EncodeToString(raw) + "\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b64), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dbvault.yaml")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunBrokenConfigFails(t *testing.T) {
	broken := writeConfig(t, "version: 9\n")
	report, err := Run(context.Background(), broken, discardLogger())
	if err != nil {
		t.Fatalf("Run returns report, not transport error: %v", err)
	}
	if report.Healthy() {
		t.Fatalf("broken config must not be healthy:\n%+v", report.Checks)
	}
	var found bool
	for _, c := range report.Checks {
		if c.Name == "config" && !c.OK() {
			found = true
		}
	}
	if !found {
		t.Fatalf("config check missing or passing: %+v", report.Checks)
	}
}

func TestRunMissingFileFailsFast(t *testing.T) {
	report, err := Run(context.Background(), filepath.Join(t.TempDir(), "nope.yaml"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Healthy() || len(report.Checks) != 1 || report.Checks[0].Name != "config" {
		t.Fatalf("expected single failing config check: %+v", report.Checks)
	}
}

func TestRunValidLocalSetupPasses(t *testing.T) {
	dir := t.TempDir()
	writeKeyMaterial(t, filepath.Join(dir, "keys"))
	cfg := writeConfig(t, strings.ReplaceAll(validConfig, "%s", dir))
	report, err := Run(context.Background(), cfg, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Healthy() {
		t.Fatalf("local filesystem setup should pass:\n%+v", report.Checks)
	}
	want := map[string]bool{"config": false, "catalogue": false, "storage": false, "scratch": false}
	for _, c := range report.Checks {
		want[c.Name] = true
		if !c.OK() {
			t.Errorf("%s unexpectedly failed: %+v", c.Name, c)
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("check %q never ran", name)
		}
	}
}

func TestScratchWatermarkRespected(t *testing.T) {
	dir := t.TempDir()
	if _, err := scratchCheck(dir, 1<<50); err == nil {
		t.Fatal("expected watermark failure with impossible threshold")
	}
}
