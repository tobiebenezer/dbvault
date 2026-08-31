package productexperience

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jobqueue "github.com/dbvault/dbvault/internal/adapters/jobqueue/sqlite"
	"github.com/dbvault/dbvault/internal/application/scheduler"
	"github.com/dbvault/dbvault/internal/domain"
)

// openDurableQueue backs a test with a real durable queue file so the tests
// exercise the same persistence path the appliance uses in production.
func openDurableQueue(t *testing.T) *jobqueue.Queue {
	t.Helper()
	q, err := jobqueue.Open(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatalf("queue open: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })
	return q
}

func TestEnqueueWarehouseSyncSurvivesQueueReopen(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "jobs.db")
	q, err := jobqueue.Open(path)
	if err != nil {
		t.Fatalf("queue open: %v", err)
	}
	svc.SetJobQueue(q)
	view, err := svc.EnqueueWarehouseSync(context.Background(), "db-reopen", WarehouseSyncOptions{})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if view.JobType != "warehouse_sync" || view.Status != "queued" {
		t.Fatalf("view type=%s status=%s", view.JobType, view.Status)
	}
	if err := q.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := jobqueue.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	// Backup-only leases must not pick up the warehouse sync job.
	if _, ok, err := reopened.LeaseNext(context.Background(), "w1", []domain.JobType{domain.JobBackup}, time.Minute); ok || err != nil {
		t.Fatalf("backup lease picked warehouse job (ok=%v err=%v)", ok, err)
	}
	job, ok, err := reopened.LeaseNext(context.Background(), "w1", []domain.JobType{domain.JobWarehouseSync}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("lease after reopen: ok=%v err=%v", ok, err)
	}
	if job.Type != domain.JobWarehouseSync || job.ResourceID != "db-reopen" {
		t.Fatalf("leased job type=%s resource=%s", job.Type, job.ResourceID)
	}
	var payload struct {
		Source string `json:"source"`
		JobID  string `json:"job_id"`
	}
	if err := json.Unmarshal(job.PayloadJSON, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.Source != "db-reopen" || payload.JobID != view.ID {
		t.Fatalf("payload source=%q jobID=%q want source=%q jobID=%q", payload.Source, payload.JobID, "db-reopen", view.ID)
	}
}

func TestEnqueueWarehouseSyncFailClosedWithoutQueue(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnqueueWarehouseSync(context.Background(), "db-noqueue", WarehouseSyncOptions{}); err == nil || !strings.Contains(err.Error(), "durable scheduler not configured") {
		t.Fatalf("expected fail-closed error, got %v", err)
	}
	jobs := svc.Jobs(nil)
	if len(jobs) != 1 || jobs[0].Status != "failed" {
		t.Fatalf("job record must honestly report failure, got %+v", jobs)
	}
	if got, ok := svc.Job(jobs[0].ID); !ok || got.Status != "failed" {
		t.Fatalf("Job lookup ok=%v status=%s", ok, got.Status)
	}
}

func TestExecuteWarehouseSyncJobLifecycle(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var ran []string
	svc.warehouseSyncRunner = func(ctx context.Context, databaseID string, opts WarehouseSyncOptions) error {
		ran = append(ran, databaseID)
		return nil
	}
	view := svc.newJobRecord("warehouse_sync", "db-lc", "db-lc")
	payload, err := json.Marshal(map[string]string{"source": "db-lc", "operation": "warehouse_sync", "job_id": view.ID})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ExecuteWarehouseSyncJob(context.Background(), domain.Job{ID: "whsync-1", Type: domain.JobWarehouseSync, ResourceID: "db-lc", PayloadJSON: payload})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(ran) != 1 || ran[0] != "db-lc" {
		t.Fatalf("pipeline ran for %v", ran)
	}
	var decoded map[string]any
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["job_id"] != view.ID || decoded["status"] != "complete" {
		t.Fatalf("result=%v", decoded)
	}
	got, ok := svc.Job(view.ID)
	if !ok || got.Status != "completed" || got.Stage != "complete" || got.Percentage < 100 {
		t.Fatalf("record ok=%v status=%s stage=%s pct=%v", ok, got.Status, got.Stage, got.Percentage)
	}
}

func TestExecuteWarehouseSyncJobFailureFailsRecord(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc.warehouseSyncRunner = func(ctx context.Context, databaseID string, opts WarehouseSyncOptions) error {
		return errors.New("extract boom")
	}
	view := svc.newJobRecord("warehouse_sync", "db-fail", "db-fail")
	payload, _ := json.Marshal(map[string]string{"source": "db-fail", "operation": "warehouse_sync", "job_id": view.ID})
	if _, err := svc.ExecuteWarehouseSyncJob(context.Background(), domain.Job{ID: "whsync-2", Type: domain.JobWarehouseSync, ResourceID: "db-fail", PayloadJSON: payload}); err == nil {
		t.Fatal("expected executor error")
	}
	got, ok := svc.Job(view.ID)
	if !ok || got.Status != "failed" {
		t.Fatalf("record ok=%v status=%s", ok, got.Status)
	}
	if !strings.Contains(got.Message, "extract boom") {
		t.Fatalf("message=%q", got.Message)
	}
}

// TestExecuteWarehouseSyncJobRecreatesRecordAfterRestart covers schedule-fired
// jobs (no job_id in the payload) and crashed jobs whose px record is gone:
// the executor recreates the record so /api/v1/jobs stays accurate.
func TestExecuteWarehouseSyncJobRecreatesRecordAfterRestart(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc.warehouseSyncRunner = func(ctx context.Context, databaseID string, opts WarehouseSyncOptions) error { return nil }
	payload, _ := json.Marshal(map[string]string{"source": "db-sched", "operation": "warehouse_sync", "schedule": "wh-nightly"})
	result, err := svc.ExecuteWarehouseSyncJob(context.Background(), domain.Job{ID: "sched-wh-nightly-1", Type: domain.JobWarehouseSync, ResourceID: "db-sched", PayloadJSON: payload})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatal(err)
	}
	jobID, _ := decoded["job_id"].(string)
	if jobID == "" {
		t.Fatalf("result missing job_id: %v", decoded)
	}
	got, ok := svc.Job(jobID)
	if !ok || got.Status != "completed" || got.ResourceID != "db-sched" {
		t.Fatalf("recreated record ok=%v status=%s resource=%s", ok, got.Status, got.ResourceID)
	}
}

func TestWorkerPoolExecutesWarehouseSyncEndToEnd(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	executed := make(chan string, 4)
	svc.warehouseSyncRunner = func(ctx context.Context, databaseID string, opts WarehouseSyncOptions) error {
		executed <- databaseID
		return nil
	}
	q := openDurableQueue(t)
	svc.SetJobQueue(q)
	view, err := svc.EnqueueWarehouseSync(context.Background(), "db-e2e", WarehouseSyncOptions{})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	settled := make(chan error, 1)
	pool := scheduler.NewWorkerPool(scheduler.WorkerOptions{
		Queue:         q,
		Executor:      scheduler.ExecutorFunc(svc.ExecuteWarehouseSyncJob),
		Workers:       1,
		Types:         []domain.JobType{domain.JobWarehouseSync},
		PollInterval:  20 * time.Millisecond,
		LeaseDuration: 5 * time.Second,
		OnSettled:     func(job domain.Job, execErr error, result []byte) { settled <- execErr },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for {
		got, ok := svc.Job(view.ID)
		if ok && got.Status == "completed" && got.Percentage >= 100 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("warehouse sync job not completed in time: %+v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := <-settled; err != nil {
		t.Fatalf("settled with error: %v", err)
	}
	if db := <-executed; db != "db-e2e" {
		t.Fatalf("pipeline ran for %q", db)
	}
}

// TestWarehouseSyncJobRecoveredAfterLeaseCrash proves the durable path is
// restart-safe: a job leased right before a crash is recovered through
// RecoverExpired, re-executed, and its px record still completes.
func TestWarehouseSyncJobRecoveredAfterLeaseCrash(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc.warehouseSyncRunner = func(ctx context.Context, databaseID string, opts WarehouseSyncOptions) error { return nil }
	path := filepath.Join(t.TempDir(), "jobs.db")
	q, err := jobqueue.Open(path)
	if err != nil {
		t.Fatalf("queue open: %v", err)
	}
	svc.SetJobQueue(q)
	view, err := svc.EnqueueWarehouseSync(context.Background(), "db-crash", WarehouseSyncOptions{})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Simulate a worker leasing the job and the process dying before completion.
	leased, ok, err := q.LeaseNext(context.Background(), "worker-crash", []domain.JobType{domain.JobWarehouseSync}, 300*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("lease: ok=%v err=%v", ok, err)
	}
	if err := q.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Restart: reopen the queue and recover the expired lease.
	time.Sleep(350 * time.Millisecond)
	reopened, err := jobqueue.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	recovered, err := reopened.RecoverExpired(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if len(recovered) != 1 || recovered[0] != leased.ID {
		t.Fatalf("recovered=%v want [%s]", recovered, leased.ID)
	}

	job, ok, err := reopened.LeaseNext(context.Background(), "worker-2", []domain.JobType{domain.JobWarehouseSync}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("re-lease: ok=%v err=%v", ok, err)
	}
	result, err := svc.ExecuteWarehouseSyncJob(context.Background(), job)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if err := reopened.Complete(context.Background(), job.ID, result); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, ok := svc.Job(view.ID)
	if !ok || got.Status != "completed" || got.Percentage < 100 {
		t.Fatalf("px record after recovery ok=%v status=%s pct=%v", ok, got.Status, got.Percentage)
	}
}
