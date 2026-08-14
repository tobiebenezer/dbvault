package ports

import (
	"context"
	"github.com/dbvault/dbvault/internal/domain"
)

type SnapshotArtifact interface {
	Path() string
	Size() int64
	Metadata() SnapshotMetadata
	Close() error
}

type SnapshotMetadata struct {
	EngineVersion string
	PageSize      int
	PageCount     int64
	SchemaDigest  string
}

type SnapshotRequest struct {
	RunID            domain.BackupRunID
	Source           domain.Source
	ScratchDirectory string
	Mode             domain.SQLiteSnapshotMode
}

type SourceDriver interface {
	Name() string
	Validate(ctx context.Context, source domain.Source) error
	Inspect(ctx context.Context, source domain.Source) (domain.SourceInspection, error)
	CreateSnapshot(ctx context.Context, req SnapshotRequest) (SnapshotArtifact, error)
}
