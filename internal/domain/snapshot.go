package domain

import "time"

type SnapshotStatus string

const (
	SnapshotBuilding   SnapshotStatus = "building"
	SnapshotCommitted  SnapshotStatus = "committed"
	SnapshotProtected  SnapshotStatus = "protected"
	SnapshotTombstoned SnapshotStatus = "tombstoned"
	SnapshotDeleted    SnapshotStatus = "deleted"
	SnapshotCorrupted  SnapshotStatus = "corrupted"
)

type Snapshot struct {
	ID                  SnapshotID
	SourceID            SourceID
	RepositoryID        RepositoryID
	Status              SnapshotStatus
	SnapshotMode        SQLiteSnapshotMode
	DatabaseSize        int64
	PageSize            int
	PageCount           int64
	RootDigest          string
	SchemaDigest        string
	ChunkCount          int
	UniqueChunkCount    int
	CompressedBytes     int64
	UniqueUploadedBytes int64
	CreatedAt           time.Time
	CommittedAt         *time.Time
	VerifiedAt          *time.Time
	RestoreTestedAt     *time.Time
	TombstonedAt        *time.Time
	DeleteAfter         *time.Time
	ManifestObjectKey   string
	CompletionObjectKey string
}

type Chunk struct {
	ID             ChunkID
	RepositoryID   RepositoryID
	KeyVersion     string
	Compression    string
	PlaintextSize  int64
	StoredSize     int64
	ObjectKey      string
	CreatedAt      time.Time
	LastVerifiedAt *time.Time
}

type SnapshotChunk struct {
	SnapshotID    SnapshotID
	ChunkID       ChunkID
	RepositoryID  RepositoryID
	Sequence      int
	PageStart     int64
	PageCount     int
	PlaintextSize int64
}
