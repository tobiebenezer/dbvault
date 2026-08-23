package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

func TestQueueLeasesOneWinner(t *testing.T) {
	q := New()
	job := domain.Job{ID: "job1", Type: domain.JobBackup, Status: domain.JobPending, AvailableAt: time.Now().Add(-time.Second), CreatedAt: time.Now(), MaximumAttempts: 3}
	if err := q.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := q.LeaseNext(context.Background(), "w1", []domain.JobType{domain.JobBackup}, time.Minute); err != nil || !ok {
		t.Fatalf("first lease ok=%v err=%v", ok, err)
	}
	if _, ok, err := q.LeaseNext(context.Background(), "w2", []domain.JobType{domain.JobBackup}, time.Minute); err != nil || ok {
		t.Fatalf("second lease ok=%v err=%v", ok, err)
	}
}

func TestQueueRecoverExpired(t *testing.T) {
	q := New()
	now := time.Now()
	exp := now.Add(-time.Minute)
	_ = q.Enqueue(context.Background(), domain.Job{ID: "job1", Type: domain.JobBackup, Status: domain.JobLeased, AvailableAt: now.Add(-time.Hour), LeaseOwner: "w1", LeaseExpiresAt: &exp, CreatedAt: now, MaximumAttempts: 3})
	ids, err := q.RecoverExpired(context.Background(), now)
	if err != nil || len(ids) != 1 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}

// TestQueueDurabilityAcrossReopen proves jobs survive a full process restart:
// enqueue, close, reopen from the same path, lease. Runs under both build
// tags (restricted JSON-file store; prod SQLite table). Under a
// CGO_ENABLED=0 production compile go-sqlite3 degrades to a non-functional
// stub, so that exact configuration skips instead of failing.
func TestQueueDurabilityAcrossReopen(t *testing.T) {
	probe, err := Open(t.TempDir() + "/probe.db")
	if err != nil {
		if sqliteDriverUnavailable(err) {
			t.Skip("prod sqlite3 driver requires cgo; unavailable under this build gate")
		}
		t.Fatal(err)
	}
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}

	path := t.TempDir() + "/jobs.db"
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	job := domain.Job{ID: "durable-1", Type: domain.JobBackup, Status: domain.JobPending, AvailableAt: time.Now().Add(-time.Second), CreatedAt: time.Now(), MaximumAttempts: 3}
	if err := first.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	got, ok, err := second.LeaseNext(context.Background(), "w1", []domain.JobType{domain.JobBackup}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("reopen lease ok=%v err=%v", ok, err)
	}
	if got.ID != job.ID || got.Type != domain.JobBackup {
		t.Fatalf("reopened job mismatch: %+v", got)
	}
}

func sqliteDriverUnavailable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "requires cgo")
}
