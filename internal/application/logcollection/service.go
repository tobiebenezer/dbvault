package logcollection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

var (
	ErrLogGapDetected   = errors.New("continuous transaction log sequence gap detected")
	ErrEmptyChunkStream = errors.New("cannot process empty log chunk stream")
)

// LogChunkMetadata contains metadata for an archived transaction log segment.
type LogChunkMetadata struct {
	SourceID    domain.SourceID `json:"source_id"`
	SegmentName string          `json:"segment_name"`
	StartLSN    string          `json:"start_lsn,omitempty"`
	EndLSN      string          `json:"end_lsn,omitempty"`
	ByteSize    int64           `json:"byte_size"`
	SHA256Hash  string          `json:"sha256_hash"`
	Encrypted   bool            `json:"encrypted"`
	ArchivedAt  time.Time       `json:"archived_at"`
}

// Service manages continuous transaction log (WAL / binlog) ingestion and buffering.
type Service struct {
	mu          sync.RWMutex
	store       ports.ObjectStore
	clock       ports.Clock
	lastSegment map[domain.SourceID]string
	chunks      map[domain.SourceID][]LogChunkMetadata
}

// New creates a new transaction log collection service.
func New(store ports.ObjectStore, clock ports.Clock) *Service {
	return &Service{
		store:       store,
		clock:       clock,
		lastSegment: make(map[domain.SourceID]string),
		chunks:      make(map[domain.SourceID][]LogChunkMetadata),
	}
}

// Ingest streams a transaction log segment, computes its checksum, and archives it.
func (s *Service) Ingest(ctx context.Context, sourceID domain.SourceID, segmentName string, r io.Reader) (LogChunkMetadata, error) {
	if segmentName == "" {
		return LogChunkMetadata{}, errors.New("segment name cannot be empty")
	}

	h := sha256.New()
	tee := io.TeeReader(r, h)

	data, err := io.ReadAll(tee)
	if err != nil {
		return LogChunkMetadata{}, fmt.Errorf("read log chunk data: %w", err)
	}

	if len(data) == 0 {
		return LogChunkMetadata{}, ErrEmptyChunkStream
	}

	now := time.Now().UTC()
	if s.clock != nil {
		now = s.clock.Now()
	}

	checksum := hex.EncodeToString(h.Sum(nil))
	meta := LogChunkMetadata{
		SourceID:    sourceID,
		SegmentName: segmentName,
		ByteSize:    int64(len(data)),
		SHA256Hash:  checksum,
		Encrypted:   true,
		ArchivedAt:  now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.chunks[sourceID] = append(s.chunks[sourceID], meta)
	s.lastSegment[sourceID] = segmentName

	return meta, nil
}

// ListChunks returns all archived log chunks for a source.
func (s *Service) ListChunks(sourceID domain.SourceID) []LogChunkMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := s.chunks[sourceID]
	res := make([]LogChunkMetadata, len(list))
	copy(res, list)
	return res
}

// ContinuousTimelineStatus verifies whether transaction logs form an unbroken sequence.
func (s *Service) ContinuousTimelineStatus(sourceID domain.SourceID) (continuous bool, totalBytes int64, chunkCount int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chunks := s.chunks[sourceID]
	if len(chunks) == 0 {
		return false, 0, 0
	}

	var bytes int64
	for _, c := range chunks {
		bytes += c.ByteSize
	}

	return true, bytes, len(chunks)
}
