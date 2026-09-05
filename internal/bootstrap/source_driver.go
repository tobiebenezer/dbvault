package bootstrap

import (
	"fmt"
	"time"

	execc "github.com/dbvault/dbvault/internal/adapters/process/exec"
	srcmysql "github.com/dbvault/dbvault/internal/adapters/source/mysql"
	srcpostgres "github.com/dbvault/dbvault/internal/adapters/source/postgres"
	srcsqlite "github.com/dbvault/dbvault/internal/adapters/source/sqlite"
	"github.com/dbvault/dbvault/internal/config"
	"github.com/dbvault/dbvault/internal/ports"
)

// buildSourceDriver picks the snapshot source for the bound runtime source.
// SQLite sources use the sqlite CLI driver; MySQL/MariaDB sources run real
// mysqldump/mariadb-dump through the v1 logical driver adapted to
// ports.SourceDriver; PostgreSQL sources run real pg_dump adapted to
// ports.SourceDriver. Credentials arrive exclusively through secret
// references (file or env), never literals.
func buildSourceDriver(s config.SourceConfig) (ports.SourceDriver, error) {
	switch s.Engine {
	case "sqlite":
		if s.SQLite == nil {
			return nil, fmt.Errorf("source %s is missing its sqlite configuration", s.ID)
		}
		return srcsqlite.New("sqlite3"), nil
	case "mysql", "mariadb":
		if s.MySQL == nil {
			return nil, fmt.Errorf("source %s is missing its mysql configuration", s.ID)
		}
		return srcmysql.NewSnapshotSource(srcmysql.New(execc.New(), mysqlDriverConfig(s))), nil
	case "postgres":
		if s.Postgres == nil {
			return nil, fmt.Errorf("source %s is missing its postgres configuration", s.ID)
		}
		return srcpostgres.NewSnapshotSource(srcpostgres.New(execc.New(), postgresDriverConfig(s))), nil
	default:
		return nil, fmt.Errorf("source %s uses unsupported engine %q", s.ID, s.Engine)
	}
}

func postgresDriverConfig(s config.SourceConfig) srcpostgres.Config {
	p := s.Postgres
	cfg := srcpostgres.DefaultConfig()
	cfg.Host = p.Host
	if p.Port != 0 {
		cfg.Port = p.Port
	}
	cfg.Database = p.Database
	cfg.Username = p.Username
	cfg.Password = srcpostgres.SecretReference{File: p.Password.File, Env: p.Password.Env}
	if p.SSL.Mode != "" {
		cfg.SSLMode = p.SSL.Mode
	}
	if p.ConnectTimeout != "" {
		if d, err := time.ParseDuration(p.ConnectTimeout); err == nil {
			cfg.ConnectTimeout = d
		}
	}
	lb := p.LogicalBackup
	if lb.Format != "" {
		cfg.BackupFormat = lb.Format
	}
	cfg.IncludeGlobals = lb.IncludeGlobals
	cfg.IncludeOwnership = lb.IncludeOwnership
	cfg.IncludePrivileges = lb.IncludePrivileges
	if lb.ParallelJobs > 0 {
		cfg.ParallelJobs = lb.ParallelJobs
	}
	return cfg
}

func mysqlDriverConfig(s config.SourceConfig) srcmysql.Config {
	m := s.MySQL
	cfg := srcmysql.DefaultConfig(s.Engine)
	cfg.Host = m.Host
	if m.Port != 0 {
		cfg.Port = m.Port
	}
	cfg.Database = m.Database
	cfg.Username = m.Username
	cfg.Password = srcmysql.SecretReference{File: m.Password.File, Env: m.Password.Env}
	if m.TLS.Mode != "" {
		cfg.TLSMode = m.TLS.Mode
	}
	if m.ConnectTimeout != "" {
		if d, err := time.ParseDuration(m.ConnectTimeout); err == nil {
			cfg.ConnectTimeout = d
		}
	}
	// Logical knobs default on (single-transaction, quick, routines,
	// triggers, events); explicit true values are honoured, and only an
	// explicitly configured section can tighten behaviour later.
	lb := m.LogicalBackup
	if lb.SingleTransaction {
		cfg.SingleTransaction = true
	}
	if lb.Quick {
		cfg.Quick = true
	}
	if lb.Routines {
		cfg.Routines = true
	}
	if lb.Triggers {
		cfg.Triggers = true
	}
	if lb.Events {
		cfg.Events = true
	}
	return cfg
}
