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

// WarehouseEvidenceStore persists per-dataset warehouse sync evidence (real
// schema, measured row counts and byte sizes, written Parquet paths, and the
// incremental watermark state) in the catalogue. It is a separate port so the
// backup catalogue contract stays unchanged.
type WarehouseEvidenceStore interface {
	UpsertWarehouseDataset(ctx context.Context, ds domain.WarehouseDataset) error
	GetWarehouseDataset(ctx context.Context, databaseID, datasetName string) (domain.WarehouseDataset, bool, error)
	ListWarehouseDatasets(ctx context.Context, databaseID string) ([]domain.WarehouseDataset, error)
}
