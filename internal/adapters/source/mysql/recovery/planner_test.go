package recovery

import (
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

func TestPlanTimestamp(t *testing.T) {
	p := Planner{Engine: domain.EngineMySQL}
	target := time.Date(2026, 8, 30, 16, 0, 0, 0, time.UTC)
	logs := []domain.TransactionLog{
		{ID: "mysql-bin.000001", Status: domain.LogVerified},
		{ID: "mysql-bin.000002", Status: domain.LogVerified},
		{ID: "mysql-bin.000003", Status: domain.LogCorrupted},
	}

	plan, err := p.PlanTimestamp("mysql_src", target, "snap_100", logs)
	if err != nil {
		t.Fatal(err)
	}

	if plan.Engine != domain.EngineMySQL || plan.SourceID != "mysql_src" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if len(plan.Logs) != 2 {
		t.Fatalf("expected 2 verified logs in plan, got %d", len(plan.Logs))
	}
	if plan.Target.Type != domain.RecoveryTargetTimestamp || *plan.Target.Timestamp != target {
		t.Fatalf("unexpected target: %+v", plan.Target)
	}
	if plan.Metadata["base_snapshot"] != "snap_100" {
		t.Fatalf("unexpected metadata: %+v", plan.Metadata)
	}
}

func TestPlanTimestampMariaDB(t *testing.T) {
	p := Planner{Engine: domain.EngineMariaDB}
	target := time.Now()
	plan, err := p.PlanTimestamp("maria_src", target, "snap_200", nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Engine != domain.EngineMariaDB {
		t.Fatalf("expected MariaDB engine, got %v", plan.Engine)
	}
}
