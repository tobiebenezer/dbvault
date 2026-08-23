package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

// recordingQueue wraps a queue and counts Renew heartbeats.
type recordingQueue struct {
	ports.JobQueue
	renews   atomic.Int64
	failures atomic.Int64
}

func (r *recordingQueue) Renew(ctx context.Context, id domain.JobID, worker string, d time.Duration) error {
	r.renews.Add(1)
	return r.JobQueue.Renew(ctx, id, worker, d)
}

func (r *recordingQueue) Fail(ctx context.Context, id domain.JobID, f domain.JobFailure) error {
	r.failures.Add(1)
	return r.JobQueue.Fail(ctx, id, f)
}

func TestWorkerPoolExecutesAndCompletes(t *testing.T) {
	fq := newFakeQueue()
	q := &recordingQueue{JobQueue: fq}
	_ = q.Enqueue(context.Background(), domain.Job{ID: "j1", Type: domain.JobBackup})

	var executed int32
	pool := NewWorkerPool(WorkerOptions{
		Queue: q,
		Executor: ExecutorFunc(func(context.Context, domain.Job) ([]byte, error) {
			atomic.AddInt32(&executed, 1)
			return []byte(`{"ok":true}`), nil
		}),
		Workers: 2,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if fq.snapshot("j1").Status == domain.JobSucceeded && q.renews.Load() >= 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	pool.Wait()

	j := fq.snapshot("j1")
	if j.Status != domain.JobSucceeded || string(j.ResultJSON) != `{"ok":true}` {
		t.Fatalf("job not completed cleanly: %+v result=%s", j.Status, j.ResultJSON)
	}
	if atomic.LoadInt32(&executed) != 1 {
		t.Fatalf("executor ran %d times", executed)
	}
}

func TestWorkerHeartbeatRenewsDuringLongExecution(t *testing.T) {
	fq := newFakeQueue()
	q := &recordingQueue{JobQueue: fq}
	_ = q.Enqueue(context.Background(), domain.Job{ID: "slow", Type: domain.JobBackup})

	started := make(chan struct{})
	release := make(chan struct{})
	pool := NewWorkerPool(WorkerOptions{
		Queue: q,
		Executor: ExecutorFunc(func(context.Context, domain.Job) ([]byte, error) {
			close(started)
			<-release // execution outlives the lease without heartbeats
			return []byte(`{}`), nil
		}),
		Workers:       1,
		LeaseDuration: 2 * time.Second,
		PollInterval:  20 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("executor never started")
	}
	time.Sleep(2200 * time.Millisecond) // lease would expire at 2s without renewal
	close(release)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && fq.snapshot("slow").Status != domain.JobSucceeded {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	pool.Wait()

	if n := q.renews.Load(); n == 0 {
		t.Fatal("expected at least one heartbeat renew during execution")
	}
	if fq.snapshot("slow").Status != domain.JobSucceeded {
		t.Fatalf("job status=%v", fq.snapshot("slow").Status)
	}
}

func TestWorkerFailsJobWithRetryableError(t *testing.T) {
	fq := newFakeQueue()
	q := &recordingQueue{JobQueue: fq}
	_ = q.Enqueue(context.Background(), domain.Job{ID: "bad", Type: domain.JobBackup, MaximumAttempts: 3})

	pool := NewWorkerPool(WorkerOptions{
		Queue: q,
		Executor: ExecutorFunc(func(context.Context, domain.Job) ([]byte, error) {
			return nil, domain.NewError(domain.ErrSnapshotFailed, "boom", nil)
		}),
		Workers:      1,
		PollInterval: 20 * time.Millisecond,
	})
	settled := make(chan domain.Job, 1)
	_ = settled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && q.failures.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	pool.Wait()

	j := fq.snapshot("bad")
	if j.Status != domain.JobFailed && j.Status != domain.JobRetrying {
		t.Fatalf("job must land in failed/retrying, got %v", j.Status)
	}
	if q.failures.Load() != 1 {
		t.Fatalf("Fail recorded %d times", q.failures.Load())
	}
}

func TestWorkerNoExecutorFailsJob(t *testing.T) {
	fq := newFakeQueue()
	q := &recordingQueue{JobQueue: fq}
	_ = q.Enqueue(context.Background(), domain.Job{ID: "nx", Type: domain.JobBackup})

	pool := NewWorkerPool(WorkerOptions{Queue: q, Workers: 1, PollInterval: 20 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && fq.snapshot("nx").Status == domain.JobPending {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	pool.Wait()
	if st := fq.snapshot("nx").Status; st != domain.JobFailed {
		t.Fatalf("no-executor job must fail loudly, got %v", st)
	}
}

func TestTwoWorkersNeverShareOneJob(t *testing.T) {
	fq := newFakeQueue()
	_ = fq.Enqueue(context.Background(), domain.Job{ID: "only-one", Type: domain.JobBackup})

	var mu sync.Mutex
	executions := 0
	block := make(chan struct{})
	pool := NewWorkerPool(WorkerOptions{
		Queue: fq,
		Executor: ExecutorFunc(func(context.Context, domain.Job) ([]byte, error) {
			mu.Lock()
			executions++
			mu.Unlock()
			<-block
			return []byte(`{}`), nil
		}),
		Workers:      2,
		PollInterval: 10 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	close(block)
	cancel()
	pool.Wait()

	mu.Lock()
	defer mu.Unlock()
	if executions != 1 {
		t.Fatalf("one job executed %d times across two workers", executions)
	}
}
