package domain

import "time"

type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
	ApprovalExpired  ApprovalStatus = "expired"
)

type ApprovalRequest struct {
	ID                string         `json:"id"`
	OrganisationID    OrganisationID `json:"organisation_id"`
	ProjectID         ProjectID      `json:"project_id,omitempty"`
	ResourceType      string         `json:"resource_type"`
	ResourceID        string         `json:"resource_id"`
	RequestedBy       string         `json:"requested_by"`
	RequiredApprovals int            `json:"required_approvals"`
	Status            ApprovalStatus `json:"status"`
	CreatedAt         time.Time      `json:"created_at"`
	ExpiresAt         time.Time      `json:"expires_at"`
}

type ApprovalDecision struct {
	RequestID string    `json:"request_id"`
	ActorID   string    `json:"actor_id"`
	Approved  bool      `json:"approved"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
