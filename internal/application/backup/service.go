package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/dbvault/dbvault/internal/application/chunking"
	"github.com/dbvault/dbvault/internal/application/digest"
	mf "github.com/dbvault/dbvault/internal/application/manifest"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
	"io"
	"strings"
)

type IDGenerator interface{ NewID(prefix string) string }

type Service struct {
	Catalogue        ports.Catalogue
	Source           ports.SourceDriver
	Store            ports.ObjectStore
	Scratch          ports.ScratchManager
	Compressor       ports.Compressor
	Encryptor        ports.Encryptor
	Signer           ports.ManifestSigner
	Clock            ports.Clock
	IDs              IDGenerator
	DedupKey         []byte
	Repository       domain.Repository
	TargetChunkBytes int64
}

type Command struct {
	Source  domain.Source
	Trigger domain.BackupTrigger
}
type Result struct {
	Run       domain.BackupRun
	Snapshot  *domain.Snapshot
	Duplicate bool
}

func (s *Service) Create(ctx context.Context, cmd Command) (Result, error) {
	now := s.Clock.Now()
	runID := domain.BackupRunID(s.IDs.NewID("run"))
	run := domain.BackupRun{ID: runID, SourceID: cmd.Source.ID, RepositoryID: s.Repository.ID, Trigger: cmd.Trigger, Status: domain.RunSnapshotting, StartedAt: now}
	_ = s.Catalogue.CreateBackupRun(ctx, run)
	reservation, err := s.Scratch.Reserve(ctx, runID, 0)
	if err != nil {
		return s.fail(ctx, run, domain.ErrScratchInsufficient, err)
	}
	defer reservation.Release()
	art, err := s.Source.CreateSnapshot(ctx, ports.SnapshotRequest{RunID: runID, Source: cmd.Source, ScratchDirectory: reservation.Dir, Mode: domain.SQLiteSnapshotOnlineBackup})
	if err != nil {
		return s.fail(ctx, run, domain.ErrSnapshotFailed, err)
	}
	defer art.Close()
	meta := art.Metadata()
	run.SourceSize = art.Size()
	run.Status = domain.RunChunking
	_ = s.Catalogue.UpdateBackupRun(ctx, run)
	target := s.TargetChunkBytes
	if target == 0 {
		target = 4 * 1024 * 1024
	}
	chunks, err := chunking.NewPageChunker().Chunks(ctx, art, target)
	if err != nil {
		return s.fail(ctx, run, domain.ErrChunkReadFailed, err)
	}
	ids := make([]string, len(chunks))
	for i, ch := range chunks {
		ids[i] = digest.ChunkID(s.DedupKey, meta.PageSize, ch.Data)
	}
	root := digest.RootDigest(s.DedupKey, chunks, ids, art.Size(), meta.PageSize)
	if existing, ok, err := s.Catalogue.FindSnapshotByRoot(ctx, cmd.Source.ID, root); err == nil && ok {
		run.Status = domain.RunDuplicate
		run.DuplicateOf = &existing.ID
		t := s.Clock.Now()
		run.CompletedAt = &t
		_ = s.Catalogue.UpdateBackupRun(ctx, run)
		return Result{Run: run, Snapshot: &existing, Duplicate: true}, nil
	}
	snapID := domain.SnapshotID(s.IDs.NewID("snap"))
	snapChunks := []domain.SnapshotChunk{}
	chunkRecords := []domain.Chunk{}
	manifestChunks := []mf.ChunkInfo{}
	run.Status = domain.RunUploading
	_ = s.Catalogue.UpdateBackupRun(ctx, run)
	var compressedBytes, uniqueBytes int64
	for i, ch := range chunks {
		chunkID := ids[i]
		key := chunkObjectKey(string(cmd.Source.ID), string(s.Repository.ID), s.Repository.Encryption.KeyID, chunkID)
		if _, err := s.Store.Head(ctx, key); err == nil {
			run.ReusedBytes += ch.PlaintextSize
		} else if !isObjectNotFound(err) {
			return s.fail(ctx, run, domain.ErrStorageUnavailable, err)
		} else {
			var buf bytes.Buffer
			if err := s.Compressor.Compress(ctx, bytes.NewReader(ch.Data), &buf); err != nil {
				return s.fail(ctx, run, domain.ErrSnapshotFailed, err)
			}
			enc, err := s.Encryptor.EncryptChunk(ctx, chunkID, buf.Bytes())
			if err != nil {
				return s.fail(ctx, run, domain.ErrSnapshotFailed, err)
			}
			stored, err := s.Store.Put(ctx, ports.PutObjectRequest{Key: key, Body: bytes.NewReader(enc), Size: int64(len(enc)), ContentType: "application/octet-stream", IfNotExists: true})
			if err != nil {
				return s.fail(ctx, run, domain.ErrStorageUnavailable, err)
			}
			uniqueBytes += stored.Size
			compressedBytes += int64(buf.Len())
		}
		snapChunks = append(snapChunks, domain.SnapshotChunk{SnapshotID: snapID, ChunkID: domain.ChunkID(chunkID), RepositoryID: s.Repository.ID, Sequence: ch.Sequence, PageStart: ch.PageStart, PageCount: ch.PageCount, PlaintextSize: ch.PlaintextSize})
		chunkRecords = append(chunkRecords, domain.Chunk{ID: domain.ChunkID(chunkID), RepositoryID: s.Repository.ID, KeyVersion: s.Repository.Encryption.KeyID, Compression: s.Compressor.Name(), PlaintextSize: ch.PlaintextSize, ObjectKey: key, CreatedAt: now})
		manifestChunks = append(manifestChunks, mf.ChunkInfo{Sequence: ch.Sequence, ChunkID: chunkID, ObjectKey: key, PageStart: ch.PageStart, PageCount: ch.PageCount, PlaintextSize: ch.PlaintextSize, Compression: s.Compressor.Name(), KeyID: s.Repository.Encryption.KeyID})
	}
	manifest := mf.SnapshotManifest{Format: "dbvault-snapshot", FormatVersion: 1, RepositoryID: string(s.Repository.ID), SnapshotID: string(snapID), SourceID: string(cmd.Source.ID), Database: mf.DatabaseInfo{Engine: "sqlite", SQLiteVersion: meta.EngineVersion, PageSize: meta.PageSize, PageCount: meta.PageCount, LogicalSize: art.Size(), SchemaDigest: meta.SchemaDigest}, Snapshot: mf.SnapshotInfo{Mode: string(domain.SQLiteSnapshotOnlineBackup), CreatedAt: now.Format("2006-01-02T15:04:05.999999999Z07:00"), RootDigest: root, Chunking: map[string]any{"strategy": "sqlite-pages-v1", "target_size": target}}, Chunks: manifestChunks}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return s.fail(ctx, run, domain.ErrManifestInvalid, err)
	}
	sig, err := s.Signer.Sign(payload)
	if err != nil {
		return s.fail(ctx, run, domain.ErrManifestInvalid, err)
	}
	manifestKey := fmt.Sprintf("%s/snapshots/%s/manifest.json", cmd.Source.ID, snapID)
	sigKey := fmt.Sprintf("%s/snapshots/%s/manifest.sig", cmd.Source.ID, snapID)
	completeKey := fmt.Sprintf("%s/snapshots/%s/complete.json", cmd.Source.ID, snapID)
	if _, err := s.Store.Put(ctx, ports.PutObjectRequest{Key: manifestKey, Body: bytes.NewReader(payload), Size: int64(len(payload)), ContentType: "application/json", IfNotExists: true}); err != nil {
		return s.fail(ctx, run, domain.ErrStorageUnavailable, err)
	}
	if _, err := s.Store.Put(ctx, ports.PutObjectRequest{Key: sigKey, Body: bytes.NewReader(sig), Size: int64(len(sig)), ContentType: "application/octet-stream", IfNotExists: true}); err != nil {
		return s.fail(ctx, run, domain.ErrStorageUnavailable, err)
	}
	h := sha256.Sum256(payload)
	completion := mf.Completion{Format: "dbvault-completion", FormatVersion: 1, SnapshotID: string(snapID), ManifestDigest: hex.EncodeToString(h[:]), CommittedAt: s.Clock.Now().Format("2006-01-02T15:04:05.999999999Z07:00")}
	compPayload, _ := json.MarshalIndent(completion, "", "  ")
	if _, err := s.Store.Put(ctx, ports.PutObjectRequest{Key: completeKey, Body: bytes.NewReader(compPayload), Size: int64(len(compPayload)), ContentType: "application/json", IfNotExists: true}); err != nil {
		return s.fail(ctx, run, domain.ErrStorageUnavailable, err)
	}
	committed := s.Clock.Now()
	snap := domain.Snapshot{ID: snapID, SourceID: cmd.Source.ID, RepositoryID: s.Repository.ID, Status: domain.SnapshotCommitted, SnapshotMode: domain.SQLiteSnapshotOnlineBackup, DatabaseSize: art.Size(), PageSize: meta.PageSize, PageCount: meta.PageCount, RootDigest: root, SchemaDigest: meta.SchemaDigest, ChunkCount: len(chunks), UniqueChunkCount: len(chunks), CompressedBytes: compressedBytes, UniqueUploadedBytes: uniqueBytes, CreatedAt: now, CommittedAt: &committed, VerifiedAt: &committed, ManifestObjectKey: manifestKey, CompletionObjectKey: completeKey}
	if err := s.Catalogue.CreateSnapshot(ctx, snap, snapChunks, chunkRecords); err != nil {
		return s.fail(ctx, run, domain.ErrManifestInvalid, err)
	}
	run.Status = domain.RunCommitted
	run.SnapshotID = &snapID
	run.UniqueBytes = uniqueBytes
	t := s.Clock.Now()
	run.CompletedAt = &t
	_ = s.Catalogue.UpdateBackupRun(ctx, run)
	return Result{Run: run, Snapshot: &snap}, nil
}
func (s *Service) fail(ctx context.Context, run domain.BackupRun, code domain.ErrorCode, err error) (Result, error) {
	run.Status = domain.RunFailed
	run.ErrorCode = string(code)
	run.ErrorMessage = err.Error()
	t := s.Clock.Now()
	run.CompletedAt = &t
	_ = s.Catalogue.UpdateBackupRun(ctx, run)
	return Result{Run: run}, err
}
func isObjectNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	var app *domain.AppError
	if errors.As(err, &app) && app.Code == domain.ErrChunkMissing {
		return true
	}
	return false
}

func chunkObjectKey(sourceID, repo, keyVersion, chunkID string) string {
	p := chunkID
	if len(p) < 4 {
		p = strings.Repeat("0", 4-len(p)) + p
	}
	if sourceID != "" {
		return fmt.Sprintf("%s/chunks/%s/%s/%s/%s.dvchunk", sourceID, keyVersion, p[:2], p[2:4], chunkID)
	}
	return fmt.Sprintf("chunks/%s/%s/%s/%s.dvchunk", keyVersion, p[:2], p[2:4], chunkID)
}

var _ = io.EOF
