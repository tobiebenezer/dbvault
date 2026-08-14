package domain

import "time"

type VerificationStatus string

const (
	VerificationPending   VerificationStatus = "pending"
	VerificationSucceeded VerificationStatus = "succeeded"
	VerificationFailed    VerificationStatus = "failed"
)

type VerificationLevel int

const (
	VerifyPublication VerificationLevel = iota + 1
	VerifyMetadata
	VerifyExistence
	VerifyAuthenticationSample
	VerifyAllChunks
	VerifySQLiteQuickCheck
	VerifySQLiteIntegrityCheck
	VerifyApplicationProbes
)

type VerificationResult struct {
	ID            VerificationRunID
	SnapshotID    SnapshotID
	Level         VerificationLevel
	Status        VerificationStatus
	CheckedChunks int
	FailedChunks  int
	ErrorCode     string
	ErrorMessage  string
	StartedAt     time.Time
	CompletedAt   *time.Time
}

type RestoreDrillID string

type RestoreDrillResult struct {
	ID               RestoreDrillID
	SnapshotID       SnapshotID
	DestinationID    DestinationID
	Status           VerificationStatus
	DownloadBytes    int64
	Duration         time.Duration
	QuickCheckPassed bool
	IntegrityPassed  bool
	StartedAt        time.Time
	CompletedAt      *time.Time
	ErrorCode        string
	ErrorMessage     string
}

type Probe struct {
	ID        string
	Query     string
	Timeout   time.Duration
	Sensitive bool
	Required  bool
}
