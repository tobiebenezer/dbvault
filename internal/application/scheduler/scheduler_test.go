package scheduler

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/config"
	"github.com/dbvault/dbvault/internal/domain"
)

// fakeQueue is a thread-safe ports.JobQueue test double: worker pools run
// concurrent goroutines, so every method takes the lock.
type fakeQueue struct {
	sync.Mutex
	mu      map[domain.JobID]domain.Job
	order   []domain.JobID
	enqDup  int
	recover []domain.JobID
}

func newFakeQueue() *fakeQueue { return &fakeQueue{mu: map[domain.JobID]domain.Job{}} }

// snapshot returns a copy of one job for assertions from the test goroutine.
func (f *fakeQueue) snapshot(id domain.JobID) domain.Job {
	f.Lock()
	defer f.Unlock()
	return f.mu[id]
}

func (f *fakeQueue) Enqueue(_ context.Context, job domain.Job) error {
	f.Lock()
	defer f.Unlock()
	if existing, ok := f.mu[job.ID]; ok && !existing.Terminal() {
		f.enqDup++
		return nil
	}
	if job.Status == "" {
		job.Status = domain.JobPending
	}
	if job.AvailableAt.IsZero() {
		job.AvailableAt = time.Now().UTC()
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	f.mu[job.ID] = job
	f.order = append(f.order, job.ID)
	return nil
}

func (f *fakeQueue) LeaseNext(_ context.Context, workerID string, _ []domain.JobType, lease time.Duration) (domain.Job, bool, error) {
	f.Lock()
	defer f.Unlock()
	now := time.Now().UTC()
	for _, id := range f.order {
		j := f.mu[id]
		if j.Terminal() || j.Status == domain.JobLeased {
			continue
		}
		exp := now.Add(lease)
		j.LeaseOwner, j.LeaseExpiresAt, j.Status = workerID, &exp, domain.JobLeased
		f.mu[id] = j
		return j, true, nil
	}
	return domain.Job{}, false, nil
}

func (f *fakeQueue) Renew(_ context.Context, id domain.JobID, workerID string, lease time.Duration) error {
	f.Lock()
	defer f.Unlock()
	j, ok := f.mu[id]
	if !ok || j.LeaseOwner != workerID {
		return domain.NewError(domain.ErrJobUnavailable, "owner mismatch", nil)
	}
	exp := time.Now().UTC().Add(lease)
	j.LeaseExpiresAt = &exp
	f.mu[id] = j
	return nil
}

func (f *fakeQueue) Complete(_ context.Context, id domain.JobID, result []byte) error {
	f.Lock()
	defer f.Unlock()
	j := f.mu[id]
	j.Status, j.ResultJSON = domain.JobSucceeded, result
	f.mu[id] = j
	return nil
}

func (f *fakeQueue) Fail(_ context.Context, id domain.JobID, failure domain.JobFailure) error {
	f.Lock()
	defer f.Unlock()
	j := f.mu[id]
	j.Attempt++
	j.LastError = failure.Message
	j.Status = domain.JobFailed
	f.mu[id] = j
	return nil
}

func (f *fakeQueue) Cancel(_ context.Context, id domain.JobID) error {
	f.Lock()
	defer f.Unlock()
	j := f.mu[id]
	j.Status = domain.JobCancelled
	f.mu[id] = j
	return nil
}

func (f *fakeQueue) RecoverExpired(_ context.Context, now time.Time) ([]domain.JobID, error) {
	f.Lock()
	defer f.Unlock()
	out := []domain.JobID{}
	for _, id := range f.order {
		j := f.mu[id]
		if j.LeaseExpiresAt != nil && j.LeaseExpiresAt.Before(now) && !j.Terminal() {
			j.Status = domain.JobInterrupted
			f.mu[id] = j
			out = append(out, id)
		}
	}
	f.recover = append(f.recover, out...)
	return out, nil
}

func fixedNow(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestSpecsFromConfigEverySeconds(t *testing.T) {
	var cfg config.Config
	cfg.Schedule.Enabled = true
	cfg.Schedule.EverySeconds = 30
	cfg.Schedules = []config.ScheduleConfig{{ID: "legacy-schedule", Source: "src1", Operation: "logical_backup"}}
	specs := SpecsFromConfig(cfg)
	if len(specs) != 1 {
		t.Fatalf("specs=%v", specs)
	}
	if specs[0].EverySeconds != 30 || specs[0].SourceID != "src1" {
		t.Fatalf("spec=%+v", specs[0])
	}
}

func TestSchedulerEnqueueOnTickAndDedupe(t *testing.T) {
	base := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
	q := newFakeQueue()
	s := New(Options{Queue: q, Specs: []Spec{{ID: "s1", SourceID: "src1", Operation: "logical_backup", EverySeconds: 10}}, Now: fixedNow(base), Tick: time.Second})

	if got := s.Tick(context.Background(), base); len(got) != 1 {
		t.Fatalf("first tick enqueued %d jobs", len(got))
	}
	// Still in flight: no re-enqueue inside the same window or the next one.
	if got := s.Tick(context.Background(), base.Add(5*time.Second)); len(got) != 0 {
		t.Fatalf("in-flight tick must not enqueue: %d", len(got))
	}
	if got := s.Tick(context.Background(), base.Add(11*time.Second)); len(got) != 0 {
		t.Fatalf("unsettled schedule must not double-book: %d", len(got))
	}
	// Settle then a due window exists -> next occurrence fires exactly once.
	s.Settle(domain.Job{ID: gotFirst(t, q)})
	next := s.Tick(context.Background(), base.Add(12*time.Second))
	if len(next) != 1 || next[0].ID == q.snapshot(q.order[0]).ID {
		t.Fatalf("expected new occurrence after settle, got %v", next)
	}
	if q.enqDup != 0 {
		t.Fatalf("unexpected duplicate enqueues observed upstream: %d", q.enqDup)
	}
}

// gotFirst leases the first enqueued job to learn its ID as stored.
func gotFirst(t *testing.T, q *fakeQueue) domain.JobID {
	t.Helper()
	if len(q.order) == 0 {
		t.Fatal("no jobs enqueued")
	}
	return q.order[0]
}

func TestSchedulerCronFiresAfterBaseline(t *testing.T) {
	base := time.Date(2026, 8, 22, 9, 7, 0, 0, time.UTC)
	q := newFakeQueue()
	s := New(Options{Queue: q, Specs: []Spec{{ID: "cron1", SourceID: "src1", Operation: "logical_backup", Cron: "*/15 * * * *"}}, Now: fixedNow(base), Tick: time.Second})

	if got := s.Tick(context.Background(), base.Add(3*time.Minute)); len(got) != 0 {
		t.Fatalf("09:10 not yet due: %d", len(got))
	}
	got := s.Tick(context.Background(), base.Add(8*time.Minute)) // 09:15 window passed
	if len(got) != 1 {
		t.Fatalf("cron occurrence at 09:15 must fire once, got %d", len(got))
	}
	fire := time.Date(2026, 8, 22, 9, 15, 0, 0, time.UTC)
	wantID := domain.JobID("sched-cron1-" + strconv.FormatInt(fire.Unix(), 10))
	if got[0].ID != wantID {
		t.Fatalf("job id=%s want=%s", got[0].ID, wantID)
	}
}

func TestSchedulerRecoverExpiredOnStartup(t *testing.T) {
	base := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
	stale := base.Add(-time.Hour)
	exp := stale.Add(-time.Minute)
	q := newFakeQueue()
	_ = q.Enqueue(context.Background(), domain.Job{ID: "stale-1", Type: domain.JobBackup, Status: domain.JobLeased, LeaseOwner: "dead-worker", LeaseExpiresAt: &exp, AvailableAt: stale, CreatedAt: stale})
	s := New(Options{Queue: q, Specs: nil, Now: fixedNow(base), Tick: time.Second})
	if err := s.recoverExpired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(q.recover) != 1 || q.recover[0] != "stale-1" {
		t.Fatalf("recovered=%v", q.recover)
	}
}

func TestJobForFireDeterministic(t *testing.T) {
	spec := Spec{ID: "s1", SourceID: "srcA", Operation: "logical_backup"}
	fire := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
	a, b := JobForFire(spec, fire), JobForFire(spec, fire)
	if a.ID != b.ID || a.ResourceID != "srcA" || a.Type != domain.JobBackup {
		t.Fatalf("jobs not deterministic/complete: %+v %+v", a, b)
	}
}
