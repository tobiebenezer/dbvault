package domain

import "time"

type JobID string

type JobType string

const (
	JobBackup            JobType = "backup"
	JobVerify            JobType = "verify"
	JobRestoreDrill      JobType = "restore_drill"
	JobRestore           JobType = "restore"
	JobReplication       JobType = "replication"
	JobRetention         JobType = "retention"
	JobGarbageCollection JobType = "garbage_collection"
	JobRepositoryScan    JobType = "repository_scan"
	JobRepositoryRepair  JobType = "repository_repair"
)

type JobStatus string

const (
	JobPending     JobStatus = "pending"
	JobLeased      JobStatus = "leased"
	JobRunning     JobStatus = "running"
	JobRetrying    JobStatus = "retrying"
	JobSucceeded   JobStatus = "succeeded"
	JobFailed      JobStatus = "failed"
	JobCancelled   JobStatus = "cancelled"
	JobInterrupted JobStatus = "interrupted"
	JobDeadLetter  JobStatus = "dead_letter"
)

type Job struct {
	ID              JobID
	Type            JobType
	Status          JobStatus
	ResourceID      string
	PayloadJSON     []byte
	ResultJSON      []byte
	Attempt         int
	MaximumAttempts int
	Priority        int
	AvailableAt     time.Time
	LeaseOwner      string
	LeaseExpiresAt  *time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	LastErrorCode   string
	LastError       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type JobFailure struct {
	Code      ErrorCode
	Message   string
	Retryable bool
	At        time.Time
}

func (j Job) Terminal() bool {
	switch j.Status {
	case JobSucceeded, JobFailed, JobCancelled, JobDeadLetter:
		return true
	default:
		return false
	}
}

func (j Job) CanRetry() bool { return !j.Terminal() && j.Attempt < j.MaximumAttempts }

func RetryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return 30 * time.Second
	}
	switch attempt {
	case 2:
		return 2 * time.Minute
	case 3:
		return 10 * time.Minute
	case 4:
		return 30 * time.Minute
	default:
		return 2 * time.Hour
	}
}
