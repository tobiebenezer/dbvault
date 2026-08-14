package domain

import "time"

type DatabaseEngine string

const (
	EngineSQLite   DatabaseEngine = "sqlite"
	EnginePostgres DatabaseEngine = "postgres"
	EngineMySQL    DatabaseEngine = "mysql"
	EngineMariaDB  DatabaseEngine = "mariadb"
)

type DatabaseBackupMode string

const (
	BackupModeLogical  DatabaseBackupMode = "logical"
	BackupModePhysical DatabaseBackupMode = "physical"
)

type BackupFormat string

const (
	FormatSQLiteFile        BackupFormat = "sqlite-file"
	FormatPostgresCustom    BackupFormat = "postgres-custom"
	FormatPostgresDirectory BackupFormat = "postgres-directory"
	FormatPostgresPlainSQL  BackupFormat = "postgres-plain-sql"
	FormatPostgresGlobals   BackupFormat = "postgres-globals-sql"
	FormatMySQLSQL          BackupFormat = "mysql-sql"
	FormatMariaDBSQL        BackupFormat = "mariadb-sql"
)

type DriverDescriptor struct {
	API               int                  `json:"api"`
	Name              string               `json:"name"`
	Engine            DatabaseEngine       `json:"engine"`
	SupportedModes    []DatabaseBackupMode `json:"supported_modes"`
	SupportedFormats  []BackupFormat       `json:"supported_formats"`
	SupportsStreaming bool                 `json:"supports_streaming"`
	SupportsParallel  bool                 `json:"supports_parallel"`
	SupportsGlobals   bool                 `json:"supports_globals"`
	SupportsSelection bool                 `json:"supports_selection"`
	SupportsHooks     bool                 `json:"supports_hooks"`
}

const DatabaseDriverAPIV1 = 1

type Version struct {
	Raw   string `json:"raw"`
	Major int    `json:"major"`
	Minor int    `json:"minor"`
	Patch int    `json:"patch"`
}

type Toolchain struct {
	Engine          DatabaseEngine    `json:"engine"`
	ServerVersion   Version           `json:"server_version"`
	ClientVersion   Version           `json:"client_version"`
	ExecutablePaths map[string]string `json:"executable_paths"`
	Capabilities    ToolCapabilities  `json:"capabilities"`
	DetectedAt      time.Time         `json:"detected_at"`
}

type ToolCapabilities struct {
	ParallelDump       bool `json:"parallel_dump"`
	ParallelRestore    bool `json:"parallel_restore"`
	CustomArchive      bool `json:"custom_archive"`
	DirectoryArchive   bool `json:"directory_archive"`
	ConsistentSnapshot bool `json:"consistent_snapshot"`
	IncludeGlobals     bool `json:"include_globals"`
	NoOwner            bool `json:"no_owner"`
	NoPrivileges       bool `json:"no_privileges"`
}

type DatabaseInspection struct {
	Engine        DatabaseEngine      `json:"engine"`
	EngineVersion Version             `json:"engine_version"`
	DatabaseSize  int64               `json:"database_size"`
	Summary       map[string]string   `json:"summary"`
	Warnings      []string            `json:"warnings"`
	Extensions    []DatabaseExtension `json:"extensions"`
}

type DatabaseExtension struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ConsistencyMetadata struct {
	Method        string    `json:"method"`
	TransactionID string    `json:"transaction_id,omitempty"`
	SnapshotID    string    `json:"snapshot_id,omitempty"`
	StartedAt     time.Time `json:"started_at"`
}
