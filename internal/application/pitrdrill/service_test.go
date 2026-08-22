package pitrdrill

import (
	"context"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

func TestPITRDrillSuccess(t *testing.T) {
	svc := New(nil, nil)
	ctx := context.Background()

	targetTime := time.Now().UTC().Add(-10 * time.Minute)
	res, err := svc.Run(ctx, "pg-prod", "snap-123", targetTime)
	if err != nil {
		t.Fatalf("expected PITR drill to succeed, got %v", err)
	}

	if res.Status != domain.VerificationSucceeded {
		t.Fatalf("expected status %s, got %s", domain.VerificationSucceeded, res.Status)
	}

	if !res.IntegrityPassed || !res.QuickCheckPassed {
		t.Fatalf("expected integrity and quick check to pass")
	}

	if res.WALSegmentsReplayed <= 0 {
		t.Fatalf("expected WAL segments to be replayed")
	}
}

func TestPITRDrillInvalidTargetTime(t *testing.T) {
	svc := New(nil, nil)
	ctx := context.Background()

	// Future target time should fail
	futureTime := time.Now().UTC().Add(1 * time.Hour)
	_, err := svc.Run(ctx, "pg-prod", "snap-123", futureTime)
	if err != ErrInvalidTargetTime {
		t.Fatalf("expected ErrInvalidTargetTime, got %v", err)
	}
}
