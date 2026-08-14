package retention

import (
	"github.com/dbvault/dbvault/internal/domain"
	"testing"
	"time"
)

func TestPlannerKeepsRecent(t *testing.T) {
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	var snaps []domain.Snapshot
	for i := 0; i < 10; i++ {
		snaps = append(snaps, domain.Snapshot{ID: domain.SnapshotID(string(rune('a' + i))), Status: domain.SnapshotCommitted, CreatedAt: now.Add(-time.Duration(i) * 6 * time.Hour)})
	}
	p := New(domain.DefaultRetentionPolicy(), time.UTC)
	d := p.Plan(snaps)
	if len(d.Keep) < 4 {
		t.Fatalf("expected at least 4 kept, got %d", len(d.Keep))
	}
	if len(d.Tombstone) == 0 {
		t.Fatalf("expected old snapshots to be tombstoned")
	}
}
