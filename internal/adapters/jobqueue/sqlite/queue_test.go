package sqlite

import (
	"context"
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
