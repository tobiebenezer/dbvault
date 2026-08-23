package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

// Executor runs one leased job and returns its result JSON. Unknown job types
// fail with a clear reason.
type Executor interface {
	Execute(ctx context.Context, job domain.Job) ([]byte, error)
}

// ExecutorFunc adapts a function to Executor.
type ExecutorFunc func(ctx context.Context, job domain.Job) ([]byte, error)

func (f ExecutorFunc) Execute(ctx context.Context, job domain.Job) ([]byte, error) {
	return f(ctx, job)
}

// WorkerOptions configures the pool. Zero values get production defaults.
type WorkerOptions struct {
	Queue         ports.JobQueue
	Executor      Executor
	Workers       int              // default 1
	Types         []domain.JobType // default [JobBackup]
	LeaseDuration time.Duration    // default 5m
	PollInterval  time.Duration    // default 500ms
	Logger        *slog.Logger
	OnSettled     func(job domain.Job, execErr error, result []byte) // invoked after Complete/Fail with executor result JSON
}

// WorkerPool leases jobs from the queue and drives them to completion. Each
// worker heartbeats Renew at leaseDuration/3 while its job executes so long
// backups are not stolen by lease expiry.
type WorkerPool struct {
	opts WorkerOptions
	log  *slog.Logger
	wg   sync.WaitGroup
}

func NewWorkerPool(opts WorkerOptions) *WorkerPool {
	if opts.Workers <= 0 {
		opts.Workers = 1
	}
	if opts.LeaseDuration <= 0 {
		opts.LeaseDuration = 5 * time.Minute
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 500 * time.Millisecond
	}
	if len(opts.Types) == 0 {
		opts.Types = []domain.JobType{domain.JobBackup}
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	return &WorkerPool{opts: opts, log: log}
}

// Start launches the workers; they stop when ctx is cancelled.
func (p *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < p.opts.Workers; i++ {
		id := fmt.Sprintf("worker-%d", i+1)
		p.wg.Add(1)
		go p.loop(ctx, id)
	}
}

// Wait blocks until every worker has returned.
func (p *WorkerPool) Wait() { p.wg.Wait() }

func (p *WorkerPool) loop(ctx context.Context, workerID string) {
	defer p.wg.Done()
	ticker := time.NewTicker(p.opts.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.step(ctx, workerID)
		}
	}
}

func (p *WorkerPool) step(ctx context.Context, workerID string) {
	job, ok, err := p.opts.Queue.LeaseNext(ctx, workerID, p.opts.Types, p.opts.LeaseDuration)
	if err != nil {
		p.log.Warn("worker lease failed", "worker", workerID, "error", err)
		return
	}
	if !ok {
		return
	}
	p.log.Info("job leased", "worker", workerID, "job_id", job.ID, "type", job.Type, "source", job.ResourceID, "attempt", job.Attempt+1)

	// Heartbeat keeps the lease alive during execution.
	stopHeartbeat := make(chan struct{})
	heartbeatDone := make(chan struct{})
	interval := p.opts.LeaseDuration / 3
	if interval < time.Second {
		interval = time.Second
	}
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ticker.C:
				if err := p.opts.Queue.Renew(ctx, job.ID, workerID, p.opts.LeaseDuration); err != nil {
					p.log.Warn("lease renew failed", "worker", workerID, "job_id", job.ID, "error", err)
					return
				}
			}
		}
	}()

	resultJSON, execErr := func() (result []byte, execErr error) {
		defer func() {
			if r := recover(); r != nil {
				execErr = fmt.Errorf("executor panicked: %v", r)
			}
		}()
		if p.opts.Executor == nil {
			return nil, fmt.Errorf("no executor configured for job type %q", job.Type)
		}
		return p.opts.Executor.Execute(ctx, job)
	}()

	close(stopHeartbeat)
	<-heartbeatDone

	var settleErr error
	if execErr != nil {
		settleErr = p.opts.Queue.Fail(ctx, job.ID, domain.JobFailure{
			Code:      domain.ErrJobUnavailable,
			Message:   execErr.Error(),
			Retryable: ctx.Err() == nil,
			At:        time.Now().UTC(),
		})
		if settleErr != nil {
			p.log.Error("job failure recording failed", "job_id", job.ID, "error", settleErr)
		}
		p.log.Warn("job failed", "worker", workerID, "job_id", job.ID, "source", job.ResourceID, "error", execErr)
	} else {
		settleErr = p.opts.Queue.Complete(ctx, job.ID, resultJSON)
		if settleErr != nil {
			p.log.Error("job completion recording failed", "job_id", job.ID, "error", settleErr)
		} else {
			p.log.Info("job completed", "worker", workerID, "job_id", job.ID, "source", job.ResourceID)
		}
	}
	if p.opts.OnSettled != nil {
		p.opts.OnSettled(job, execErr, resultJSON)
	}
}
