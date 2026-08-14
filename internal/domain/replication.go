package domain

import "time"

type DestinationRole string

const (
	DestinationPrimary DestinationRole = "primary"
	DestinationMirror  DestinationRole = "mirror"
	DestinationReplica DestinationRole = "replica"
)

type SnapshotDestinationStatus string

const (
	SnapshotDestinationPending   SnapshotDestinationStatus = "pending"
	SnapshotDestinationUploading SnapshotDestinationStatus = "uploading"
	SnapshotDestinationVerifying SnapshotDestinationStatus = "verifying"
	SnapshotDestinationComplete  SnapshotDestinationStatus = "complete"
	SnapshotDestinationFailed    SnapshotDestinationStatus = "failed"
	SnapshotDestinationDelayed   SnapshotDestinationStatus = "delayed"
	SnapshotDestinationCorrupted SnapshotDestinationStatus = "corrupted"
)

type SnapshotDestination struct {
	SnapshotID     SnapshotID
	DestinationID  DestinationID
	Role           DestinationRole
	Status         SnapshotDestinationStatus
	ChunkCount     int
	VerifiedChunks int
	StoredBytes    int64
	LastAttemptAt  *time.Time
	CompletedAt    *time.Time
	ErrorCode      string
	ErrorMessage   string
}

type ReplicationPolicy struct {
	RequiredWithin time.Duration
	MaximumRetries int
	Priority       int
}
