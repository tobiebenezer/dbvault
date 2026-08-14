//go:build !restricted

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/dbvault/dbvault/internal/domain"
)

type Catalogue struct {
	path string
	db   *sql.DB
	mu   sync.Mutex
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
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&_synchronous=FULL&_busy_timeout=5000", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	c := &Catalogue{path: path, db: db}
	if err := c.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return c, nil
}

func (c *Catalogue) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, name TEXT NOT NULL, checksum TEXT NOT NULL, applied_at TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS backup_runs(id TEXT PRIMARY KEY, source_id TEXT NOT NULL, repository_id TEXT NOT NULL, status TEXT NOT NULL, started_at TEXT NOT NULL, json TEXT NOT NULL);`,
		`CREATE INDEX IF NOT EXISTS idx_backup_runs_source_started ON backup_runs(source_id, started_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_backup_runs_status ON backup_runs(status);`,
		`CREATE TABLE IF NOT EXISTS snapshots(id TEXT PRIMARY KEY, source_id TEXT NOT NULL, repository_id TEXT NOT NULL, root_digest TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, json TEXT NOT NULL);`,
		`CREATE INDEX IF NOT EXISTS idx_snapshots_source_created ON snapshots(source_id, created_at DESC);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_snapshots_source_root ON snapshots(source_id, root_digest) WHERE status IN ('committed','protected');`,
		`CREATE TABLE IF NOT EXISTS snapshot_chunks(snapshot_id TEXT NOT NULL, sequence INTEGER NOT NULL, json TEXT NOT NULL, PRIMARY KEY(snapshot_id, sequence));`,
		`CREATE TABLE IF NOT EXISTS chunks(repository_id TEXT NOT NULL, id TEXT NOT NULL, object_key TEXT NOT NULL, json TEXT NOT NULL, PRIMARY KEY(repository_id, id));`,
		`CREATE TABLE IF NOT EXISTS leases(resource TEXT PRIMARY KEY, owner_id TEXT NOT NULL, acquired_at TEXT NOT NULL, heartbeat_at TEXT NOT NULL, expires_at TEXT NOT NULL, json TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS audit_events(id TEXT PRIMARY KEY, event_type TEXT NOT NULL, resource TEXT, json TEXT NOT NULL, created_at TEXT NOT NULL);`,
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *Catalogue) Close() error { return c.db.Close() }

func (c *Catalogue) CreateBackupRun(ctx context.Context, run domain.BackupRun) error {
	b, _ := json.Marshal(run)
	_, err := c.db.ExecContext(ctx, `INSERT INTO backup_runs(id, source_id, repository_id, status, started_at, json) VALUES(?,?,?,?,?,?)`, run.ID, run.SourceID, run.RepositoryID, run.Status, run.StartedAt.UTC().Format(time.RFC3339Nano), string(b))
	return err
}
func (c *Catalogue) UpdateBackupRun(ctx context.Context, run domain.BackupRun) error {
	b, _ := json.Marshal(run)
	_, err := c.db.ExecContext(ctx, `INSERT INTO backup_runs(id, source_id, repository_id, status, started_at, json) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,json=excluded.json`, run.ID, run.SourceID, run.RepositoryID, run.Status, run.StartedAt.UTC().Format(time.RFC3339Nano), string(b))
	return err
}
func (c *Catalogue) GetBackupRun(ctx context.Context, id domain.BackupRunID) (domain.BackupRun, error) {
	var js string
	if err := c.db.QueryRowContext(ctx, `SELECT json FROM backup_runs WHERE id=?`, id).Scan(&js); err != nil {
		return domain.BackupRun{}, err
	}
	var r domain.BackupRun
	return r, json.Unmarshal([]byte(js), &r)
}
func (c *Catalogue) ListRecoverableRuns(ctx context.Context) ([]domain.BackupRun, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT json FROM backup_runs WHERE status IN ('snapshotting','chunking','uploading','publishing') ORDER BY started_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.BackupRun{}
	for rows.Next() {
		var js string
		if err := rows.Scan(&js); err != nil {
			return nil, err
		}
		var r domain.BackupRun
		if err := json.Unmarshal([]byte(js), &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (c *Catalogue) CreateSnapshot(ctx context.Context, snap domain.Snapshot, links []domain.SnapshotChunk, chunkRecords []domain.Chunk) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	b, _ := json.Marshal(snap)
	if _, err := tx.ExecContext(ctx, `INSERT INTO snapshots(id, source_id, repository_id, root_digest, status, created_at, json) VALUES(?,?,?,?,?,?,?)`, snap.ID, snap.SourceID, snap.RepositoryID, snap.RootDigest, snap.Status, snap.CreatedAt.UTC().Format(time.RFC3339Nano), string(b)); err != nil {
		return err
	}
	for _, l := range links {
		lb, _ := json.Marshal(l)
		if _, err := tx.ExecContext(ctx, `INSERT INTO snapshot_chunks(snapshot_id, sequence, json) VALUES(?,?,?)`, l.SnapshotID, l.Sequence, string(lb)); err != nil {
			return err
		}
	}
	for _, ch := range chunkRecords {
		cb, _ := json.Marshal(ch)
		if _, err := tx.ExecContext(ctx, `INSERT INTO chunks(repository_id,id,object_key,json) VALUES(?,?,?,?) ON CONFLICT(repository_id,id) DO UPDATE SET object_key=excluded.object_key,json=excluded.json`, ch.RepositoryID, ch.ID, ch.ObjectKey, string(cb)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (c *Catalogue) GetSnapshot(ctx context.Context, id domain.SnapshotID) (domain.Snapshot, []domain.SnapshotChunk, error) {
	var js string
	if err := c.db.QueryRowContext(ctx, `SELECT json FROM snapshots WHERE id=?`, id).Scan(&js); err != nil {
		return domain.Snapshot{}, nil, err
	}
	var s domain.Snapshot
	if err := json.Unmarshal([]byte(js), &s); err != nil {
		return domain.Snapshot{}, nil, err
	}
	rows, err := c.db.QueryContext(ctx, `SELECT json FROM snapshot_chunks WHERE snapshot_id=? ORDER BY sequence`, id)
	if err != nil {
		return domain.Snapshot{}, nil, err
	}
	defer rows.Close()
	links := []domain.SnapshotChunk{}
	for rows.Next() {
		var ljs string
		if err := rows.Scan(&ljs); err != nil {
			return domain.Snapshot{}, nil, err
		}
		var l domain.SnapshotChunk
		if err := json.Unmarshal([]byte(ljs), &l); err != nil {
			return domain.Snapshot{}, nil, err
		}
		links = append(links, l)
	}
	return s, links, rows.Err()
}
func (c *Catalogue) ListSnapshots(ctx context.Context, sourceID domain.SourceID) ([]domain.Snapshot, error) {
	q := `SELECT json FROM snapshots`
	args := []any{}
	if sourceID != "" {
		q += ` WHERE source_id=?`
		args = append(args, sourceID)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := c.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Snapshot{}
	for rows.Next() {
		var js string
		if err := rows.Scan(&js); err != nil {
			return nil, err
		}
		var s domain.Snapshot
		if err := json.Unmarshal([]byte(js), &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (c *Catalogue) FindSnapshotByRoot(ctx context.Context, sourceID domain.SourceID, rootDigest string) (domain.Snapshot, bool, error) {
	var js string
	err := c.db.QueryRowContext(ctx, `SELECT json FROM snapshots WHERE source_id=? AND root_digest=? AND status IN ('committed','protected') LIMIT 1`, sourceID, rootDigest).Scan(&js)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Snapshot{}, false, nil
	}
	if err != nil {
		return domain.Snapshot{}, false, err
	}
	var s domain.Snapshot
	if err := json.Unmarshal([]byte(js), &s); err != nil {
		return domain.Snapshot{}, false, err
	}
	return s, true, nil
}
func (c *Catalogue) AppendAudit(ctx context.Context, e AuditEvent) error {
	b, _ := json.Marshal(e)
	_, err := c.db.ExecContext(ctx, `INSERT INTO audit_events(id,event_type,resource,json,created_at) VALUES(?,?,?,?,?)`, e.ID, e.Type, e.Resource, string(b), e.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}
func (c *Catalogue) UpsertLease(ctx context.Context, l LeaseRecord) error {
	b, _ := json.Marshal(l)
	_, err := c.db.ExecContext(ctx, `INSERT INTO leases(resource,owner_id,acquired_at,heartbeat_at,expires_at,json) VALUES(?,?,?,?,?,?) ON CONFLICT(resource) DO UPDATE SET owner_id=excluded.owner_id,heartbeat_at=excluded.heartbeat_at,expires_at=excluded.expires_at,json=excluded.json`, l.Resource, l.OwnerID, l.AcquiredAt.Format(time.RFC3339Nano), l.HeartbeatAt.Format(time.RFC3339Nano), l.ExpiresAt.Format(time.RFC3339Nano), string(b))
	return err
}
func (c *Catalogue) GetLease(ctx context.Context, resource string) (LeaseRecord, bool, error) {
	var js string
	err := c.db.QueryRowContext(ctx, `SELECT json FROM leases WHERE resource=?`, resource).Scan(&js)
	if errors.Is(err, sql.ErrNoRows) {
		return LeaseRecord{}, false, nil
	}
	if err != nil {
		return LeaseRecord{}, false, err
	}
	var l LeaseRecord
	if err := json.Unmarshal([]byte(js), &l); err != nil {
		return LeaseRecord{}, false, err
	}
	return l, true, nil
}
func (c *Catalogue) DeleteLease(ctx context.Context, resource string) error {
	_, err := c.db.ExecContext(ctx, `DELETE FROM leases WHERE resource=?`, resource)
	return err
}
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

var _ = sort.Slice
