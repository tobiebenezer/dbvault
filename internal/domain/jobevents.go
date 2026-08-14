package domain

import "time"

// JobEvent is the browser-safe, tenant-scoped event contract used by the
// appliance UI. It is intentionally separate from the durable scheduler's
// internal Job model so alpha UI progress can be wired without weakening the
// backup engine.
type JobEvent struct {
	EventID        string    `json:"event_id"`
	EventType      string    `json:"event_type"`
	JobID          string    `json:"job_id"`
	OrganisationID string    `json:"organisation_id"`
	ProjectID      string    `json:"project_id,omitempty"`
	ResourceType   string    `json:"resource_type"`
	ResourceID     string    `json:"resource_id"`
	JobType        string    `json:"job_type"`
	Status         string    `json:"status"`
	Stage          string    `json:"stage"`
	StageIndex     int       `json:"stage_index"`
	StageCount     int       `json:"stage_count"`
	Percentage     float64   `json:"percentage"`
	BytesProcessed int64     `json:"bytes_processed"`
	BytesTotal     int64     `json:"bytes_total"`
	ThroughputBPS  float64   `json:"throughput_bps"`
	Message        string    `json:"message"`
	CanCancel      bool      `json:"can_cancel"`
	ErrorCode      string    `json:"error_code,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type JobView struct {
	ID             string     `json:"id"`
	OrganisationID string     `json:"organisation_id"`
	ProjectID      string     `json:"project_id,omitempty"`
	ResourceType   string     `json:"resource_type"`
	ResourceID     string     `json:"resource_id"`
	ResourceName   string     `json:"resource_name"`
	JobType        string     `json:"job_type"`
	Status         string     `json:"status"`
	Stage          string     `json:"stage"`
	StageIndex     int        `json:"stage_index"`
	StageCount     int        `json:"stage_count"`
	Percentage     float64    `json:"percentage"`
	BytesProcessed int64      `json:"bytes_processed"`
	BytesTotal     int64      `json:"bytes_total"`
	ThroughputBPS  float64    `json:"throughput_bps"`
	Message        string     `json:"message"`
	CanCancel      bool       `json:"can_cancel"`
	Attempt        int        `json:"attempt"`
	OriginalJobID  string     `json:"original_job_id,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastErrorCode  string     `json:"last_error_code,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
}

type JobLogLine struct {
	ID        string    `json:"id"`
	JobID     string    `json:"job_id"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Stage     string    `json:"stage,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type JobListResponse struct {
	Jobs        []JobView `json:"jobs"`
	GeneratedAt time.Time `json:"generated_at"`
}

type JobRetryResponse struct {
	OriginalJobID string  `json:"original_job_id"`
	NewJobID      string  `json:"new_job_id"`
	Job           JobView `json:"job"`
}
