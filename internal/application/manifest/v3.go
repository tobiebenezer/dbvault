package manifest

type SnapshotManifestV3 struct {
	Format        string           `json:"format"`
	FormatVersion int              `json:"format_version"`
	RepositoryID  string           `json:"repository_id"`
	SnapshotID    string           `json:"snapshot_id"`
	SourceID      string           `json:"source_id"`
	Database      DatabaseInfoV3   `json:"database"`
	BackupSet     BackupSetInfoV3  `json:"backup_set"`
	Artifacts     []ArtifactInfoV3 `json:"artifacts"`
}

type DatabaseInfoV3 struct {
	Engine        string            `json:"engine"`
	ServerVersion string            `json:"server_version"`
	DriverAPI     int               `json:"driver_api"`
	BackupMode    string            `json:"backup_mode"`
	BackupFormat  string            `json:"backup_format"`
	Toolchain     map[string]string `json:"toolchain"`
}

type BackupSetInfoV3 struct {
	ID           string   `json:"id"`
	SetDigest    string   `json:"set_digest"`
	RestoreOrder []string `json:"restore_order"`
}

type ArtifactInfoV3 struct {
	ArtifactID  string      `json:"artifact_id"`
	Type        string      `json:"type"`
	Name        string      `json:"name"`
	Format      string      `json:"format"`
	Required    bool        `json:"required"`
	Sequence    int         `json:"sequence"`
	LogicalSize int64       `json:"logical_size"`
	RootDigest  string      `json:"root_digest"`
	Chunks      []ChunkInfo `json:"chunks"`
}
