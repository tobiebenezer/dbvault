package recoverywindow

import (
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

func TestCalculatorSplitsOnGap(t *testing.T) {
	now := time.Unix(10, 0)
	base := domain.PhysicalBackupSet{ID: "base1", SourceID: "s", LineageID: "l", CompletedAt: now}
	logs := []domain.TransactionLog{
		{ID: "a", Status: domain.LogVerified, EndTime: ptr(now.Add(time.Hour))},
		{ID: "gap", Status: domain.LogGap},
		{ID: "b", Status: domain.LogVerified, StartTime: ptr(now.Add(3 * time.Hour)), EndTime: ptr(now.Add(4 * time.Hour))},
	}
	wins := Calculator{Now: func() time.Time { return now }}.Calculate("s", "l", []domain.PhysicalBackupSet{base}, logs)
	if len(wins) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(wins))
	}
	if !wins[0].Continuous {
		t.Fatalf("first window should be continuous")
	}
}
func ptr(t time.Time) *time.Time { return &t }
