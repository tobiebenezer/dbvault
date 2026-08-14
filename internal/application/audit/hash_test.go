package audit

import (
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

func TestAuditHashChainChangesWithPreviousHash(t *testing.T) {
	e := domain.AuditEvent{ID: "a1", EventType: "backup.completed", ActorType: "system", Outcome: "success", CreatedAt: time.Unix(1, 0)}
	a := domain.ChainAuditEvent(e, "")
	b := domain.ChainAuditEvent(e, "previous")
	if a.EventHash == "" || b.EventHash == "" || a.EventHash == b.EventHash {
		t.Fatalf("hash chain did not include previous hash")
	}
}
