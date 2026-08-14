package domain

import "time"

type AgentID string

type AgentStatus string

const (
	AgentPending  AgentStatus = "pending"
	AgentOnline   AgentStatus = "online"
	AgentOffline  AgentStatus = "offline"
	AgentRevoked  AgentStatus = "revoked"
	AgentDegraded AgentStatus = "degraded"
)

type Agent struct {
	ID                AgentID        `json:"id"`
	OrganisationID    OrganisationID `json:"organisation_id"`
	ProjectID         ProjectID      `json:"project_id,omitempty"`
	EnvironmentID     EnvironmentID  `json:"environment_id,omitempty"`
	Name              string         `json:"name"`
	Status            AgentStatus    `json:"status"`
	Version           string         `json:"version"`
	ProtocolVersion   int            `json:"protocol_version"`
	LastSeenAt        *time.Time     `json:"last_seen_at,omitempty"`
	CertificateSerial string         `json:"certificate_serial,omitempty"`
	ConfigGeneration  int64          `json:"config_generation"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type AgentTask struct {
	ID             string         `json:"id"`
	OrganisationID OrganisationID `json:"organisation_id"`
	ProjectID      ProjectID      `json:"project_id,omitempty"`
	AgentID        AgentID        `json:"agent_id"`
	Type           string         `json:"type"`
	Payload        []byte         `json:"payload"`
	IssuedAt       time.Time      `json:"issued_at"`
	ExpiresAt      time.Time      `json:"expires_at"`
	Nonce          string         `json:"nonce"`
}
