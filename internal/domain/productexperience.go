package domain

import "time"

type ProtectionStatus string

const (
	ProtectionProtected   ProtectionStatus = "protected"
	ProtectionAtRisk      ProtectionStatus = "at_risk"
	ProtectionUnprotected ProtectionStatus = "unprotected"
	ProtectionUnknown     ProtectionStatus = "unknown"
)

type ProtectionReason struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Evidence string `json:"evidence,omitempty"`
}

type SuggestedAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Safe        bool   `json:"safe"`
	ReviewOnly  bool   `json:"review_only"`
}

type ProtectionSummary struct {
	SourceID         SourceID           `json:"source_id"`
	Status           ProtectionStatus   `json:"status"`
	Score            int                `json:"score"`
	Summary          string             `json:"summary"`
	Reasons          []ProtectionReason `json:"reasons"`
	SuggestedActions []SuggestedAction  `json:"suggested_actions"`
	CalculatedAt     time.Time          `json:"calculated_at"`
}

type RecoveryTimelineEvent struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Label       string    `json:"label"`
	Status      string    `json:"status"`
	OccurredAt  time.Time `json:"occurred_at"`
	Description string    `json:"description,omitempty"`
}

type RecoveryWindowView struct {
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Continuous bool      `json:"continuous"`
	Verified   bool      `json:"verified"`
}

type RecoveryTimelineGap struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Reason string    `json:"reason"`
}

type RecoveryTimelineResponse struct {
	SourceID    string                  `json:"source_id"`
	Earliest    *time.Time              `json:"earliest,omitempty"`
	Latest      *time.Time              `json:"latest,omitempty"`
	Continuous  bool                    `json:"continuous"`
	Windows     []RecoveryWindowView    `json:"windows"`
	Events      []RecoveryTimelineEvent `json:"events"`
	Gaps        []RecoveryTimelineGap   `json:"gaps"`
	GeneratedAt time.Time               `json:"generated_at"`
}

type SetupState struct {
	ID              string                       `json:"id"`
	CurrentStep     string                       `json:"current_step"`
	CompletedSteps  []string                     `json:"completed_steps"`
	DraftConfigJSON []byte                       `json:"draft_config_json,omitempty"`
	Drafts          map[string]map[string]string `json:"drafts,omitempty"`
	ConfigPath      string                       `json:"config_path,omitempty"`
	Status          string                       `json:"status"`
	CreatedAt       time.Time                    `json:"created_at"`
	UpdatedAt       time.Time                    `json:"updated_at"`
}

type DiscoveredDatabase struct {
	ID            string   `json:"id"`
	AgentID       string   `json:"agent_id"`
	Engine        string   `json:"engine"`
	Host          string   `json:"host,omitempty"`
	Port          int      `json:"port,omitempty"`
	Path          string   `json:"path,omitempty"`
	ServiceName   string   `json:"service_name,omitempty"`
	ContainerName string   `json:"container_name,omitempty"`
	Confidence    float64  `json:"confidence"`
	Evidence      []string `json:"evidence"`
	Status        string   `json:"status"`
}

type DoctorResult struct {
	ID          string        `json:"id"`
	Status      string        `json:"status"`
	Checks      []DoctorCheck `json:"checks"`
	GeneratedAt time.Time     `json:"generated_at"`
}

type DoctorCheck struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	Summary         string `json:"summary"`
	TechnicalDetail string `json:"technical_detail,omitempty"`
	SuggestedFix    string `json:"suggested_fix,omitempty"`
	CopyableCommand string `json:"copyable_command,omitempty"`
}

type ProtectionPolicyDraft struct {
	TemplateID       string `json:"template_id"`
	BackupFrequency  string `json:"backup_frequency"`
	DailyRetention   int    `json:"daily_retention"`
	WeeklyRetention  int    `json:"weekly_retention"`
	MonthlyRetention int    `json:"monthly_retention"`
	ReplicaCount     int    `json:"replica_count"`
	RestoreDrill     string `json:"restore_drill"`
}

type PolicySimulationRequest struct {
	SourceID     string                `json:"source_id"`
	RepositoryID string                `json:"repository_id"`
	Policy       ProtectionPolicyDraft `json:"policy"`
}

type PolicySimulationResult struct {
	BackupsPerDay           float64  `json:"backups_per_day"`
	EstimatedDailyBytes     int64    `json:"estimated_daily_bytes"`
	EstimatedThirtyDayBytes int64    `json:"estimated_thirty_day_bytes"`
	EstimatedNinetyDayBytes int64    `json:"estimated_ninety_day_bytes"`
	EstimatedOperations     int64    `json:"estimated_operations"`
	RestoreScratchBytes     int64    `json:"restore_scratch_bytes"`
	RecoveryWindowSeconds   int64    `json:"recovery_window_seconds"`
	Warnings                []string `json:"warnings"`
}

type SandboxRestore struct {
	ID            string     `json:"id"`
	SourceID      string     `json:"source_id"`
	SnapshotID    string     `json:"snapshot_id,omitempty"`
	TargetTime    *time.Time `json:"target_time,omitempty"`
	Engine        string     `json:"engine"`
	Status        string     `json:"status"`
	ConnectionRef string     `json:"connection_ref,omitempty"`
	JobID         string     `json:"job_id,omitempty"`
	ExpiresAt     time.Time  `json:"expires_at"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
}

type EvidenceReference struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

type Alert struct {
	ID               string              `json:"id"`
	Severity         string              `json:"severity"`
	Status           string              `json:"status"`
	ResourceType     string              `json:"resource_type"`
	ResourceID       string              `json:"resource_id"`
	Title            string              `json:"title"`
	Summary          string              `json:"summary"`
	LikelyCause      string              `json:"likely_cause"`
	SafetyImpact     string              `json:"safety_impact"`
	SuggestedActions []SuggestedAction   `json:"suggested_actions"`
	Evidence         []EvidenceReference `json:"evidence"`
	CreatedAt        time.Time           `json:"created_at"`
}

type DatabaseResource struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Engine         string           `json:"engine"`
	Version        string           `json:"version"`
	Environment    string           `json:"environment"`
	Protection     ProtectionStatus `json:"protection"`
	Score          int              `json:"score"`
	LastBackupAt   time.Time        `json:"last_backup_at"`
	LastDrillAt    time.Time        `json:"last_drill_at"`
	DestinationIDs []string         `json:"destination_ids"`
	RepositoryID   string           `json:"repository_id"`
}

type RepositoryResource struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Mode           string   `json:"mode"`
	Encrypted      bool     `json:"encrypted"`
	SourceIDs      []string `json:"source_ids"`
	DestinationIDs []string `json:"destination_ids"`
}

type DestinationResource struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Provider      string    `json:"provider"`
	Role          string    `json:"role"`
	Region        string    `json:"region"`
	Status        string    `json:"status"`
	LagSeconds    int64     `json:"lag_seconds"`
	LastCheckedAt time.Time `json:"last_checked_at"`
}

type ResourceInventory struct {
	Databases    []DatabaseResource    `json:"databases"`
	Repositories []RepositoryResource  `json:"repositories"`
	Destinations []DestinationResource `json:"destinations"`
	GeneratedAt  time.Time             `json:"generated_at"`
}

type RestoreApprovalRequest struct {
	ID          string     `json:"id"`
	SourceID    string     `json:"source_id"`
	Reason      string     `json:"reason"`
	TargetTime  *time.Time `json:"target_time,omitempty"`
	Target      string     `json:"target"`
	RequestedBy string     `json:"requested_by"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
}
