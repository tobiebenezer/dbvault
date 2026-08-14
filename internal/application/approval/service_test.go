package approval

import (
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

func TestTwoPersonApproval(t *testing.T) {
	svc := New()
	now := time.Now().UTC()
	if err := svc.Create(domain.ApprovalRequest{ID: "r", RequestedBy: "requester", RequiredApprovals: 2, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Decide("r", domain.ApprovalDecision{ActorID: "a", Approved: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != domain.ApprovalPending {
		t.Fatal(r.Status)
	}
	r, err = svc.Decide("r", domain.ApprovalDecision{ActorID: "b", Approved: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != domain.ApprovalApproved {
		t.Fatal(r.Status)
	}
}
