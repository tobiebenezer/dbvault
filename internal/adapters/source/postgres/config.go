package postgres

import "time"

type SecretReference struct {
	File string `yaml:"file" json:"file"`
	Env  string `yaml:"env" json:"env"`
}

type Config struct {
	Host              string          `yaml:"host" json:"host"`
	Port              int             `yaml:"port" json:"port"`
	Database          string          `yaml:"database" json:"database"`
	Username          string          `yaml:"username" json:"username"`
	Password          SecretReference `yaml:"password" json:"password"`
	SSLMode           string          `yaml:"ssl_mode" json:"ssl_mode"`
	ConnectTimeout    time.Duration   `yaml:"connect_timeout" json:"connect_timeout"`
	BackupFormat      string          `yaml:"backup_format" json:"backup_format"`
	IncludeGlobals    bool            `yaml:"include_globals" json:"include_globals"`
	IncludeOwnership  bool            `yaml:"include_ownership" json:"include_ownership"`
	IncludePrivileges bool            `yaml:"include_privileges" json:"include_privileges"`
	ParallelJobs      int             `yaml:"parallel_jobs" json:"parallel_jobs"`
	SchemaInclude     []string        `yaml:"schema_include" json:"schema_include"`
	SchemaExclude     []string        `yaml:"schema_exclude" json:"schema_exclude"`
	TableInclude      []string        `yaml:"table_include" json:"table_include"`
	TableExclude      []string        `yaml:"table_exclude" json:"table_exclude"`
	ExtraOptions      []string        `yaml:"extra_options" json:"extra_options"`
}

func DefaultConfig() Config {
	return Config{Port: 5432, SSLMode: "prefer", BackupFormat: "postgres-custom", ConnectTimeout: 10 * time.Second}
}
