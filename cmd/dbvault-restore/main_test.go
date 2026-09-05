package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	zstdc "github.com/dbvault/dbvault/internal/adapters/compression/zstd"
	aead "github.com/dbvault/dbvault/internal/adapters/encryption/aead"
	edsigner "github.com/dbvault/dbvault/internal/adapters/manifest/ed25519"
	"github.com/dbvault/dbvault/internal/application/chunking"
	"github.com/dbvault/dbvault/internal/application/digest"
	mf "github.com/dbvault/dbvault/internal/application/manifest"
)

const (
	testBucket      = "dbvault-test"
	testPageSize    = 4096
	testManifestKey = "src1/snapshots/snap1/manifest.json"
	testKeyID       = "test-active-key"
)

type fixture struct {
	objects     map[string][]byte
	masterHex   string
	pubHex      string
	original    []byte
	snapID      string
	endpointURL string
}

func silenceStdout(t *testing.T, fn func()) {
	t.Helper()
	old := os.Stdout
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("cannot open devnull: %v", err)
	}
	os.Stdout = devnull
	defer func() {
		os.Stdout = old
		_ = devnull.Close()
	}()
	fn()
}

// buildFixture creates a realistic snapshot (chunks -> compress -> encrypt ->
// manifest -> root digest -> sign -> completion record) using the same real
// packages and conventions as internal/application/backup, served by a fake
// S3-compatible HTTP server.
func buildFixture(t *testing.T, mutate func(f *fixture)) *fixture {
	t.Helper()

	master := make([]byte, 32)
	if _, err := rand.Read(master); err != nil {
		t.Fatalf("rand: %v", err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}

	// Two full pages plus a partial page of deterministic pseudo-random data.
	original := make([]byte, testPageSize*2+1000)
	if _, err := rand.Read(original); err != nil {
		t.Fatalf("rand: %v", err)
	}
	copy(original, "SQLite format 3\x00")

	compressor := zstdc.New(0, 0)
	enc, err := aead.New(master)
	if err != nil {
		t.Fatalf("aead.New: %v", err)
	}
	ctx := context.Background()

	type rawChunk struct {
		seq    int
		data   []byte
		offset int64
	}
	var raws []rawChunk
	for start := 0; start < len(original); start += testPageSize {
		end := start + testPageSize
		if end > len(original) {
			end = len(original)
		}
		raws = append(raws, rawChunk{seq: len(raws), data: original[start:end], offset: int64(start)})
	}

	f := &fixture{
		objects:   map[string][]byte{},
		masterHex: hex.EncodeToString(master),
		pubHex:    hex.EncodeToString(pub),
		original:  original,
		snapID:    "snap1",
	}

	var (
		manifestChunks []mf.ChunkInfo
		plainChunks    []chunking.PlainChunk
		chunkIDs       []string
	)
	for _, rc := range raws {
		chunkID := digest.ChunkID(master, testPageSize, rc.data)
		var compressed bytes.Buffer
		if err := compressor.Compress(ctx, bytes.NewReader(rc.data), &compressed); err != nil {
			t.Fatalf("compress: %v", err)
		}
		ciphertext, err := enc.EncryptChunk(ctx, chunkID, compressed.Bytes())
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		objectKey := fmt.Sprintf("src1/snapshots/snap1/chunks/%06d.bin", rc.seq)
		f.objects[objectKey] = ciphertext
		chunkIDs = append(chunkIDs, chunkID)
		plainChunks = append(plainChunks, chunking.PlainChunk{Sequence: rc.seq, PageStart: rc.offset / testPageSize, PageCount: 1, PlaintextSize: int64(len(rc.data))})
		manifestChunks = append(manifestChunks, mf.ChunkInfo{
			Sequence:      rc.seq,
			ChunkID:       chunkID,
			ObjectKey:     objectKey,
			PageStart:     rc.offset / testPageSize,
			PageCount:     1,
			PlaintextSize: int64(len(rc.data)),
			StoredSize:    int64(len(ciphertext)),
			Compression:   compressor.Name(),
			KeyID:         testKeyID,
		})
	}

	root := digest.RootDigest(master, plainChunks, chunkIDs, int64(len(original)), testPageSize)
	man := mf.SnapshotManifest{
		Format:        "dbvault-snapshot",
		FormatVersion: 1,
		RepositoryID:  "repo1",
		SnapshotID:    f.snapID,
		SourceID:      "src1",
		Database: mf.DatabaseInfo{
			Engine:      "sqlite",
			PageSize:    testPageSize,
			LogicalSize: int64(len(original)),
		},
		Snapshot: mf.SnapshotInfo{Mode: "full", RootDigest: root},
		Chunks:   manifestChunks,
	}
	manifestBytes, err := json.Marshal(man)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	signer := edsigner.New("test-signing", priv, pub)
	sigBytes, err := signer.Sign(manifestBytes)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	sum := sha256.Sum256(manifestBytes)
	completion, err := json.Marshal(mf.Completion{
		Format:         "dbvault-completion",
		FormatVersion:  1,
		SnapshotID:     f.snapID,
		ManifestDigest: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatalf("marshal completion: %v", err)
	}

	f.objects[testManifestKey] = manifestBytes
	f.objects[strings.TrimSuffix(testManifestKey, ".json")+".sig"] = sigBytes
	f.objects["src1/snapshots/snap1/complete.json"] = completion

	if mutate != nil {
		mutate(f)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/"+testBucket+"/")
		obj, ok := f.objects[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(obj)
	}))
	t.Cleanup(srv.Close)
	f.endpointURL = srv.URL
	return f
}

func runRestore(t *testing.T, f *fixture, extraArgs ...string) int {
	t.Helper()
	args := []string{
		"--endpoint", f.endpointURL,
		"--bucket", testBucket,
		"--access-key", "AKID",
		"--secret-key", "SECRET",
		"--manifest", testManifestKey,
		"--master-key", f.masterHex,
		"--public-key", f.pubHex,
	}
	args = append(args, extraArgs...)
	var code int
	silenceStdout(t, func() {
		code = run(args)
	})
	return code
}

func TestRestore_RoundTrip(t *testing.T) {
	f := buildFixture(t, nil)
	outDir := t.TempDir()
	if code := runRestore(t, f, "--out", outDir); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	restored, err := os.ReadFile(filepath.Join(outDir, "database.restored"))
	if err != nil {
		t.Fatalf("restored file missing: %v", err)
	}
	if !bytes.Equal(restored, f.original) {
		t.Fatalf("restored bytes differ: got %d bytes, want %d bytes", len(restored), len(f.original))
	}
	summaryRaw, err := os.ReadFile(filepath.Join(outDir, "restore_summary.json"))
	if err != nil {
		t.Fatalf("summary missing: %v", err)
	}
	var summary map[string]any
	if err := json.Unmarshal(summaryRaw, &summary); err != nil {
		t.Fatalf("summary malformed: %v", err)
	}
	if summary["signature_state"] != "verified" {
		t.Fatalf("signature_state = %v, want verified", summary["signature_state"])
	}
	if summary["snapshot_id"] != f.snapID {
		t.Fatalf("snapshot_id = %v, want %s", summary["snapshot_id"], f.snapID)
	}
}

func TestRestore_VerifyOnly(t *testing.T) {
	f := buildFixture(t, nil)
	outDir := t.TempDir()
	if code := runRestore(t, f, "--out", outDir, "--verify-only"); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("verify-only wrote %d entries, want 0", len(entries))
	}
}

func TestRestore_MissingSignatureFailsClosed(t *testing.T) {
	f := buildFixture(t, nil)
	delete(f.objects, strings.TrimSuffix(testManifestKey, ".json")+".sig")

	outDir := t.TempDir()
	if code := runRestore(t, f, "--out", outDir); code == 0 {
		t.Fatal("missing signature must fail without --allow-unsigned")
	}
	// With --allow-unsigned the restore proceeds (no public key needed either).
	f2 := buildFixture(t, nil)
	delete(f2.objects, strings.TrimSuffix(testManifestKey, ".json")+".sig")
	f2.pubHex = ""
	outDir2 := t.TempDir()
	if code := runRestore(t, f2, "--out", outDir2, "--allow-unsigned"); code != 0 {
		t.Fatalf("expected --allow-unsigned restore to succeed, got %d", code)
	}
}

func TestRestore_TamperedManifestFailsSignature(t *testing.T) {
	f := buildFixture(t, func(f *fixture) {
		man := f.objects[testManifestKey]
		idx := bytes.Index(man, []byte(`"repo1"`))
		man[idx+1] = 'X' // valid JSON, different content than what was signed
		f.objects[testManifestKey] = man
	})
	if code := runRestore(t, f, "--out", t.TempDir()); code == 0 {
		t.Fatal("tampered manifest must fail signature verification")
	}
}

func TestRestore_TamperedChunkFails(t *testing.T) {
	f := buildFixture(t, nil)
	for key, obj := range f.objects {
		if strings.HasSuffix(key, ".bin") {
			obj[len(obj)-1] ^= 0xFF
			break
		}
	}
	if code := runRestore(t, f, "--out", t.TempDir()); code == 0 {
		t.Fatal("tampered chunk ciphertext must fail")
	}
}

func TestRestore_WrongMasterKeyFails(t *testing.T) {
	f := buildFixture(t, nil)
	f.masterHex = hex.EncodeToString(bytes.Repeat([]byte{0x99}, 32))
	if code := runRestore(t, f, "--out", t.TempDir()); code == 0 {
		t.Fatal("wrong master key must fail")
	}
}

func TestRestore_ReSignedWrongRootFails(t *testing.T) {
	// A fully re-signed manifest whose root digest does not match the chunks
	// must still fail — signature alone is not sufficient.
	f := buildFixture(t, nil)

	var man mf.SnapshotManifest
	if err := json.Unmarshal(f.objects[testManifestKey], &man); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	man.Snapshot.RootDigest = strings.Repeat("ab", 32)
	tampered, err := json.Marshal(man)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pub, _ := priv.Public().(ed25519.PublicKey)
	sig, err := edsigner.New("attacker-signing", priv, pub).Sign(tampered)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	f.objects[testManifestKey] = tampered
	f.objects[strings.TrimSuffix(testManifestKey, ".json")+".sig"] = sig
	f.pubHex = hex.EncodeToString(pub)
	// Also fix the completion record so the failure is attributable to the
	// root-digest check specifically, not the completion digest.
	tSum := sha256.Sum256(tampered)
	f.objects["src1/snapshots/snap1/complete.json"] = []byte(fmt.Sprintf(`{"format":"dbvault-completion","format_version":1,"snapshot_id":"snap1","manifest_digest":%q}`, hex.EncodeToString(tSum[:])))

	if code := runRestore(t, f, "--out", t.TempDir()); code == 0 {
		t.Fatal("re-signed manifest with wrong root digest must fail")
	}
}

func TestRestore_CompletionDigestMismatchFails(t *testing.T) {
	f := buildFixture(t, func(f *fixture) {
		f.objects["src1/snapshots/snap1/complete.json"] = []byte(`{"format":"dbvault-completion","format_version":1,"snapshot_id":"snap1","manifest_digest":"deadbeef"}`)
	})
	if code := runRestore(t, f, "--out", t.TempDir()); code == 0 {
		t.Fatal("completion digest mismatch must fail")
	}
}
