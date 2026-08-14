// Package sqlite provides a dependency-free durable-job adapter shape.
// In restricted builds it is an in-memory queue; production can replace the
// internals with atomic SQLite leasing without changing application services.
package sqlite

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

type Queue struct {
	mu   sync.Mutex
	jobs map[domain.JobID]domain.Job
}

func New() *Queue { return &Queue{jobs: map[domain.JobID]domain.Job{}} }

func (q *Queue) Enqueue(ctx context.Context, job domain.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job.Status == "" {
		job.Status = domain.JobPending
	}
	if job.MaximumAttempts == 0 {
		job.MaximumAttempts = 5
	}
	if job.AvailableAt.IsZero() {
		job.AvailableAt = time.Now().UTC()
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	job.UpdatedAt = time.Now().UTC()
	q.jobs[job.ID] = job
	return nil
}

func (q *Queue) LeaseNext(ctx context.Context, workerID string, types []domain.JobType, leaseDuration time.Duration) (domain.Job, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now().UTC()
	allowed := map[domain.JobType]bool{}
	for _, t := range types {
		allowed[t] = true
	}
	candidates := []domain.Job{}
	for _, j := range q.jobs {
		if j.Terminal() || j.Status == domain.JobCancelled {
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
		return domain.Job{}, false, nil
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
	q.jobs[j.ID] = j
	return j, true, nil
}

func (q *Queue) Renew(ctx context.Context, jobID domain.JobID, workerID string, leaseDuration time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	j := q.jobs[jobID]
	if j.LeaseOwner != workerID {
		return domain.NewError(domain.ErrJobUnavailable, "job lease owner mismatch", nil)
	}
	exp := time.Now().UTC().Add(leaseDuration)
	j.LeaseExpiresAt = &exp
	j.UpdatedAt = time.Now().UTC()
	q.jobs[jobID] = j
	return nil
}
func (q *Queue) Complete(ctx context.Context, jobID domain.JobID, resultJSON []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	j := q.jobs[jobID]
	now := time.Now().UTC()
	j.Status = domain.JobSucceeded
	j.ResultJSON = resultJSON
	j.CompletedAt = &now
	j.UpdatedAt = now
	q.jobs[jobID] = j
	return nil
}
func (q *Queue) Fail(ctx context.Context, jobID domain.JobID, failure domain.JobFailure) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	j := q.jobs[jobID]
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
	q.jobs[jobID] = j
	return nil
}
func (q *Queue) Cancel(ctx context.Context, jobID domain.JobID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	j := q.jobs[jobID]
	now := time.Now().UTC()
	j.Status = domain.JobCancelled
	j.CompletedAt = &now
	j.UpdatedAt = now
	q.jobs[jobID] = j
	return nil
}
func (q *Queue) RecoverExpired(ctx context.Context, now time.Time) ([]domain.JobID, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := []domain.JobID{}
	for id, j := range q.jobs {
		if j.LeaseExpiresAt != nil && j.LeaseExpiresAt.Before(now) && !j.Terminal() {
			j.Status = domain.JobInterrupted
			j.LeaseOwner = ""
			j.LeaseExpiresAt = nil
			j.AvailableAt = now
			j.UpdatedAt = now
			q.jobs[id] = j
			out = append(out, id)
		}
	}
	return out, nil
}
