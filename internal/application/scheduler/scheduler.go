package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/config"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

// Spec is one resolved schedule: a source to back up and when.
type Spec struct {
	ID           string
	SourceID     string
	Operation    string
	Cron         string
	Timezone     string
	EverySeconds int
}

const legacyScheduleID = "legacy-schedule"

// SpecsFromConfig resolves enabled schedules from cfg.Schedules, falling back
// to the legacy schedule.enabled/every_seconds/cron/timezone knobs (which
// Normalize() already mirrors into Schedules[0] as "legacy-schedule").
func SpecsFromConfig(cfg config.Config) []Spec {
	out := []Spec{}
	for _, sc := range cfg.Schedules {
		if sc.Enabled != nil && !*sc.Enabled {
			continue
		}
		id := sc.ID
		if id == "" {
			id = sc.Source + "-" + sc.Operation
		}
		spec := Spec{ID: id, SourceID: sc.Source, Operation: sc.Operation, Cron: sc.Cron, Timezone: sc.Timezone}
		if spec.Operation == "" {
			spec.Operation = "logical_backup"
		}
		if id == legacyScheduleID && spec.Cron == "" {
			spec.EverySeconds = cfg.Schedule.EverySeconds
		}
		if spec.Cron == "" && spec.EverySeconds <= 0 {
			continue
		}
		out = append(out, spec)
	}
	return out
}

// Options configures the Scheduler. Queue is required; zero-value fields get
// production defaults.
type Options struct {
	Queue  ports.JobQueue
	Specs  []Spec
	Tick   time.Duration // bounded to >= 1s
	Logger *slog.Logger

	// Now allows tests to drive time with a fake clock.
	Now func() time.Time
}

type Scheduler struct {
	opts Options
	log  *slog.Logger

	exprs map[string]*cronExpr
	locs  map[string]*time.Location

	mu       sync.Mutex
	inflight map[string]domain.JobID // schedule ID -> live job ID
	lastFire map[string]time.Time
	baseline map[string]time.Time // schedule ID -> scheduler start instant
}

func New(opts Options) *Scheduler {
	if opts.Tick < time.Second {
		opts.Tick = time.Second
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	s := &Scheduler{
		opts:     opts,
		log:      log,
		exprs:    map[string]*cronExpr{},
		locs:     map[string]*time.Location{},
		inflight: map[string]domain.JobID{},
		lastFire: map[string]time.Time{},
		baseline: map[string]time.Time{},
	}
	for _, spec := range opts.Specs {
		if spec.Cron == "" {
			continue
		}
		s.baseline[spec.ID] = now()
		if e, err := parseCron(spec.Cron); err == nil {
			s.exprs[spec.ID] = e
		} else {
			log.Warn("scheduler skipping schedule with invalid cron", "schedule", spec.ID, "cron", spec.Cron, "error", err)
		}
		loc, err := loadLocation(spec.Timezone)
		if err != nil {
			log.Warn("scheduler falling back to UTC", "schedule", spec.ID, "timezone", spec.Timezone, "error", err)
			loc = time.UTC
		}
		s.locs[spec.ID] = loc
	}
	return s
}

func loadLocation(name string) (*time.Location, error) {
	switch name {
	case "", "UTC":
		return time.UTC, nil
	case "Local":
		return time.Local, nil
	default:
		return time.LoadLocation(name)
	}
}

// Run drives the scheduler until ctx is cancelled. RecoverExpired runs at
// startup and on every tick so crashed leases re-enter the queue.
func (s *Scheduler) Run(ctx context.Context) {
	if err := s.recoverExpired(ctx); err != nil {
		s.log.Warn("scheduler lease recovery failed", "error", err)
	}
	ticker := time.NewTicker(s.opts.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Tick(ctx, s.opts.Now())
		}
	}
}

// Tick performs one scheduling pass at the given instant and returns the jobs
// it enqueued. Exported for deterministic testing with a fake clock.
func (s *Scheduler) Tick(ctx context.Context, now time.Time) []domain.Job {
	enqueued := []domain.Job{}
	for _, spec := range s.opts.Specs {
		fire, ok := s.dueFire(spec, now)
		if !ok {
			continue
		}
		if s.live(spec.ID) {
			continue
		}
		job, err := s.enqueueBackupJob(ctx, spec, fire)
		if err != nil {
			s.log.Warn("scheduler enqueue failed", "schedule", spec.ID, "error", err)
			continue
		}
		s.setLastFire(spec.ID, fire)
		s.markInflight(spec.ID, job.ID)
		s.log.Info("job enqueued by schedule", "schedule", spec.ID, "source", spec.SourceID, "job_id", job.ID, "fire_at", fire.Format(time.RFC3339))
		enqueued = append(enqueued, job)
	}
	return enqueued
}

// dueFire reports the schedule occurrence that has come due at now and has
// not been handled yet.
func (s *Scheduler) dueFire(spec Spec, now time.Time) (time.Time, bool) {
	last := s.getLastFire(spec.ID)
	if spec.Cron != "" {
		e, ok := s.exprs[spec.ID]
		if !ok {
			return time.Time{}, false
		}
		from := last
		if from.IsZero() {
			// Nothing is due before the scheduler started; this avoids
			// replaying windows missed while the process was down.
			from = s.baseline[spec.ID]
			if from.IsZero() {
				from = now
			}
		}
		next := e.NextAfter(from, s.locs[spec.ID])
		if next.IsZero() || next.After(now) {
			return time.Time{}, false
		}
		return next, true
	}
	window := time.Duration(spec.EverySeconds) * time.Second
	if window <= 0 {
		return time.Time{}, false
	}
	current := now.Truncate(window)
	if !last.IsZero() && !current.After(last) {
		return time.Time{}, false
	}
	return current, true
}

// JobForFire builds the deterministic job ID for one schedule occurrence so
// restart-safe dedupe works through the queue's idempotent Enqueue.
func JobForFire(spec Spec, fire time.Time) domain.Job {
	payload, _ := json.Marshal(map[string]string{"source": spec.SourceID, "operation": spec.Operation, "schedule": spec.ID})
	return domain.Job{
		ID:          domain.JobID(fmt.Sprintf("sched-%s-%d", spec.ID, fire.Unix())),
		Type:        domain.JobBackup,
		Status:      domain.JobPending,
		ResourceID:  spec.SourceID,
		PayloadJSON: payload,
		Priority:    0,
	}
}

func (s *Scheduler) enqueueBackupJob(ctx context.Context, spec Spec, fire time.Time) (domain.Job, error) {
	job := JobForFire(spec, fire)
	if err := s.opts.Queue.Enqueue(ctx, job); err != nil {
		return domain.Job{}, err
	}
	return job, nil
}

func (s *Scheduler) recoverExpired(ctx context.Context) error {
	recovered, err := s.opts.Queue.RecoverExpired(ctx, s.opts.Now())
	if err != nil {
		return err
	}
	if len(recovered) > 0 {
		s.log.Warn("recovered expired leases", "count", len(recovered), "job_ids", recovered)
	}
	return nil
}

func (s *Scheduler) live(scheduleID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.inflight[scheduleID]
	return ok
}

// Settle marks an in-flight job terminal; the worker pool invokes this via
// OnSettled so schedules can enqueue their next occurrence.
func (s *Scheduler) Settle(job domain.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for scheduleID, id := range s.inflight {
		if id == job.ID {
			delete(s.inflight, scheduleID)
		}
	}
}

func (s *Scheduler) markInflight(scheduleID string, jobID domain.JobID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inflight[scheduleID] = jobID
}

func (s *Scheduler) getLastFire(id string) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastFire[id]
}

func (s *Scheduler) setLastFire(id string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastFire[id] = t
}
