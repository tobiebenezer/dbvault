package domain

import "time"

type BackupTrigger string

const (
	TriggerSchedule BackupTrigger = "schedule"
	TriggerManual   BackupTrigger = "manual"
	TriggerRetry    BackupTrigger = "retry"
)

type BackupRunStatus string

const (
	RunScheduled    BackupRunStatus = "scheduled"
	RunSnapshotting BackupRunStatus = "snapshotting"
	RunChunking     BackupRunStatus = "chunking"
	RunUploading    BackupRunStatus = "uploading"
	RunPublishing   BackupRunStatus = "publishing"
	RunCommitted    BackupRunStatus = "committed"
	RunDuplicate    BackupRunStatus = "duplicate"
	RunFailed       BackupRunStatus = "failed"
)

type BackupRun struct {
	ID           BackupRunID
	SourceID     SourceID
	RepositoryID RepositoryID
	Trigger      BackupTrigger
	Status       BackupRunStatus
	SnapshotID   *SnapshotID
	DuplicateOf  *SnapshotID
	SourceSize   int64
	UniqueBytes  int64
	ReusedBytes  int64
	ErrorCode    string
	ErrorMessage string
	StartedAt    time.Time
	CompletedAt  *time.Time
}
