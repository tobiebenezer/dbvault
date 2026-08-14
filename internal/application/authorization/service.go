package authorization

import (
	"errors"

	"github.com/dbvault/dbvault/internal/domain"
)

var ErrDenied = errors.New("permission denied")

type Service struct {
	permissions map[domain.Role]map[domain.Permission]struct{}
}

func New() *Service {
	all := []domain.Permission{domain.PermissionSourceRead, domain.PermissionSourceConfigure, domain.PermissionBackupCreate, domain.PermissionBackupCancel, domain.PermissionRestorePlan, domain.PermissionRestoreExecute, domain.PermissionRestoreApprove, domain.PermissionRepositoryConfigure, domain.PermissionRetentionApply, domain.PermissionGCApply, domain.PermissionKeyRotate, domain.PermissionAgentEnrol, domain.PermissionAgentRevoke, domain.PermissionAuditExport, domain.PermissionUsageRead}
	set := func(items ...domain.Permission) map[domain.Permission]struct{} {
		out := map[domain.Permission]struct{}{}
		for _, item := range items {
			out[item] = struct{}{}
		}
		return out
	}
	owner := set(all...)
	return &Service{permissions: map[domain.Role]map[domain.Permission]struct{}{
		domain.RoleOrganisationOwner:         owner,
		domain.RoleOrganisationAdministrator: set(domain.PermissionSourceRead, domain.PermissionSourceConfigure, domain.PermissionBackupCreate, domain.PermissionBackupCancel, domain.PermissionRestorePlan, domain.PermissionRestoreExecute, domain.PermissionRepositoryConfigure, domain.PermissionRetentionApply, domain.PermissionGCApply, domain.PermissionAgentEnrol, domain.PermissionAgentRevoke, domain.PermissionUsageRead),
		domain.RoleSecurityAdministrator:     set(domain.PermissionRestoreApprove, domain.PermissionKeyRotate, domain.PermissionAuditExport, domain.PermissionAgentRevoke, domain.PermissionSourceRead),
		domain.RoleBackupAdministrator:       set(domain.PermissionSourceRead, domain.PermissionSourceConfigure, domain.PermissionBackupCreate, domain.PermissionBackupCancel, domain.PermissionRepositoryConfigure, domain.PermissionRetentionApply, domain.PermissionGCApply),
		domain.RoleRestoreApprover:           set(domain.PermissionSourceRead, domain.PermissionRestorePlan, domain.PermissionRestoreApprove),
		domain.RoleOperator:                  set(domain.PermissionSourceRead, domain.PermissionBackupCreate, domain.PermissionBackupCancel, domain.PermissionRestorePlan),
		domain.RoleAuditor:                   set(domain.PermissionSourceRead, domain.PermissionAuditExport, domain.PermissionUsageRead),
		domain.RoleViewer:                    set(domain.PermissionSourceRead),
		domain.RoleBillingViewer:             set(domain.PermissionUsageRead),
	}}
}

func (s *Service) Authorize(scope domain.TenantScope, membership domain.Membership, permission domain.Permission) error {
	if membership.Status != "active" || membership.OrganisationID != scope.OrganisationID {
		return ErrDenied
	}
	permissions := s.permissions[membership.Role]
	if _, ok := permissions[permission]; !ok {
		return ErrDenied
	}
	return nil
}
