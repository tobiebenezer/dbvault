package database

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sync"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

// MemoryArtifactSink is a test and adapter seam implementation. Production backup services
// can replace it with a chunking/compression/encryption pipeline without changing drivers.
type MemoryArtifactSink struct {
	mu        sync.Mutex
	artifacts map[domain.ArtifactID][]byte
}

func NewMemoryArtifactSink() *MemoryArtifactSink {
	return &MemoryArtifactSink{artifacts: map[domain.ArtifactID][]byte{}}
}
func (s *MemoryArtifactSink) OpenArtifact(ctx context.Context, spec domain.ArtifactSpec) (ports.ArtifactWriter, error) {
	return &memoryArtifactWriter{sink: s, spec: spec}, nil
}

type memoryArtifactWriter struct {
	sink    *MemoryArtifactSink
	spec    domain.ArtifactSpec
	buf     bytes.Buffer
	aborted bool
}

func (w *memoryArtifactWriter) Write(p []byte) (int, error) {
	if w.aborted {
		return 0, io.ErrClosedPipe
	}
	return w.buf.Write(p)
}
func (w *memoryArtifactWriter) Close() error { return nil }
func (w *memoryArtifactWriter) Abort(ctx context.Context, cause error) error {
	w.aborted = true
	return nil
}
func (w *memoryArtifactWriter) Commit(ctx context.Context, meta domain.ArtifactCommitMetadata) (domain.StoredArtifact, error) {
	sum := sha256.Sum256(w.buf.Bytes())
	root := hex.EncodeToString(sum[:])
	if meta.RootDigest != "" {
		root = meta.RootDigest
	}
	art := domain.BackupArtifact{ID: w.spec.ID, Type: w.spec.Type, Name: w.spec.Name, Format: w.spec.Format, ContentType: w.spec.ContentType, Required: w.spec.Required, Sequence: w.spec.Sequence, LogicalSize: int64(w.buf.Len()), RootDigest: root, Metadata: meta.Metadata}
	w.sink.mu.Lock()
	w.sink.artifacts[w.spec.ID] = append([]byte(nil), w.buf.Bytes()...)
	w.sink.mu.Unlock()
	return domain.StoredArtifact{Artifact: art}, nil
}

func (s *MemoryArtifactSink) Source() *MemoryArtifactSource {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := map[domain.ArtifactID][]byte{}
	for k, v := range s.artifacts {
		cp[k] = append([]byte(nil), v...)
	}
	return &MemoryArtifactSource{artifacts: cp}
}

type MemoryArtifactSource struct{ artifacts map[domain.ArtifactID][]byte }

func (s *MemoryArtifactSource) OpenArtifact(ctx context.Context, id domain.ArtifactID) (io.ReadCloser, domain.BackupArtifact, error) {
	b, ok := s.artifacts[id]
	if !ok {
		return nil, domain.BackupArtifact{}, domain.NewError(domain.ErrChunkMissing, "artifact not found", nil)
	}
	sum := sha256.Sum256(b)
	return io.NopCloser(bytes.NewReader(b)), domain.BackupArtifact{ID: id, LogicalSize: int64(len(b)), RootDigest: hex.EncodeToString(sum[:])}, nil
}
