package recovery

import (
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

func TestPlanTimestamp(t *testing.T) {
	p := Planner{}
	target := time.Date(2026, 8, 30, 15, 0, 0, 0, time.UTC)
	backups := []domain.PhysicalBackupSet{
		{
			ID:       "pgbase_20260830120000",
			SourceID: "pg_source",
		},
	}
	logs := []domain.TransactionLog{
		{
			ID:     "000000010000000000000001",
			Status: domain.LogVerified,
		},
		{
			ID:     "000000010000000000000002",
			Status: domain.LogVerified,
		},
		{
			ID:     "000000010000000000000003",
			Status: domain.LogCorrupted,
		},
	}

	plan, err := p.PlanTimestamp("pg_source", target, backups, logs)
	if err != nil {
		t.Fatal(err)
	}

	if plan.Engine != domain.EnginePostgres || plan.SourceID != "pg_source" {
		t.Fatalf("unexpected PITR plan: %+v", plan)
	}
	if len(plan.BaseBackups) != 1 || plan.BaseBackups[0] != "pgbase_20260830120000" {
		t.Fatalf("unexpected base backups in plan: %+v", plan.BaseBackups)
	}
	// Only verified logs should be included
	if len(plan.Logs) != 2 {
		t.Fatalf("expected 2 verified logs in plan, got %d", len(plan.Logs))
	}
	if plan.Target.Type != domain.RecoveryTargetTimestamp || *plan.Target.Timestamp != target {
		t.Fatalf("unexpected target: %+v", plan.Target)
	}
}

func TestPlanTimestampMissingBackups(t *testing.T) {
	p := Planner{}
	target := time.Now()
	_, err := p.PlanTimestamp("pg_source", target, nil, nil)
	if err == nil {
		t.Fatal("expected error on empty base backups")
	}
	appErr, ok := err.(*domain.AppError)
	if !ok || appErr.Code != domain.ErrRecoveryWindowUnavailable {
		t.Fatalf("expected ErrRecoveryWindowUnavailable, got %v", err)
	}
}
