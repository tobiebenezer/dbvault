package domain

import "time"

type PolicyScope string
type PolicySeverity string
type PolicyEnforcement string

const (
	PolicyScopeOrganisation PolicyScope = "organisation"
	PolicyScopeProject      PolicyScope = "project"
	PolicyScopeEnvironment  PolicyScope = "environment"

	PolicyInfo     PolicySeverity = "info"
	PolicyWarning  PolicySeverity = "warning"
	PolicyCritical PolicySeverity = "critical"

	PolicyAudit PolicyEnforcement = "audit"
	PolicyWarn  PolicyEnforcement = "warn"
	PolicyDeny  PolicyEnforcement = "deny"
)

type PolicyRule struct {
	ID             string            `json:"id"`
	OrganisationID OrganisationID    `json:"organisation_id"`
	ProjectID      ProjectID         `json:"project_id,omitempty"`
	Scope          PolicyScope       `json:"scope"`
	Severity       PolicySeverity    `json:"severity"`
	Enforcement    PolicyEnforcement `json:"enforcement"`
	Expression     string            `json:"expression"`
	Message        string            `json:"message"`
	Remediation    string            `json:"remediation"`
	CreatedAt      time.Time         `json:"created_at"`
}

type PolicyResult struct {
	RuleID         string         `json:"rule_id"`
	OrganisationID OrganisationID `json:"organisation_id"`
	ResourceType   string         `json:"resource_type"`
	ResourceID     string         `json:"resource_id"`
	Passed         bool           `json:"passed"`
	Message        string         `json:"message"`
	EvaluatedAt    time.Time      `json:"evaluated_at"`
}
