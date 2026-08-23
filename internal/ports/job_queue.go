package ports

import (
	"context"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

type JobQueue interface {
	Enqueue(ctx context.Context, job domain.Job) error
	LeaseNext(ctx context.Context, workerID string, types []domain.JobType, leaseDuration time.Duration) (domain.Job, bool, error)
	Renew(ctx context.Context, jobID domain.JobID, workerID string, leaseDuration time.Duration) error
	Complete(ctx context.Context, jobID domain.JobID, resultJSON []byte) error
	Fail(ctx context.Context, jobID domain.JobID, failure domain.JobFailure) error
	Cancel(ctx context.Context, jobID domain.JobID) error
	RecoverExpired(ctx context.Context, now time.Time) ([]domain.JobID, error)
}

// QueueDepthReporter optionally exposes the number of live jobs in a queue.
type QueueDepthReporter interface {
	Depth(ctx context.Context) (int, error)
}

type FaultInjector interface {
	Check(ctx context.Context, point string, metadata map[string]string) error
}

type NoopFaultInjector struct{}

func (NoopFaultInjector) Check(context.Context, string, map[string]string) error { return nil }
