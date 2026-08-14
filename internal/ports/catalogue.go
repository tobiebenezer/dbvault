package ports

import (
	"context"
	"github.com/dbvault/dbvault/internal/domain"
)

type Catalogue interface {
	CreateBackupRun(ctx context.Context, run domain.BackupRun) error
	UpdateBackupRun(ctx context.Context, run domain.BackupRun) error
	CreateSnapshot(ctx context.Context, snap domain.Snapshot, chunks []domain.SnapshotChunk, chunkRecords []domain.Chunk) error
	GetSnapshot(ctx context.Context, id domain.SnapshotID) (domain.Snapshot, []domain.SnapshotChunk, error)
	ListSnapshots(ctx context.Context, sourceID domain.SourceID) ([]domain.Snapshot, error)
	FindSnapshotByRoot(ctx context.Context, sourceID domain.SourceID, rootDigest string) (domain.Snapshot, bool, error)
}
