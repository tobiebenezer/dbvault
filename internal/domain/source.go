package domain

import "time"

type Source struct {
	ID           SourceID
	Name         string
	Driver       string
	Enabled      bool
	Path         string
	RepositoryID RepositoryID
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type SQLiteSnapshotMode string

const (
	SQLiteSnapshotOnlineBackup SQLiteSnapshotMode = "online_backup"
	SQLiteSnapshotVacuumInto   SQLiteSnapshotMode = "vacuum_into"
)

type SourceInspection struct {
	EngineVersion string
	FileSize      int64
	PageSize      int
	PageCount     int64
	JournalMode   string
	SchemaDigest  string
}
