package domain

import "time"

type UserID string
type Role string
type Permission string

const (
	RoleOrganisationOwner         Role = "organisation_owner"
	RoleOrganisationAdministrator Role = "organisation_administrator"
	RoleSecurityAdministrator     Role = "security_administrator"
	RoleBackupAdministrator       Role = "backup_administrator"
	RoleRestoreApprover           Role = "restore_approver"
	RoleOperator                  Role = "operator"
	RoleAuditor                   Role = "auditor"
	RoleViewer                    Role = "viewer"
	RoleBillingViewer             Role = "billing_viewer"
)

const (
	PermissionSourceRead          Permission = "source.read"
	PermissionSourceConfigure     Permission = "source.configure"
	PermissionBackupCreate        Permission = "backup.create"
	PermissionBackupCancel        Permission = "backup.cancel"
	PermissionRestorePlan         Permission = "restore.plan"
	PermissionRestoreExecute      Permission = "restore.execute"
	PermissionRestoreApprove      Permission = "restore.approve"
	PermissionRepositoryConfigure Permission = "repository.configure"
	PermissionRetentionApply      Permission = "retention.apply"
	PermissionGCApply             Permission = "gc.apply"
	PermissionKeyRotate           Permission = "key.rotate"
	PermissionAgentEnrol          Permission = "agent.enrol"
	PermissionAgentRevoke         Permission = "agent.revoke"
	PermissionAuditExport         Permission = "audit.export"
	PermissionUsageRead           Permission = "usage.read"
)

type Membership struct {
	OrganisationID OrganisationID `json:"organisation_id"`
	UserID         UserID         `json:"user_id"`
	Role           Role           `json:"role"`
	Status         string         `json:"status"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type IdentityProvider struct {
	ID             string         `json:"id"`
	OrganisationID OrganisationID `json:"organisation_id"`
	Type           string         `json:"type"`
	Issuer         string         `json:"issuer,omitempty"`
	EntityID       string         `json:"entity_id,omitempty"`
	Enabled        bool           `json:"enabled"`
}
