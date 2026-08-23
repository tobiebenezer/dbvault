package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

// Hook runs after a job settles. execErr is nil when the job succeeded.
// Hooks must never panic outward and must never block the worker long.
type Hook func(ctx context.Context, job domain.Job, execErr error)

// Hooks is a small post-job hook registry invoked by the worker pool's
// OnSettled callback.
type Hooks struct {
	mu    sync.Mutex
	hooks []Hook
}

func (h *Hooks) Add(fn Hook) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hooks = append(h.hooks, fn)
}

// Run invokes every hook; panics and errors inside a hook are logged, never
// propagated — notifications and GC triggers may not break job settlement.
func (h *Hooks) Run(ctx context.Context, log *slog.Logger, job domain.Job, execErr error) {
	h.mu.Lock()
	hooks := append([]Hook(nil), h.hooks...)
	h.mu.Unlock()
	if log == nil {
		log = slog.Default()
	}
	for _, fn := range hooks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error("job hook panicked", "job_id", job.ID, "panic", r)
				}
			}()
			fn(ctx, job, execErr)
		}()
	}
}

// GCTrigger enqueues garbage-collection jobs after successful backups and,
// optionally, on a periodic sweep interval as a fallback.
type GCTrigger struct {
	Queue      ports.JobQueue
	Repository string
	// Interval > 0 enables the periodic fallback sweep.
	Interval time.Duration
	// Now allows tests to drive time deterministically.
	Now func() time.Time

	mu         sync.Mutex
	lastBucket int64
}

func gcJob(id, repository, reason string) domain.Job {
	payload, _ := json.Marshal(map[string]string{"repository": repository, "reason": reason})
	return domain.Job{ID: domain.JobID(id), Type: domain.JobGarbageCollection, Status: domain.JobPending, ResourceID: repository, PayloadJSON: payload, Priority: -1}
}

// AfterJob enqueues one GC job per successful backup completion. The job ID
// is deterministic so queue-side idempotent Enqueue dedupes replays.
func (g *GCTrigger) AfterJob(ctx context.Context, job domain.Job, execErr error) {
	if execErr != nil || job.Type != domain.JobBackup || g.Queue == nil {
		return
	}
	_ = g.Queue.Enqueue(ctx, gcJob(fmt.Sprintf("gc-after-%s", job.ID), g.Repository, "post-backup"))
}

// SweepLoop runs the periodic fallback sweep until ctx is cancelled. One GC
// job is enqueued per interval bucket; restarts dedupe through the bucketed
// deterministic ID.
func (g *GCTrigger) SweepLoop(ctx context.Context, log *slog.Logger) {
	if g.Interval <= 0 || g.Queue == nil {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	ticker := time.NewTicker(g.Interval)
	defer ticker.Stop()
	now := g.now()
	g.sweepOnce(ctx, now, log)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.sweepOnce(ctx, g.now(), log)
		}
	}
}

func (g *GCTrigger) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now().UTC()
}

func (g *GCTrigger) sweepOnce(ctx context.Context, now time.Time, log *slog.Logger) {
	bucket := now.Unix() / int64(g.Interval.Seconds())
	g.mu.Lock()
	if bucket == g.lastBucket {
		g.mu.Unlock()
		return
	}
	g.lastBucket = bucket
	g.mu.Unlock()
	j := gcJob(fmt.Sprintf("gc-sweep-%d", bucket), g.Repository, "periodic-sweep")
	if err := g.Queue.Enqueue(ctx, j); err != nil {
		log.Warn("gc sweep enqueue failed", "error", err)
		return
	}
	log.Info("gc sweep scheduled", "job_id", j.ID)
}
