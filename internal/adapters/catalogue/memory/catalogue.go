package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/dbvault/dbvault/internal/domain"
)

type Catalogue struct {
	mu                sync.Mutex
	runs              map[domain.BackupRunID]domain.BackupRun
	snaps             map[domain.SnapshotID]domain.Snapshot
	snapChunks        map[domain.SnapshotID][]domain.SnapshotChunk
	chunks            map[domain.ChunkID]domain.Chunk
	warehouseDatasets map[string]domain.WarehouseDataset
}

func New() *Catalogue {
	return &Catalogue{runs: map[domain.BackupRunID]domain.BackupRun{}, snaps: map[domain.SnapshotID]domain.Snapshot{}, snapChunks: map[domain.SnapshotID][]domain.SnapshotChunk{}, chunks: map[domain.ChunkID]domain.Chunk{}, warehouseDatasets: map[string]domain.WarehouseDataset{}}
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

func (c *Catalogue) UpsertWarehouseDataset(ctx context.Context, ds domain.WarehouseDataset) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.warehouseDatasets[domain.WarehouseDatasetKey(ds.DatabaseID, ds.DatasetName)] = ds
	return nil
}

func (c *Catalogue) GetWarehouseDataset(ctx context.Context, databaseID, datasetName string) (domain.WarehouseDataset, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ds, ok := c.warehouseDatasets[domain.WarehouseDatasetKey(databaseID, datasetName)]
	return ds, ok, nil
}

func (c *Catalogue) ListWarehouseDatasets(ctx context.Context, databaseID string) ([]domain.WarehouseDataset, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []domain.WarehouseDataset{}
	for _, ds := range c.warehouseDatasets {
		if databaseID == "" || ds.DatabaseID == databaseID {
			out = append(out, ds)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSyncAt.After(out[j].LastSyncAt) })
	return out, nil
}
