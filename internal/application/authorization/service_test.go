package authorization

import (
	"errors"
	"testing"

	"github.com/dbvault/dbvault/internal/domain"
)

func TestCrossTenantAuthorizationIsDenied(t *testing.T) {
	svc := New()
	err := svc.Authorize(domain.TenantScope{OrganisationID: "org-b"}, domain.Membership{OrganisationID: "org-a", Role: domain.RoleOrganisationOwner, Status: "active"}, domain.PermissionRestoreExecute)
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("expected denial, got %v", err)
	}
}
