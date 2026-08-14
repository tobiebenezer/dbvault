package memory

import (
	"context"
	"fmt"
	"github.com/dbvault/dbvault/internal/domain"
	"sync"
)

type Catalogue struct {
	mu         sync.Mutex
	runs       map[domain.BackupRunID]domain.BackupRun
	snaps      map[domain.SnapshotID]domain.Snapshot
	snapChunks map[domain.SnapshotID][]domain.SnapshotChunk
	chunks     map[domain.ChunkID]domain.Chunk
}

func New() *Catalogue {
	return &Catalogue{runs: map[domain.BackupRunID]domain.BackupRun{}, snaps: map[domain.SnapshotID]domain.Snapshot{}, snapChunks: map[domain.SnapshotID][]domain.SnapshotChunk{}, chunks: map[domain.ChunkID]domain.Chunk{}}
}
func (c *Catalogue) CreateBackupRun(ctx context.Context, run domain.BackupRun) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runs[run.ID] = run
	return nil
}
func (c *Catalogue) UpdateBackupRun(ctx context.Context, run domain.BackupRun) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runs[run.ID] = run
	return nil
}
func (c *Catalogue) CreateSnapshot(ctx context.Context, snap domain.Snapshot, sc []domain.SnapshotChunk, cr []domain.Chunk) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snaps[snap.ID] = snap
	c.snapChunks[snap.ID] = append([]domain.SnapshotChunk(nil), sc...)
	for _, ch := range cr {
		c.chunks[ch.ID] = ch
	}
	return nil
}
func (c *Catalogue) GetSnapshot(ctx context.Context, id domain.SnapshotID) (domain.Snapshot, []domain.SnapshotChunk, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.snaps[id]
	if !ok {
		return domain.Snapshot{}, nil, fmt.Errorf("snapshot not found")
	}
	return s, append([]domain.SnapshotChunk(nil), c.snapChunks[id]...), nil
}
func (c *Catalogue) ListSnapshots(ctx context.Context, sourceID domain.SourceID) ([]domain.Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []domain.Snapshot{}
	for _, s := range c.snaps {
		if sourceID == "" || s.SourceID == sourceID {
			out = append(out, s)
		}
	}
	return out, nil
}
func (c *Catalogue) FindSnapshotByRoot(ctx context.Context, sourceID domain.SourceID, rootDigest string) (domain.Snapshot, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.snaps {
		if s.SourceID == sourceID && s.RootDigest == rootDigest && s.Status == domain.SnapshotCommitted {
			return s, true, nil
		}
	}
	return domain.Snapshot{}, false, nil
}
