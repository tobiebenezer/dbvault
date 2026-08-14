package domain

import "time"

type BackupSetID string
type ArtifactID string

type ArtifactType string

const (
	ArtifactDatabaseDump    ArtifactType = "database_dump"
	ArtifactGlobals         ArtifactType = "globals"
	ArtifactMetadata        ArtifactType = "metadata"
	ArtifactSchema          ArtifactType = "schema"
	ArtifactData            ArtifactType = "data"
	ArtifactTableOfContents ArtifactType = "table_of_contents"
)

type BackupSet struct {
	ID            BackupSetID         `json:"id"`
	Engine        DatabaseEngine      `json:"engine"`
	EngineVersion string              `json:"engine_version"`
	Toolchain     Toolchain           `json:"toolchain"`
	Mode          DatabaseBackupMode  `json:"mode"`
	Format        BackupFormat        `json:"format"`
	StartedAt     time.Time           `json:"started_at"`
	CompletedAt   time.Time           `json:"completed_at"`
	Artifacts     []BackupArtifact    `json:"artifacts"`
	RestoreOrder  []ArtifactID        `json:"restore_order"`
	Metadata      map[string]string   `json:"metadata"`
	Consistency   ConsistencyMetadata `json:"consistency"`
	SetDigest     string              `json:"set_digest"`
}

type BackupArtifact struct {
	ID          ArtifactID        `json:"id"`
	Type        ArtifactType      `json:"type"`
	Name        string            `json:"name"`
	Format      BackupFormat      `json:"format"`
	ContentType string            `json:"content_type"`
	LogicalSize int64             `json:"logical_size"`
	Required    bool              `json:"required"`
	Sequence    int               `json:"sequence"`
	RootDigest  string            `json:"root_digest"`
	Metadata    map[string]string `json:"metadata"`
}

type ArtifactChunk struct {
	ArtifactID    ArtifactID   `json:"artifact_id"`
	ChunkID       ChunkID      `json:"chunk_id"`
	RepositoryID  RepositoryID `json:"repository_id"`
	Sequence      int          `json:"sequence"`
	LogicalOffset int64        `json:"logical_offset"`
	PlaintextSize int64        `json:"plaintext_size"`
}

type ArtifactSpec struct {
	ID          ArtifactID
	Type        ArtifactType
	Name        string
	Format      BackupFormat
	ContentType string
	Required    bool
	Sequence    int
	Metadata    map[string]string
}

type ArtifactCommitMetadata struct {
	LogicalSize int64
	RootDigest  string
	Metadata    map[string]string
}

type StoredArtifact struct {
	Artifact BackupArtifact
	Chunks   []ArtifactChunk
}
