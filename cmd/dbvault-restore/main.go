// Command dbvault-restore is a standalone emergency restore utility for
// DBVault. It reads encrypted snapshot manifests directly from an
// S3-compatible object storage bucket, verifies the Ed25519 signature and the
// keyed root digest, decrypts and decompresses chunks, and reassembles the
// database file — without requiring a running DBVault daemon.
//
// The tool shares the manifest, digest and AEAD formats with the server so it
// can restore real snapshots. The only external protocol it implements itself
// is a minimal S3 SigV4 GET client.
//
// Usage:
//
//	dbvault-restore \
//	  --endpoint    <s3_endpoint_url>    \
//	  --bucket      <bucket_name>        \
//	  --access-key  <access_key_id>      \
//	  --secret-key  <secret_key>         \
//	  --manifest    <manifest_object_key>\
//	  --master-key  <hex_encoded_32byte_key> \
//	  --public-key  <hex_encoded_32byte_ed25519_key> \
//	  --out         <output_directory>   \
//	  [--verify-only]                    \
//	  [--allow-unsigned]                 \
//	  [--region     <aws_region>]
//
// The master key is the repository root key. For restricted installs it is
// SHA-256("<repository-id>:<active-key>"); for production installs it is the
// repository encryption key material stored by the configured secret provider.
// The signing public key must be supplied out-of-band (never from the bucket).
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	zstdc "github.com/dbvault/dbvault/internal/adapters/compression/zstd"
	aead "github.com/dbvault/dbvault/internal/adapters/encryption/aead"
	edsigner "github.com/dbvault/dbvault/internal/adapters/manifest/ed25519"
	"github.com/dbvault/dbvault/internal/application/chunking"
	"github.com/dbvault/dbvault/internal/application/digest"
	mf "github.com/dbvault/dbvault/internal/application/manifest"
	"github.com/dbvault/dbvault/internal/platform/keyderive"
)

const version = "0.1.0-alpha"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("dbvault-restore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		endpoint      = fs.String("endpoint", "", "S3-compatible endpoint URL (e.g. https://s3.cloudflare.com/)")
		bucket        = fs.String("bucket", "", "S3 bucket name containing the backup")
		accessKey     = fs.String("access-key", "", "S3 access key ID")
		secretKey     = fs.String("secret-key", "", "S3 secret access key")
		manifestKey   = fs.String("manifest", "", "Object key for manifest.json (e.g. backups/src/2026-08-30/manifest.json)")
		masterKeyHex  = fs.String("master-key", "", "Hex-encoded 32-byte repository master key")
		publicKeyHex  = fs.String("public-key", "", "Hex-encoded 32-byte Ed25519 manifest signing public key (out-of-band)")
		outDir        = fs.String("out", ".", "Output directory for the restored database file")
		verifyOnly    = fs.Bool("verify-only", false, "Perform cryptographic verification only; do not write output files")
		allowUnsigned = fs.Bool("allow-unsigned", false, "Skip manifest signature verification when no .sig object exists")
		region        = fs.String("region", "auto", "AWS region (use 'auto' for R2 or MinIO)")
	)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "DBVault Emergency Restore Tool v%s\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage: dbvault-restore [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  dbvault-restore \\\n")
		fmt.Fprintf(os.Stderr, "    --endpoint https://account.r2.cloudflarestorage.com \\\n")
		fmt.Fprintf(os.Stderr, "    --bucket dbvault-backups \\\n")
		fmt.Fprintf(os.Stderr, "    --access-key <key> --secret-key <secret> \\\n")
		fmt.Fprintf(os.Stderr, "    --manifest backups/src/2026-08-30T12:00:00Z/manifest.json \\\n")
		fmt.Fprintf(os.Stderr, "    --master-key aabbccddeeff00112233445566778899aabbccddeeff001122334455667788 \\\n")
		fmt.Fprintf(os.Stderr, "    --public-key 11223344556677889900aabbccddeeff00112233445566778899aabbccddeeff \\\n")
		fmt.Fprintf(os.Stderr, "    --out /var/restore/database\n")
	}
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *endpoint == "" || *bucket == "" || *accessKey == "" || *secretKey == "" || *manifestKey == "" || *masterKeyHex == "" {
		fmt.Fprintln(os.Stderr, "error: --endpoint, --bucket, --access-key, --secret-key, --manifest, and --master-key are required")
		flag.Usage()
		return 1
	}

	masterKey, err := hex.DecodeString(strings.TrimSpace(*masterKeyHex))
	if err != nil || len(masterKey) != 32 {
		fmt.Fprintln(os.Stderr, "error: --master-key must be a hex-encoded 32-byte (64 hex chars) repository master key")
		return 1
	}
	var pubKey []byte
	if pk := strings.TrimSpace(*publicKeyHex); pk != "" {
		decoded, decErr := hex.DecodeString(pk)
		if decErr != nil || len(decoded) != 32 {
			fmt.Fprintln(os.Stderr, "error: --public-key must be a hex-encoded 32-byte (64 hex chars) Ed25519 public key")
			return 1
		}
		pubKey = decoded
	} else if !*allowUnsigned {
		fmt.Fprintln(os.Stderr, "error: --public-key is required for signature verification (or pass --allow-unsigned to skip)")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	s3 := &S3Client{
		Endpoint:  strings.TrimSuffix(*endpoint, "/"),
		Bucket:    *bucket,
		AccessKey: *accessKey,
		SecretKey: *secretKey,
		Region:    *region,
	}

	fmt.Printf("DBVault Emergency Restore v%s\n", version)
	fmt.Printf("Endpoint:    %s\n", *endpoint)
	fmt.Printf("Bucket:      %s\n", *bucket)
	fmt.Printf("Manifest:    %s\n", *manifestKey)
	fmt.Printf("Verify only: %v\n\n", *verifyOnly)

	fmt.Println("[1/5] Fetching and parsing manifest...")
	manifestData, err := s3.GetObject(ctx, *manifestKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot fetch manifest %q: %v\n", *manifestKey, err)
		return 1
	}
	var man mf.SnapshotManifest
	if err := json.Unmarshal(manifestData, &man); err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid manifest JSON: %v\n", err)
		return 1
	}
	if man.SnapshotID == "" || len(man.Chunks) == 0 {
		fmt.Fprintf(os.Stderr, "error: manifest is not a DBVault snapshot manifest (missing snapshot_id or chunks)\n")
		return 1
	}
	fmt.Printf("  Snapshot:    %s\n", man.SnapshotID)
	fmt.Printf("  Engine:      %s\n", man.Database.Engine)
	fmt.Printf("  Page size:   %d\n", man.Database.PageSize)
	fmt.Printf("  Logical:     %d bytes\n", man.Database.LogicalSize)
	fmt.Printf("  Chunks:      %d\n\n", len(man.Chunks))

	fmt.Println("[2/5] Verifying Ed25519 signature...")
	sigKey := strings.TrimSuffix(*manifestKey, ".json") + ".sig"
	sigData, sigErr := s3.GetObject(ctx, sigKey)
	switch {
	case sigErr == nil:
		if len(pubKey) == 0 {
			fmt.Fprintf(os.Stderr, "error: manifest is signed but no --public-key was supplied; refusing to trust bucket-hosted keys\n")
			return 1
		}
		ok, verifyErr := edsigner.VerifyKey(pubKey, manifestData, sigData)
		if verifyErr != nil || !ok {
			fmt.Fprintf(os.Stderr, "error: SIGNATURE INVALID: %v\n", verifyErr)
			return 1
		}
		fmt.Println("  ✓ Ed25519 signature verified")
	case *allowUnsigned:
		fmt.Println("  ⚠ No signature found — verification skipped (--allow-unsigned)")
	default:
		fmt.Fprintf(os.Stderr, "error: missing manifest signature at %q (pass --allow-unsigned to accept unsigned manifests): %v\n", sigKey, sigErr)
		return 1
	}

	if completionKey := strings.TrimSuffix(*manifestKey, "manifest.json") + "complete.json"; completionKey != *manifestKey {
		if completionData, compErr := s3.GetObject(ctx, completionKey); compErr == nil {
			fmt.Printf("  ✓ Completion record found at %s\n", completionKey)
			var comp mf.Completion
			if err := json.Unmarshal(completionData, &comp); err != nil {
				fmt.Fprintf(os.Stderr, "error: malformed completion record: %v\n", err)
				return 1
			}
			sum := sha256.Sum256(manifestData)
			if comp.ManifestDigest != hex.EncodeToString(sum[:]) {
				fmt.Fprintf(os.Stderr, "error: completion digest does not match manifest (possible tampering)\n")
				return 1
			}
			fmt.Println("  ✓ Completion digest matches manifest")
		} else {
			fmt.Printf("  ⚠ No completion record at %s — skipping (signature and root digest still enforced)\n", completionKey)
		}
	}
	fmt.Println()

	fmt.Println("[3/5] Verifying root digest integrity...")
	expectedRoot := man.Snapshot.RootDigest
	if expectedRoot == "" {
		expectedRoot = man.RootDigest
	}
	if expectedRoot == "" {
		fmt.Fprintf(os.Stderr, "error: manifest has no root digest; refusing to restore\n")
		return 1
	}
	chunkIDs := make([]string, len(man.Chunks))
	plainChunks := make([]chunking.PlainChunk, len(man.Chunks))
	keyVersions := map[string]bool{}
	for i, ch := range man.Chunks {
		chunkIDs[i] = ch.ChunkID
		plainChunks[i] = chunking.PlainChunk{Sequence: ch.Sequence, PageStart: ch.PageStart, PageCount: ch.PageCount, PlaintextSize: ch.PlaintextSize}
		if ch.KeyID != "" {
			keyVersions[ch.KeyID] = true
		}
	}
	var dedupKey []byte
	for _, candidate := range dedupKeyCandidates(masterKey) {
		tree, treeErr := digest.BuildTreeFromHashes(candidate, chunkIDs)
		if treeErr != nil {
			continue
		}
		computed := digest.RootDigest(candidate, plainChunks, chunkIDs, man.Database.LogicalSize, man.Database.PageSize)
		if tree.RootHash == expectedRoot || computed == expectedRoot {
			dedupKey = candidate
			break
		}
	}
	if len(man.Chunks) > 0 && dedupKey == nil {
		fmt.Fprintf(os.Stderr, "error: TAMPER DETECTED — root digest mismatch for manifest %s!\n", man.SnapshotID)
		fmt.Fprintf(os.Stderr, "  expected root: %s\n", expectedRoot)
		return 1
	}
	fmt.Println("  ✓ Root digest verified")

	if *verifyOnly {
		fmt.Println("\n✓ Verification complete (--verify-only mode; no files written)")
		return 0
	}

	fmt.Printf("[4/5] Decrypting, decompressing and reassembling %d chunks...\n", len(man.Chunks))
	if err := os.MkdirAll(*outDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot create output directory: %v\n", err)
		return 1
	}
	outputPath := filepath.Join(*outDir, "database.restored")
	tmpPath := outputPath + ".dbvault-restore-tmp"
	_ = os.Remove(tmpPath)
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot create output file: %v\n", err)
		return 1
	}
	if man.Database.LogicalSize > 0 {
		if err := out.Truncate(man.Database.LogicalSize); err != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			fmt.Fprintf(os.Stderr, "error: cannot size output file to %d bytes: %v\n", man.Database.LogicalSize, err)
			return 1
		}
	}

	decrypter := newChunkDecrypter(masterKey, keyVersions)
	compressor := zstdc.New(0, 0)
	for i, ch := range man.Chunks {
		fmt.Printf("  chunk %d/%d: %s\r", i+1, len(man.Chunks), safeID(ch.ChunkID))
		ciphertext, err := s3.GetObject(ctx, ch.ObjectKey)
		if err != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			fmt.Fprintf(os.Stderr, "\nerror: cannot fetch chunk %s: %v\n", safeID(ch.ChunkID), err)
			return 1
		}
		compressed, err := decrypter.decrypt(ch.ChunkID, ciphertext)
		if err != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			fmt.Fprintf(os.Stderr, "\nerror: decryption failed for chunk %s: %v\n", safeID(ch.ChunkID), err)
			return 1
		}
		var plain bytes.Buffer
		if err := compressor.Decompress(ctx, bytes.NewReader(compressed), &plain); err != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			fmt.Fprintf(os.Stderr, "\nerror: decompression failed for chunk %s: %v\n", safeID(ch.ChunkID), err)
			return 1
		}
		plainBytes := plain.Bytes()
		if int64(len(plainBytes)) != ch.PlaintextSize {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			fmt.Fprintf(os.Stderr, "\nerror: chunk %s plaintext size mismatch (manifest %d, actual %d)\n", safeID(ch.ChunkID), ch.PlaintextSize, len(plainBytes))
			return 1
		}
		if dedupKey != nil && man.Database.PageSize > 0 {
			if computed := digest.ChunkID(dedupKey, man.Database.PageSize, plainBytes); computed != ch.ChunkID {
				_ = out.Close()
				_ = os.Remove(tmpPath)
				fmt.Fprintf(os.Stderr, "\nerror: TAMPER DETECTED — chunk %s plaintext hash mismatch\n", safeID(ch.ChunkID))
				return 1
			}
		}
		offset := ch.PageStart * int64(man.Database.PageSize)
		if _, err := out.WriteAt(plainBytes, offset); err != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			fmt.Fprintf(os.Stderr, "\nerror: cannot write chunk %s: %v\n", safeID(ch.ChunkID), err)
			return 1
		}
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(tmpPath)
		fmt.Fprintf(os.Stderr, "\nerror: cannot flush output file: %v\n", err)
		return 1
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		fmt.Fprintf(os.Stderr, "\nerror: cannot close output file: %v\n", err)
		return 1
	}
	engine := strings.ToLower(man.Database.Engine)
	if engine == "" || engine == "sqlite" {
		if err := verifySQLiteHeader(tmpPath); err != nil {
			_ = os.Remove(tmpPath)
			fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
			return 1
		}
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		_ = os.Remove(tmpPath)
		fmt.Fprintf(os.Stderr, "\nerror: cannot finalize output file: %v\n", err)
		return 1
	}
	fmt.Printf("\n  ✓ All %d chunks reassembled\n\n", len(man.Chunks))

	fmt.Println("[5/5] Writing restore summary...")
	summaryPath := filepath.Join(*outDir, "restore_summary.json")
	summary := map[string]any{
		"restored_at":     time.Now().UTC().Format(time.RFC3339),
		"snapshot_id":     man.SnapshotID,
		"repository_id":   man.RepositoryID,
		"source_id":       man.SourceID,
		"engine":          man.Database.Engine,
		"logical_size":    man.Database.LogicalSize,
		"page_size":       man.Database.PageSize,
		"root_digest":     expectedRoot,
		"signature_state": signatureState(*allowUnsigned),
		"total_chunks":    len(man.Chunks),
		"output_file":     outputPath,
		"dbvault_version": version,
	}
	summaryJSON, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(summaryPath, summaryJSON, 0600); err != nil {
		fmt.Printf("  ⚠ Cannot write restore summary: %v\n", err)
	} else {
		fmt.Printf("  ✓ Summary written: %s\n", summaryPath)
	}

	fmt.Printf("\n✓ Emergency restore complete!\n")
	fmt.Printf("  Output file:      %s\n", outputPath)
	fmt.Printf("  Snapshot ID:      %s\n", man.SnapshotID)
	fmt.Printf("  Engine:           %s\n", man.Database.Engine)
	fmt.Printf("  Logical size:     %d bytes\n", man.Database.LogicalSize)
	if engine == "" || engine == "sqlite" {
		fmt.Printf("\nThe output is a SQLite database file; open or copy it directly.\n")
	} else {
		fmt.Printf("\nThe output holds the reassembled %s artifact; import it with the engine's restore tooling.\n", man.Database.Engine)
	}
	return 0
}

func signatureState(allowUnsigned bool) string {
	if allowUnsigned {
		return "unsigned-accepted"
	}
	return "verified"
}

func safeID(id string) string {
	if len(id) > 16 {
		return id[:16] + "..."
	}
	return id
}

func dedupKeyCandidates(master []byte) [][]byte {
	candidates := [][]byte{master}
	if dk, err := keyderive.Derive(master, keyderive.DomainDedup); err == nil {
		candidates = append(candidates, dk[:])
	}
	return candidates
}

func verifySQLiteHeader(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, 16)
	if _, err := io.ReadFull(f, header); err != nil {
		return fmt.Errorf("cannot read restored file header: %w", err)
	}
	if string(header) != "SQLite format 3\x00" {
		return fmt.Errorf("restored file is not a SQLite database")
	}
	return nil
}

// chunkDecrypter walks candidate root-key derivations (raw key for restricted
// installs, HKDF-derived per key version, with the aead package's built-in
// legacy fallback) until a chunk authenticates, then reuses the winner.
type chunkDecrypter struct {
	master     []byte
	candidates []*aead.Encryptor
	winner     int
}

func newChunkDecrypter(master []byte, keyVersions map[string]bool) *chunkDecrypter {
	d := &chunkDecrypter{master: master, winner: -1}
	if enc, err := aead.New(master); err == nil {
		d.candidates = append(d.candidates, enc)
	}
	versions := make([]string, 0, len(keyVersions)+1)
	for v := range keyVersions {
		versions = append(versions, v)
	}
	if _, ok := keyVersions["1"]; !ok && !keyVersions[""] {
		versions = append(versions, "1")
	}
	sort.Strings(versions)
	for _, v := range versions {
		d.addVersion(v)
	}
	return d
}

func (d *chunkDecrypter) addVersion(version string) {
	if enc, err := aead.NewWithDerivation(d.master, version); err == nil {
		d.candidates = append(d.candidates, enc)
	}
}

func (d *chunkDecrypter) decrypt(chunkID string, ciphertext []byte) ([]byte, error) {
	var firstErr error
	order := []int{}
	if d.winner >= 0 {
		order = append(order, d.winner)
	}
	for i := range d.candidates {
		if i != d.winner {
			order = append(order, i)
		}
	}
	for _, i := range order {
		plain, err := d.candidates[i].DecryptChunk(context.Background(), chunkID, ciphertext)
		if err == nil {
			d.winner = i
			return plain, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, firstErr
}

// S3Client is a minimal, dependency-free S3-compatible HTTP client using
// AWS Signature Version 4 (SigV4) for authentication.
type S3Client struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
}

func (c *S3Client) GetObject(ctx context.Context, key string) ([]byte, error) {
	rawURL := fmt.Sprintf("%s/%s/%s", c.Endpoint, c.Bucket, key)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	bodyHash := sha256Hex([]byte(""))

	headers := map[string]string{
		"Host":                 parsed.Host,
		"x-amz-date":           amzDate,
		"x-amz-content-sha256": bodyHash,
	}

	sortedHeaderKeys := sortedKeys(headers)
	var canonicalHeaders strings.Builder
	var signedHeaderNames strings.Builder
	for i, k := range sortedHeaderKeys {
		canonicalHeaders.WriteString(strings.ToLower(k) + ":" + headers[k] + "\n")
		if i > 0 {
			signedHeaderNames.WriteByte(';')
		}
		signedHeaderNames.WriteString(strings.ToLower(k))
	}
	signedHeaders := signedHeaderNames.String()

	canonicalURI := parsed.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalRequest := strings.Join([]string{
		"GET",
		canonicalURI,
		"",
		canonicalHeaders.String(),
		signedHeaders,
		bodyHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, c.Region)
	crHash := sha256Hex([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		crHash,
	}, "\n")

	signingKey := c.signingKey(dateStamp)
	sig := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s,SignedHeaders=%s,Signature=%s",
		c.AccessKey, credentialScope, signedHeaders, sig,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", bodyHash)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("S3 error %d for %s: %s", resp.StatusCode, key, strings.TrimSpace(string(body)))
	}

	return io.ReadAll(resp.Body)
}

func (c *S3Client) signingKey(dateStamp string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+c.SecretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(c.Region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.ToLower(keys[i]) < strings.ToLower(keys[j])
	})
	return keys
}
