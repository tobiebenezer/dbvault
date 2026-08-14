package domain

import "time"

type UsageRecord struct {
	OrganisationID OrganisationID `json:"organisation_id"`
	ProjectID      ProjectID      `json:"project_id,omitempty"`
	Metric         string         `json:"metric"`
	Quantity       int64          `json:"quantity"`
	Unit           string         `json:"unit"`
	RecordedAt     time.Time      `json:"recorded_at"`
}

type TenantQuota struct {
	OrganisationID           OrganisationID `json:"organisation_id"`
	MaximumSources           int            `json:"maximum_sources"`
	MaximumAgents            int            `json:"maximum_agents"`
	MaximumStorageBytes      int64          `json:"maximum_storage_bytes"`
	MaximumConcurrentBackup  int            `json:"maximum_concurrent_backups"`
	MaximumConcurrentRestore int            `json:"maximum_concurrent_restores"`
}
