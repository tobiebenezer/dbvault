//go:build !restricted

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/dbvault/dbvault/internal/domain"
)

// Queue is the production durable job queue backed by a SQLite `jobs` table.
// Lease transitions run inside IMMEDIATE transactions (via the _txlock DSN
// option) so two handles never hand out one job twice. When constructed via
// New() the queue is process-local memory only.
type Queue struct {
	core *memoryCore
	db   *sql.DB
}

// New returns a process-local in-memory queue.
func New() *Queue { return &Queue{core: newMemoryCore()} }

// Open returns a durable queue backed by the SQLite database at path.
func Open(path string) (*Queue, error) {
	if path == "" {
		return nil, fmt.Errorf("job queue path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_synchronous=FULL&_busy_timeout=5000&_txlock=immediate", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	q := &Queue{db: db}
	if err := q.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return q, nil
}

func (q *Queue) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS jobs(
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			resource_id TEXT NOT NULL DEFAULT '',
			priority INTEGER NOT NULL DEFAULT 0,
			attempt INTEGER NOT NULL DEFAULT 0,
			maximum_attempts INTEGER NOT NULL DEFAULT 5,
			available_at TEXT NOT NULL,
			lease_owner TEXT NOT NULL DEFAULT '',
			lease_expires_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status, priority DESC, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_type ON jobs(type, status);`,
	}
	for _, stmt := range stmts {
		if _, err := q.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// Close releases the database handle.
func (q *Queue) Close() error {
	if q.db == nil {
		return nil
	}
	return q.db.Close()
}

const jobColumns = `id, type, status, resource_id, priority, attempt, maximum_attempts, available_at, lease_owner, lease_expires_at, created_at, updated_at, json`

func scanJob(scan func(dest ...any) error) (domain.Job, error) {
	var j domain.Job
	var id, typ, status, resourceID, leaseOwner string
	var priority, attempt, maximumAttempts int
	var availableAt, createdAt, updatedAt, js string
	var leaseExpiresAt sql.NullString
	if err := scan(&id, &typ, &status, &resourceID, &priority, &attempt, &maximumAttempts, &availableAt, &leaseOwner, &leaseExpiresAt, &createdAt, &updatedAt, &js); err != nil {
		return domain.Job{}, err
	}
	j.ID = domain.JobID(id)
	j.Type = domain.JobType(typ)
	j.Status = domain.JobStatus(status)
	j.ResourceID = resourceID
	j.Priority = priority
	j.Attempt = attempt
	j.MaximumAttempts = maximumAttempts
	j.AvailableAt = parseTimeOr(availableAt, time.Time{})
	j.LeaseOwner = leaseOwner
	j.CreatedAt = parseTimeOr(createdAt, time.Time{})
	j.UpdatedAt = parseTimeOr(updatedAt, time.Time{})
	if leaseExpiresAt.Valid && leaseExpiresAt.String != "" {
		t := parseTimeOr(leaseExpiresAt.String, time.Time{})
		j.LeaseExpiresAt = &t
	}
	if err := json.Unmarshal([]byte(js), &j); err != nil {
		return domain.Job{}, fmt.Errorf("decode job %s: %w", id, err)
	}
	// Dedicated columns win over the JSON blob for lease bookkeeping so that
	// atomic SQL updates are always authoritative.
	j.LeaseOwner = leaseOwner
	if leaseExpiresAt.Valid && leaseExpiresAt.String != "" {
		t := parseTimeOr(leaseExpiresAt.String, time.Time{})
		j.LeaseExpiresAt = &t
	} else {
		j.LeaseExpiresAt = nil
	}
	return j, nil
}

func parseTimeOr(s string, fallback time.Time) time.Time {
	if s == "" {
		return fallback
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return fallback
	}
	return t
}

func jobArgs(j domain.Job) ([]any, string) {
	b, _ := json.Marshal(j)
	leaseExpiry := ""
	if j.LeaseExpiresAt != nil {
		leaseExpiry = j.LeaseExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	return []any{j.ID, j.Type, j.Status, j.ResourceID, j.Priority, j.Attempt, j.MaximumAttempts, j.AvailableAt.UTC().Format(time.RFC3339Nano), j.LeaseOwner, leaseExpiry, j.CreatedAt.UTC().Format(time.RFC3339Nano), j.UpdatedAt.UTC().Format(time.RFC3339Nano), string(b)},
		`INSERT INTO jobs(` + jobColumns + `) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET type=excluded.type, status=excluded.status, resource_id=excluded.resource_id, priority=excluded.priority, attempt=excluded.attempt, maximum_attempts=excluded.maximum_attempts, available_at=excluded.available_at, lease_owner=excluded.lease_owner, lease_expires_at=excluded.lease_expires_at, created_at=excluded.created_at, updated_at=excluded.updated_at, json=excluded.json`
}

func (q *Queue) Enqueue(ctx context.Context, job domain.Job) error {
	if q.db == nil {
		q.core.mu.Lock()
		defer q.core.mu.Unlock()
		return q.core.enqueueLocked(job)
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id=?`, job.ID)
	existing, err := scanJob(row.Scan)
	switch {
	case err == nil:
		if !existing.Terminal() {
			// Live occurrence wins; schedule ticks and restarts must not
			// double-book the same deterministic job ID.
			return tx.Commit()
		}
	case err == sql.ErrNoRows:
	default:
		return err
	}
	args, upsert := jobArgs(job)
	if _, err := tx.ExecContext(ctx, upsert, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Queue) LeaseNext(ctx context.Context, workerID string, types []domain.JobType, leaseDuration time.Duration) (domain.Job, bool, error) {
	if q.db == nil {
		q.core.mu.Lock()
		defer q.core.mu.Unlock()
		job, ok := q.core.leaseNextLocked(workerID, types, leaseDuration, time.Now().UTC())
		return job, ok, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	expiry := time.Now().UTC().Add(leaseDuration).Format(time.RFC3339Nano)
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Job{}, false, err
	}
	defer tx.Rollback()
	query := `SELECT ` + jobColumns + ` FROM jobs WHERE status IN ('pending','retrying','interrupted')
		AND available_at <= ?
		AND (lease_expires_at IS NULL OR lease_expires_at <= ?)`
	args := []any{now, now}
	if len(types) > 0 {
		query += ` AND type IN (` + placeholders(len(types)) + `)`
		for _, t := range types {
			args = append(args, string(t))
		}
	}
	query += ` ORDER BY priority DESC, created_at ASC LIMIT 1`
	row := tx.QueryRowContext(ctx, query, args...)
	candidate, err := scanJob(row.Scan)
	if err == sql.ErrNoRows {
		return domain.Job{}, false, nil
	}
	if err != nil {
		return domain.Job{}, false, err
	}
	started := candidate.StartedAt
	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET status='leased', lease_owner=?, lease_expires_at=?, updated_at=? WHERE id=?`,
		workerID, expiry, now, candidate.ID); err != nil {
		return domain.Job{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Job{}, false, err
	}
	candidate.LeaseOwner = workerID
	t, _ := time.Parse(time.RFC3339Nano, expiry)
	candidate.LeaseExpiresAt = &t
	candidate.Status = domain.JobLeased
	candidate.StartedAt = started
	return candidate, true, nil
}

func placeholders(n int) string {
	out := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ","
		}
		out += "?"
	}
	return out
}

func (q *Queue) Renew(ctx context.Context, jobID domain.JobID, workerID string, leaseDuration time.Duration) error {
	if q.db == nil {
		q.core.mu.Lock()
		defer q.core.mu.Unlock()
		return q.core.renewLocked(jobID, workerID, leaseDuration, time.Now().UTC())
	}
	res, err := q.db.ExecContext(ctx, `UPDATE jobs SET lease_expires_at=?, updated_at=? WHERE id=? AND lease_owner=?`,
		time.Now().UTC().Add(leaseDuration).Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), jobID, workerID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NewError(domain.ErrJobUnavailable, "job lease owner mismatch or job not found", nil)
	}
	return nil
}

func (q *Queue) Complete(ctx context.Context, jobID domain.JobID, resultJSON []byte) error {
	if q.db == nil {
		q.core.mu.Lock()
		defer q.core.mu.Unlock()
		return q.core.completeLocked(jobID, resultJSON, time.Now().UTC())
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	j, err := q.loadLocked(ctx, tx, jobID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	j.Status = domain.JobSucceeded
	j.ResultJSON = resultJSON
	j.CompletedAt = &now
	j.UpdatedAt = now
	args, upsert := jobArgs(j)
	if _, err := tx.ExecContext(ctx, upsert, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Queue) Fail(ctx context.Context, jobID domain.JobID, failure domain.JobFailure) error {
	if q.db == nil {
		q.core.mu.Lock()
		defer q.core.mu.Unlock()
		return q.core.failLocked(jobID, failure, time.Now().UTC())
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	j, err := q.loadLocked(ctx, tx, jobID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	j.Attempt++
	j.LastErrorCode = string(failure.Code)
	j.LastError = failure.Message
	j.UpdatedAt = now
	if failure.Retryable && j.Attempt < j.MaximumAttempts {
		j.Status = domain.JobRetrying
		j.AvailableAt = now.Add(domain.RetryDelay(j.Attempt))
	} else if failure.Retryable {
		j.Status = domain.JobDeadLetter
		j.CompletedAt = &now
	} else {
		j.Status = domain.JobFailed
		j.CompletedAt = &now
	}
	args, upsert := jobArgs(j)
	if _, err := tx.ExecContext(ctx, upsert, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Queue) Cancel(ctx context.Context, jobID domain.JobID) error {
	if q.db == nil {
		q.core.mu.Lock()
		defer q.core.mu.Unlock()
		return q.core.cancelLocked(jobID, time.Now().UTC())
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	j, err := q.loadLocked(ctx, tx, jobID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	j.Status = domain.JobCancelled
	j.CompletedAt = &now
	j.UpdatedAt = now
	args, upsert := jobArgs(j)
	if _, err := tx.ExecContext(ctx, upsert, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Queue) RecoverExpired(ctx context.Context, now time.Time) ([]domain.JobID, error) {
	if q.db == nil {
		q.core.mu.Lock()
		defer q.core.mu.Unlock()
		return q.core.recoverExpiredLocked(now), nil
	}
	rows, err := q.db.QueryContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE lease_expires_at IS NOT NULL AND lease_expires_at < ? AND status NOT IN ('succeeded','failed','cancelled','dead_letter')`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	expired := []domain.Job{}
	for rows.Next() {
		j, err := scanJob(rows.Scan)
		if err != nil {
			rows.Close()
			return nil, err
		}
		expired = append(expired, j)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	out := make([]domain.JobID, 0, len(expired))
	for _, j := range expired {
		tx, err := q.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		jj := j
		jj.Status = domain.JobInterrupted
		jj.LeaseOwner = ""
		jj.LeaseExpiresAt = nil
		jj.AvailableAt = now
		jj.UpdatedAt = now
		args, upsert := jobArgs(jj)
		if _, err := tx.ExecContext(ctx, upsert, args...); err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		out = append(out, j.ID)
	}
	return out, nil
}

func (q *Queue) loadLocked(ctx context.Context, tx *sql.Tx, jobID domain.JobID) (domain.Job, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id=?`, jobID)
	j, err := scanJob(row.Scan)
	if err == sql.ErrNoRows {
		return domain.Job{}, domain.NewError(domain.ErrJobUnavailable, "job not found", nil)
	}
	return j, err
}
