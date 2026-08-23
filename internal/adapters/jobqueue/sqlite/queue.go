// Package sqlite provides the durable job queue adapter behind ports.JobQueue.
//
// The public API and method signatures are identical in both build variants so
// application services never change:
//
//	restricted build: jobs persist to a single JSON file using the same
//	  dependency-free atomic-write pattern as the restricted catalogue adapter.
//	production build: jobs persist to a real SQLite `jobs` table via
//	  database/sql, mirroring the production catalogue adapter.
//
// New() returns a process-local in-memory queue (used by unit tests and demo
// paths). Open(path) returns the durable queue.
package sqlite

import (
	"sort"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

const defaultMaximumAttempts = 5

// memoryCore holds the leasing decision logic shared by both build variants.
// All operations expect core.mu to be held by the caller.
type memoryCore struct {
	mu   sync.Mutex
	jobs map[domain.JobID]domain.Job
}

func newMemoryCore() *memoryCore {
	return &memoryCore{jobs: map[domain.JobID]domain.Job{}}
}

func (c *memoryCore) enqueueLocked(job domain.Job) error {
	if existing, ok := c.jobs[job.ID]; ok && !existing.Terminal() {
		// A live (queued/leased/retrying/interrupted) occurrence wins over a
		// duplicate enqueue so schedule ticks and restarts cannot double-book.
		return nil
	}
	if job.Status == "" {
		job.Status = domain.JobPending
	}
	if job.MaximumAttempts == 0 {
		job.MaximumAttempts = defaultMaximumAttempts
	}
	if job.AvailableAt.IsZero() {
		job.AvailableAt = time.Now().UTC()
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	job.UpdatedAt = time.Now().UTC()
	c.jobs[job.ID] = job
	return nil
}

func (c *memoryCore) leaseNextLocked(workerID string, types []domain.JobType, leaseDuration time.Duration, now time.Time) (domain.Job, bool) {
	allowed := map[domain.JobType]bool{}
	for _, t := range types {
		allowed[t] = true
	}
	candidates := []domain.Job{}
	for _, j := range c.jobs {
		if j.Terminal() {
			continue
		}
		if len(allowed) > 0 && !allowed[j.Type] {
			continue
		}
		if j.AvailableAt.After(now) {
			continue
		}
		if j.LeaseExpiresAt != nil && j.LeaseExpiresAt.After(now) {
			continue
		}
		candidates = append(candidates, j)
	}
	if len(candidates) == 0 {
		return domain.Job{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
		}
		return candidates[i].Priority > candidates[j].Priority
	})
	j := candidates[0]
	exp := now.Add(leaseDuration)
	j.LeaseOwner = workerID
	j.LeaseExpiresAt = &exp
	j.Status = domain.JobLeased
	j.UpdatedAt = now
	c.jobs[j.ID] = j
	return j, true
}

func (c *memoryCore) renewLocked(jobID domain.JobID, workerID string, leaseDuration time.Duration, now time.Time) error {
	j, ok := c.jobs[jobID]
	if !ok {
		return domain.NewError(domain.ErrJobUnavailable, "job not found", nil)
	}
	if j.LeaseOwner != workerID {
		return domain.NewError(domain.ErrJobUnavailable, "job lease owner mismatch", nil)
	}
	exp := now.Add(leaseDuration)
	j.LeaseExpiresAt = &exp
	j.UpdatedAt = now
	c.jobs[jobID] = j
	return nil
}

func (c *memoryCore) completeLocked(jobID domain.JobID, resultJSON []byte, now time.Time) error {
	j, ok := c.jobs[jobID]
	if !ok {
		return domain.NewError(domain.ErrJobUnavailable, "job not found", nil)
	}
	j.Status = domain.JobSucceeded
	j.ResultJSON = resultJSON
	j.CompletedAt = &now
	j.UpdatedAt = now
	c.jobs[jobID] = j
	return nil
}

func (c *memoryCore) failLocked(jobID domain.JobID, failure domain.JobFailure, now time.Time) error {
	j, ok := c.jobs[jobID]
	if !ok {
		return domain.NewError(domain.ErrJobUnavailable, "job not found", nil)
	}
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
	c.jobs[jobID] = j
	return nil
}

func (c *memoryCore) cancelLocked(jobID domain.JobID, now time.Time) error {
	j, ok := c.jobs[jobID]
	if !ok {
		return domain.NewError(domain.ErrJobUnavailable, "job not found", nil)
	}
	j.Status = domain.JobCancelled
	j.CompletedAt = &now
	j.UpdatedAt = now
	c.jobs[jobID] = j
	return nil
}

func (c *memoryCore) recoverExpiredLocked(now time.Time) []domain.JobID {
	out := []domain.JobID{}
	for id, j := range c.jobs {
		if j.LeaseExpiresAt != nil && j.LeaseExpiresAt.Before(now) && !j.Terminal() {
			j.Status = domain.JobInterrupted
			j.LeaseOwner = ""
			j.LeaseExpiresAt = nil
			j.AvailableAt = now
			j.UpdatedAt = now
			c.jobs[id] = j
			out = append(out, id)
		}
	}
	return out
}

func (c *memoryCore) depthLocked() int {
	n := 0
	for _, j := range c.jobs {
		if !j.Terminal() {
			n++
		}
	}
	return n
}
