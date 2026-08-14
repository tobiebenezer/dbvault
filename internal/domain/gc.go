package domain

import "time"

type RetentionPlanID string
type GCPlanID string

type RetentionPlan struct {
	ID               RetentionPlanID
	RepositoryID     RepositoryID
	PolicyDigest     string
	CatalogueVersion int64
	RepositoryDigest string
	CreatedAt        time.Time
	ExpiresAt        time.Time
	Keep             []SnapshotID
	Tombstone        []SnapshotID
	EstimatedBytes   int64
}

type GCChunkCandidate struct {
	ChunkID     ChunkID
	ObjectKey   string
	StoredBytes int64
	Reason      string
}

type GarbageCollectionPlan struct {
	ID               GCPlanID
	RepositoryID     RepositoryID
	CatalogueVersion int64
	RepositoryDigest string
	CreatedAt        time.Time
	ExpiresAt        time.Time
	Candidates       []GCChunkCandidate
	EstimatedBytes   int64
}
