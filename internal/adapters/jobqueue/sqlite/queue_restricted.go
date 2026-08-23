//go:build restricted

package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

// Queue is the restricted-build durable job queue. State lives in memory and
// is flushed to a single JSON file (atomic tmp+rename, mode 0600) after every
// mutation, mirroring the restricted catalogue adapter's persistence pattern.
type Queue struct {
	path string
	core *memoryCore
}

// New returns a process-local in-memory queue.
func New() *Queue { return &Queue{core: newMemoryCore()} }

// Open returns a durable queue backed by the file at path.
func Open(path string) (*Queue, error) {
	if path == "" {
		return nil, fmt.Errorf("job queue path is required")
	}
	q := &Queue{path: path, core: newMemoryCore()}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := q.flushLocked(); err != nil {
				return nil, err
			}
			return q, nil
		}
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &q.core.jobs); err != nil {
			return nil, fmt.Errorf("read job queue: %w", err)
		}
	}
	if q.core.jobs == nil {
		q.core.jobs = map[domain.JobID]domain.Job{}
	}
	return q, nil
}

// Close releases resources; the restricted queue holds no open handles.
func (q *Queue) Close() error { return nil }

func (q *Queue) flushLocked() error {
	if q.path == "" {
		return nil
	}
	tmp := q.path + ".tmp"
	b, err := json.MarshalIndent(q.core.jobs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, q.path)
}

func (q *Queue) Enqueue(ctx context.Context, job domain.Job) error {
	q.core.mu.Lock()
	defer q.core.mu.Unlock()
	if err := q.core.enqueueLocked(job); err != nil {
		return err
	}
	return q.flushLocked()
}

func (q *Queue) LeaseNext(ctx context.Context, workerID string, types []domain.JobType, leaseDuration time.Duration) (domain.Job, bool, error) {
	q.core.mu.Lock()
	defer q.core.mu.Unlock()
	job, ok := q.core.leaseNextLocked(workerID, types, leaseDuration, time.Now().UTC())
	if !ok {
		return domain.Job{}, false, nil
	}
	if err := q.flushLocked(); err != nil {
		return domain.Job{}, false, err
	}
	return job, true, nil
}

func (q *Queue) Renew(ctx context.Context, jobID domain.JobID, workerID string, leaseDuration time.Duration) error {
	q.core.mu.Lock()
	defer q.core.mu.Unlock()
	if err := q.core.renewLocked(jobID, workerID, leaseDuration, time.Now().UTC()); err != nil {
		return err
	}
	return q.flushLocked()
}

func (q *Queue) Complete(ctx context.Context, jobID domain.JobID, resultJSON []byte) error {
	q.core.mu.Lock()
	defer q.core.mu.Unlock()
	if err := q.core.completeLocked(jobID, resultJSON, time.Now().UTC()); err != nil {
		return err
	}
	return q.flushLocked()
}

func (q *Queue) Fail(ctx context.Context, jobID domain.JobID, failure domain.JobFailure) error {
	q.core.mu.Lock()
	defer q.core.mu.Unlock()
	if err := q.core.failLocked(jobID, failure, time.Now().UTC()); err != nil {
		return err
	}
	return q.flushLocked()
}

func (q *Queue) Cancel(ctx context.Context, jobID domain.JobID) error {
	q.core.mu.Lock()
	defer q.core.mu.Unlock()
	if err := q.core.cancelLocked(jobID, time.Now().UTC()); err != nil {
		return err
	}
	return q.flushLocked()
}

func (q *Queue) RecoverExpired(ctx context.Context, now time.Time) ([]domain.JobID, error) {
	q.core.mu.Lock()
	defer q.core.mu.Unlock()
	out := q.core.recoverExpiredLocked(now)
	if len(out) > 0 {
		if err := q.flushLocked(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Depth reports the number of live (non-terminal) jobs.
func (q *Queue) Depth(ctx context.Context) (int, error) {
	q.core.mu.Lock()
	defer q.core.mu.Unlock()
	return q.core.depthLocked(), nil
}
