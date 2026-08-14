package domain

import "time"

type RepositoryMode string

const (
	RepositorySingle         RepositoryMode = "single"
	RepositoryMirror         RepositoryMode = "mirror"
	RepositoryPrimaryReplica RepositoryMode = "primary_replica"
)

type Repository struct {
	ID                 RepositoryID
	Name               string
	Mode               RepositoryMode
	PrimaryDestination DestinationID
	Chunking           ChunkingPolicy
	Compression        CompressionPolicy
	Encryption         EncryptionPolicy
	Retention          RetentionPolicy
	Budget             BudgetPolicy
	CreatedAt          time.Time
}

type ChunkingPolicy struct {
	Strategy        string
	TargetSizeBytes int64
}

type CompressionPolicy struct {
	Algorithm             string
	Level                 int
	MinimumSavingsPercent int
}

type EncryptionPolicy struct {
	Algorithm string
	KeyID     string
}
