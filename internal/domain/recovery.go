package domain

import "time"

type RecoveryDriverAPI int

const RecoveryDriverAPIV1 RecoveryDriverAPI = 1

type RecoveryDriverDescriptor struct {
	API                    int            `json:"api"`
	Engine                 DatabaseEngine `json:"engine"`
	SupportsPhysicalBackup bool           `json:"supports_physical_backup"`
	SupportsIncremental    bool           `json:"supports_incremental"`
	SupportsTimestamp      bool           `json:"supports_timestamp"`
	SupportsTransactionID  bool           `json:"supports_transaction_id"`
	SupportsRestorePoint   bool           `json:"supports_restore_point"`
	SupportsGTID           bool           `json:"supports_gtid"`
	SupportsTimelines      bool           `json:"supports_timelines"`
}

type RecoveryCapabilities struct {
	Engine                    DatabaseEngine    `json:"engine"`
	ServerVersion             Version           `json:"server_version"`
	PhysicalBackup            bool              `json:"physical_backup"`
	IncrementalBackup         bool              `json:"incremental_backup"`
	ContinuousLogArchiving    bool              `json:"continuous_log_archiving"`
	TimestampRecovery         bool              `json:"timestamp_recovery"`
	TransactionIDRecovery     bool              `json:"transaction_id_recovery"`
	NamedRestorePointRecovery bool              `json:"named_restore_point_recovery"`
	GTIDRecovery              bool              `json:"gtid_recovery"`
	TimelineRecovery          bool              `json:"timeline_recovery"`
	RequiredTools             []ToolRequirement `json:"required_tools"`
	Warnings                  []string          `json:"warnings"`
}

type ToolRequirement struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Minimum  string `json:"minimum,omitempty"`
}

type RecoveryTargetType string

const (
	RecoveryTargetLatest        RecoveryTargetType = "latest"
	RecoveryTargetTimestamp     RecoveryTargetType = "timestamp"
	RecoveryTargetLSN           RecoveryTargetType = "lsn"
	RecoveryTargetTransactionID RecoveryTargetType = "transaction_id"
	RecoveryTargetRestorePoint  RecoveryTargetType = "restore_point"
	RecoveryTargetImmediate     RecoveryTargetType = "immediate"
	RecoveryTargetFilePosition  RecoveryTargetType = "file_position"
	RecoveryTargetGTID          RecoveryTargetType = "gtid"
)

type RecoveryTarget struct {
	Type          RecoveryTargetType `json:"type"`
	Timestamp     *time.Time         `json:"timestamp,omitempty"`
	LSN           string             `json:"lsn,omitempty"`
	TransactionID string             `json:"transaction_id,omitempty"`
	RestorePoint  string             `json:"restore_point,omitempty"`
	Timeline      string             `json:"timeline,omitempty"`
	File          string             `json:"file,omitempty"`
	Position      uint64             `json:"position,omitempty"`
	GTIDSet       string             `json:"gtid_set,omitempty"`
	Inclusive     bool               `json:"inclusive"`
	Action        string             `json:"action,omitempty"`
}

type PITRPlanID string

type PITRPlan struct {
	ID                  PITRPlanID              `json:"id"`
	SourceID            SourceID                `json:"source_id"`
	Engine              DatabaseEngine          `json:"engine"`
	BaseBackups         []PhysicalBackupID      `json:"base_backups"`
	Logs                []TransactionLogID      `json:"logs"`
	TimelineHistory     []string                `json:"timeline_history"`
	Target              RecoveryTarget          `json:"target"`
	RecoveryWindow      RecoveryWindow          `json:"recovery_window"`
	EstimatedDownload   int64                   `json:"estimated_download"`
	EstimatedScratch    int64                   `json:"estimated_scratch"`
	TargetDirectory     string                  `json:"target_directory"`
	TargetServerVersion string                  `json:"target_server_version"`
	Warnings            []string                `json:"warnings"`
	CreatedAt           time.Time               `json:"created_at"`
	ExpiresAt           time.Time               `json:"expires_at"`
	RepositoryDigest    string                  `json:"repository_digest"`
	CatalogueVersion    int64                   `json:"catalogue_version"`
	Metadata            map[string]string       `json:"metadata,omitempty"`
}

type PITRResult struct {
	PlanID           PITRPlanID     `json:"plan_id"`
	SourceID         SourceID       `json:"source_id"`
	Engine           DatabaseEngine `json:"engine"`
	Succeeded        bool           `json:"succeeded"`
	ReachedTarget    bool           `json:"reached_target"`
	RecoveredTo      RecoveryTarget `json:"recovered_to"`
	StartedAt        time.Time      `json:"started_at"`
	CompletedAt      time.Time      `json:"completed_at"`
	ErrorCode        ErrorCode      `json:"error_code,omitempty"`
	ErrorMessage     string         `json:"error_message,omitempty"`
}

type ChainValidationResult struct {
	SourceID       SourceID           `json:"source_id"`
	Engine         DatabaseEngine     `json:"engine"`
	Continuous     bool               `json:"continuous"`
	Gaps           []RecoveryGap      `json:"gaps"`
	VerifiedLogs   []TransactionLogID `json:"verified_logs"`
	CheckedAt      time.Time          `json:"checked_at"`
	RecoveryWindow RecoveryWindow     `json:"recovery_window"`
}

type RecoveryGap struct {
	AfterPosition  LogPosition `json:"after_position"`
	BeforePosition LogPosition `json:"before_position"`
	Reason         string      `json:"reason"`
}
