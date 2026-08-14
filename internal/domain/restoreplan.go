package domain

import "time"

type RestoreTargetType string

const (
	RestoreTargetSQLite   RestoreTargetType = "sqlite"
	RestoreTargetPostgres RestoreTargetType = "postgres"
	RestoreTargetMySQL    RestoreTargetType = "mysql"
	RestoreTargetMariaDB  RestoreTargetType = "mariadb"
)

type RestorePlan struct {
	SnapshotID    SnapshotID        `json:"snapshot_id"`
	BackupSetID   BackupSetID       `json:"backup_set_id"`
	Engine        DatabaseEngine    `json:"engine"`
	TargetType    RestoreTargetType `json:"target_type"`
	RequiredTools []string          `json:"required_tools"`
	RequiredKeys  []string          `json:"required_keys"`
	Artifacts     []ArtifactID      `json:"artifacts"`
	Destructive   bool              `json:"destructive"`
	Warnings      []string          `json:"warnings"`
	CreatedAt     time.Time         `json:"created_at"`
}

type RestoreResult struct {
	SnapshotID SnapshotID     `json:"snapshot_id"`
	Engine     DatabaseEngine `json:"engine"`
	Succeeded  bool           `json:"succeeded"`
	Warnings   []string       `json:"warnings"`
}
