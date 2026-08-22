package restore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mf "github.com/dbvault/dbvault/internal/application/manifest"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Service struct {
	Catalogue  ports.Catalogue
	Store      ports.ObjectStore
	Compressor ports.Compressor
	Encryptor  ports.Encryptor
	Signer     ports.ManifestSigner
}

type Plan struct {
	SnapshotID        domain.SnapshotID
	LogicalSize       int64
	ChunkCount        int
	Target            string
	OverwriteRequired bool
	Warnings          []string
}

func (s *Service) Plan(ctx context.Context, id domain.SnapshotID, target string) (Plan, error) {
	snap, chunks, err := s.Catalogue.GetSnapshot(ctx, id)
	if err != nil {
		return Plan{}, err
	}
	_, err = os.Stat(target)
	warnings := []string{}
	if _, _, e := s.readAndVerifyManifest(ctx, snap); e != nil {
		warnings = append(warnings, e.Error())
	}
	return Plan{SnapshotID: id, LogicalSize: snap.DatabaseSize, ChunkCount: len(chunks), Target: target, OverwriteRequired: err == nil, Warnings: warnings}, nil
}

func (s *Service) Restore(ctx context.Context, id domain.SnapshotID, target string, replace bool) error {
	if _, err := os.Stat(target); err == nil && !replace {
		return domain.NewError(domain.ErrManifestInvalid, "target exists; pass replace", nil)
	}
	snap, _, err := s.Catalogue.GetSnapshot(ctx, id)
	if err != nil {
		return err
	}
	payload, man, err := s.readAndVerifyManifest(ctx, snap)
	if err != nil {
		return err
	}
	_ = payload
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	tmp := target + ".dbvault-restore-tmp"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()
	if man.Database.LogicalSize > 0 {
		if err := f.Truncate(man.Database.LogicalSize); err != nil {
			return err
		}
	}
	for _, ch := range man.Chunks {
		r, _, err := s.Store.Get(ctx, ports.GetObjectRequest{Key: ch.ObjectKey})
		if err != nil {
			return err
		}
		encrypted, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			return err
		}
		compressed, err := s.Encryptor.DecryptChunk(ctx, ch.ChunkID, encrypted)
		if err != nil {
			return domain.NewError(domain.ErrChunkAuthenticationFailed, "chunk authentication failed", err)
		}
		var plain bytes.Buffer
		if err := s.Compressor.Decompress(ctx, bytes.NewReader(compressed), &plain); err != nil {
			return err
		}
		offset := ch.PageStart * int64(man.Database.PageSize)
		if _, err := f.WriteAt(plain.Bytes(), offset); err != nil {
			return err
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := validateSQLiteHeader(tmp); err != nil {
		return err
	}
	if replace {
		if _, err := os.Stat(target); err == nil {
			rollback := fmt.Sprintf("%s.dbvault-rollback-%s", target, time.Now().UTC().Format("20060102T150405Z"))
			if err := os.Rename(target, rollback); err != nil {
				return err
			}
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	ok = true
	if dir, err := os.Open(filepath.Dir(target)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *Service) readAndVerifyManifest(ctx context.Context, snap domain.Snapshot) ([]byte, mf.SnapshotManifest, error) {
	var man mf.SnapshotManifest
	mr, _, err := s.Store.Get(ctx, ports.GetObjectRequest{Key: snap.ManifestObjectKey})
	if err != nil {
		return nil, man, err
	}
	payload, err := io.ReadAll(mr)
	_ = mr.Close()
	if err != nil {
		return nil, man, err
	}
	sigKey := strings.TrimSuffix(snap.ManifestObjectKey, ".json") + ".sig"
	if sigKey == "" || sigKey == ".sig" {
		sigKey = fmt.Sprintf("%s/snapshots/%s/manifest.sig", snap.SourceID, snap.ID)
	}
	sigR, _, err := s.Store.Get(ctx, ports.GetObjectRequest{Key: sigKey})
	if err != nil {
		return nil, man, err
	}
	sig, err := io.ReadAll(sigR)
	_ = sigR.Close()
	if err != nil {
		return nil, man, err
	}
	if err := s.Signer.Verify(payload, sig); err != nil {
		return nil, man, err
	}
	cr, _, err := s.Store.Get(ctx, ports.GetObjectRequest{Key: snap.CompletionObjectKey})
	if err != nil {
		return nil, man, err
	}
	cb, err := io.ReadAll(cr)
	_ = cr.Close()
	if err != nil {
		return nil, man, err
	}
	var comp mf.Completion
	if err := json.Unmarshal(cb, &comp); err != nil {
		return nil, man, err
	}
	h := sha256.Sum256(payload)
	if comp.ManifestDigest != hex.EncodeToString(h[:]) {
		return nil, man, domain.NewError(domain.ErrManifestInvalid, "completion digest does not match manifest", nil)
	}
	if err := json.Unmarshal(payload, &man); err != nil {
		return nil, man, err
	}
	if man.SnapshotID != string(snap.ID) {
		return nil, man, domain.NewError(domain.ErrManifestInvalid, "manifest snapshot mismatch", nil)
	}
	return payload, man, nil
}

func validateSQLiteHeader(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, 16)
	if _, err := io.ReadFull(f, header); err != nil {
		return err
	}
	if string(header) != "SQLite format 3\x00" {
		return domain.NewError(domain.ErrVerificationFailed, "restored file is not a SQLite database", nil)
	}
	return nil
}
