package domain

import "time"

type OrganisationID string
type ProjectID string
type EnvironmentID string

type OrganisationStatus string

const (
	OrganisationActive    OrganisationStatus = "active"
	OrganisationSuspended OrganisationStatus = "suspended"
	OrganisationDeleted   OrganisationStatus = "deleted"
)

type Organisation struct {
	ID        OrganisationID     `json:"id"`
	Name      string             `json:"name"`
	Slug      string             `json:"slug"`
	Status    OrganisationStatus `json:"status"`
	Plan      string             `json:"plan"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type Project struct {
	ID             ProjectID      `json:"id"`
	OrganisationID OrganisationID `json:"organisation_id"`
	Name           string         `json:"name"`
	Slug           string         `json:"slug"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type Environment struct {
	ID             EnvironmentID  `json:"id"`
	OrganisationID OrganisationID `json:"organisation_id"`
	ProjectID      ProjectID      `json:"project_id"`
	Name           string         `json:"name"`
	Class          string         `json:"class"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type TenantScope struct {
	OrganisationID OrganisationID `json:"organisation_id"`
	ProjectID      ProjectID      `json:"project_id,omitempty"`
	EnvironmentID  EnvironmentID  `json:"environment_id,omitempty"`
	ActorID        string         `json:"actor_id,omitempty"`
}

func (s TenantScope) Valid() bool { return s.OrganisationID != "" }
