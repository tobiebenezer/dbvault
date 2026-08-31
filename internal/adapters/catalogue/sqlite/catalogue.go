//go:build restricted

// Package sqlite provides the Phase 2 durable catalogue adapter.
//
// In a normal production build this package is the seam where a real SQLite
// driver should be wired. This dependency-free implementation persists the same
// catalogue model to a single JSON file so the repository can compile and run in
// restricted environments without CGO or network access. The public adapter name
// and API are intentionally kept as "sqlite" so replacing the persistence
// backend with database/sql + SQLite migrations does not affect application
// services.
package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

type Catalogue struct {
	path string
	mu   sync.RWMutex
	db   state
}

type state struct {
	Version           int                                `json:"version"`
	Runs              map[string]domain.BackupRun        `json:"runs"`
	Snapshots         map[string]domain.Snapshot         `json:"snapshots"`
	Links             map[string][]domain.SnapshotChunk  `json:"links"`
	Chunks            map[string]domain.Chunk            `json:"chunks"`
	Audit             []AuditEvent                       `json:"audit"`
	Leases            map[string]LeaseRecord             `json:"leases"`
	WarehouseDatasets map[string]domain.WarehouseDataset `json:"warehouse_datasets,omitempty"`
	Connectors        map[string]domain.WarehouseConnectorRecord `json:"warehouse_connectors,omitempty"`
	UpdatedAt         time.Time                          `json:"updated_at"`
}

type AuditEvent struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Resource  string            `json:"resource,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

type LeaseRecord struct {
	Resource    string    `json:"resource"`
	OwnerID     string    `json:"owner_id"`
	AcquiredAt  time.Time `json:"acquired_at"`
	HeartbeatAt time.Time `json:"heartbeat_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func Open(path string) (*Catalogue, error) {
	if path == "" {
		return nil, fmt.Errorf("catalogue path is required")
	}
	c := &Catalogue{path: path, db: newState()}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, c.flushLocked()
		}
		return nil, err
	}
	if len(b) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(b, &c.db); err != nil {
		return nil, fmt.Errorf("read catalogue: %w", err)
	}
	c.ensureMaps()
	return c, nil
}

func newState() state {
	return state{Version: 2, Runs: map[string]domain.BackupRun{}, Snapshots: map[string]domain.Snapshot{}, Links: map[string][]domain.SnapshotChunk{}, Chunks: map[string]domain.Chunk{}, Leases: map[string]LeaseRecord{}, Audit: []AuditEvent{}, WarehouseDatasets: map[string]domain.WarehouseDataset{}, Connectors: map[string]domain.WarehouseConnectorRecord{}, UpdatedAt: time.Now().UTC()}
}
func (c *Catalogue) ensureMaps() {
	if c.db.Runs == nil {
		c.db.Runs = map[string]domain.BackupRun{}
	}
	if c.db.Snapshots == nil {
		c.db.Snapshots = map[string]domain.Snapshot{}
	}
	if c.db.Links == nil {
		c.db.Links = map[string][]domain.SnapshotChunk{}
	}
	if c.db.Chunks == nil {
		c.db.Chunks = map[string]domain.Chunk{}
	}
	if c.db.Leases == nil {
		c.db.Leases = map[string]LeaseRecord{}
	}
	if c.db.WarehouseDatasets == nil {
		c.db.WarehouseDatasets = map[string]domain.WarehouseDataset{}
	}
	if c.db.Connectors == nil {
		c.db.Connectors = map[string]domain.WarehouseConnectorRecord{}
	}
}
func (c *Catalogue) Close() error { return nil }

func (c *Catalogue) flushLocked() error {
	c.db.UpdatedAt = time.Now().UTC()
	tmp := c.path + ".tmp"
	b, err := json.MarshalIndent(c.db, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

func (c *Catalogue) CreateBackupRun(ctx context.Context, run domain.BackupRun) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.db.Runs[string(run.ID)] = run
	return c.flushLocked()
}
func (c *Catalogue) UpdateBackupRun(ctx context.Context, run domain.BackupRun) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.db.Runs[string(run.ID)] = run
	return c.flushLocked()
}
func (c *Catalogue) GetBackupRun(ctx context.Context, id domain.BackupRunID) (domain.BackupRun, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.db.Runs[string(id)]
	if !ok {
		return domain.BackupRun{}, os.ErrNotExist
	}
	return r, nil
}
func (c *Catalogue) ListRecoverableRuns(ctx context.Context) ([]domain.BackupRun, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := []domain.BackupRun{}
	for _, r := range c.db.Runs {
		switch r.Status {
		case domain.RunSnapshotting, domain.RunChunking, domain.RunUploading, domain.RunPublishing:
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out, nil
}
func (c *Catalogue) CreateSnapshot(ctx context.Context, snap domain.Snapshot, links []domain.SnapshotChunk, chunkRecords []domain.Chunk) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.db.Snapshots[string(snap.ID)] = snap
	c.db.Links[string(snap.ID)] = append([]domain.SnapshotChunk(nil), links...)
	for _, ch := range chunkRecords {
		key := string(ch.RepositoryID) + ":" + string(ch.ID)
		old, ok := c.db.Chunks[key]
		if ok {
			old.LastVerifiedAt = ch.LastVerifiedAt
			old.StoredSize = max64(old.StoredSize, ch.StoredSize)
			c.db.Chunks[key] = old
			continue
		}
		c.db.Chunks[key] = ch
	}
	return c.flushLocked()
}
func (c *Catalogue) GetSnapshot(ctx context.Context, id domain.SnapshotID) (domain.Snapshot, []domain.SnapshotChunk, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.db.Snapshots[string(id)]
	if !ok {
		return domain.Snapshot{}, nil, os.ErrNotExist
	}
	return s, append([]domain.SnapshotChunk(nil), c.db.Links[string(id)]...), nil
}
func (c *Catalogue) ListSnapshots(ctx context.Context, sourceID domain.SourceID) ([]domain.Snapshot, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := []domain.Snapshot{}
	for _, s := range c.db.Snapshots {
		if sourceID == "" || s.SourceID == sourceID {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (c *Catalogue) FindSnapshotByRoot(ctx context.Context, sourceID domain.SourceID, rootDigest string) (domain.Snapshot, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, s := range c.db.Snapshots {
		if s.SourceID == sourceID && s.RootDigest == rootDigest && (s.Status == domain.SnapshotCommitted || s.Status == domain.SnapshotProtected) {
			return s, true, nil
		}
	}
	return domain.Snapshot{}, false, nil
}
func (c *Catalogue) AppendAudit(ctx context.Context, e AuditEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.db.Audit = append(c.db.Audit, e)
	return c.flushLocked()
}
func (c *Catalogue) UpsertLease(ctx context.Context, l LeaseRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.db.Leases[l.Resource] = l
	return c.flushLocked()
}
func (c *Catalogue) GetLease(ctx context.Context, resource string) (LeaseRecord, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	l, ok := c.db.Leases[resource]
	return l, ok, nil
}
func (c *Catalogue) DeleteLease(ctx context.Context, resource string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.db.Leases, resource)
	return c.flushLocked()
}

func (c *Catalogue) UpsertWarehouseDataset(ctx context.Context, ds domain.WarehouseDataset) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.db.WarehouseDatasets[domain.WarehouseDatasetKey(ds.DatabaseID, ds.DatasetName)] = ds
	return c.flushLocked()
}

func (c *Catalogue) GetWarehouseDataset(ctx context.Context, databaseID, datasetName string) (domain.WarehouseDataset, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ds, ok := c.db.WarehouseDatasets[domain.WarehouseDatasetKey(databaseID, datasetName)]
	return ds, ok, nil
}

func (c *Catalogue) ListWarehouseDatasets(ctx context.Context, databaseID string) ([]domain.WarehouseDataset, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := []domain.WarehouseDataset{}
	for _, ds := range c.db.WarehouseDatasets {
		if databaseID == "" || ds.DatabaseID == databaseID {
			out = append(out, ds)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSyncAt.After(out[j].LastSyncAt) })
	return out, nil
}

func (c *Catalogue) UpsertWarehouseConnector(ctx context.Context, conn domain.WarehouseConnectorRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.db.Connectors[conn.ID] = conn
	return c.flushLocked()
}

func (c *Catalogue) GetWarehouseConnector(ctx context.Context, id string) (domain.WarehouseConnectorRecord, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	conn, ok := c.db.Connectors[id]
	return conn, ok, nil
}

func (c *Catalogue) DeleteWarehouseConnector(ctx context.Context, id string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.db.Connectors[id]; !ok {
		return false, nil
	}
	delete(c.db.Connectors, id)
	return true, c.flushLocked()
}

func (c *Catalogue) ListWarehouseConnectors(ctx context.Context) ([]domain.WarehouseConnectorRecord, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := []domain.WarehouseConnectorRecord{}
	for _, conn := range c.db.Connectors {
		out = append(out, conn)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
