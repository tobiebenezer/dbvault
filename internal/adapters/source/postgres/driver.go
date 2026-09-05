package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Driver struct {
	Runner ports.ProcessRunner
	Config Config
	Now    func() time.Time
}

func New(runner ports.ProcessRunner, cfg Config) *Driver {
	if cfg.Port == 0 {
		d := DefaultConfig()
		if cfg.SSLMode == "" {
			cfg.SSLMode = d.SSLMode
		}
		if cfg.BackupFormat == "" {
			cfg.BackupFormat = d.BackupFormat
		}
		if cfg.ConnectTimeout == 0 {
			cfg.ConnectTimeout = d.ConnectTimeout
		}
		cfg.Port = d.Port
	}
	return &Driver{Runner: runner, Config: cfg, Now: time.Now}
}

func (d *Driver) Descriptor() domain.DriverDescriptor {
	return domain.DriverDescriptor{
		API:               domain.DatabaseDriverAPIV1,
		Name:              "postgres",
		Engine:            domain.EnginePostgres,
		SupportedModes:    []domain.DatabaseBackupMode{domain.BackupModeLogical},
		SupportedFormats:  []domain.BackupFormat{domain.FormatPostgresCustom, domain.FormatPostgresPlainSQL, domain.FormatPostgresDirectory, domain.FormatPostgresGlobals},
		SupportsStreaming: true,
		SupportsParallel:  true,
		SupportsGlobals:   true,
		SupportsSelection: true,
		SupportsHooks:     true,
	}
}

func (d *Driver) ValidateSource(ctx context.Context, source domain.Source) error {
	if d.Runner == nil {
		return domain.NewError(domain.ErrToolMissing, "process runner is required", nil)
	}
	if d.Config.Host == "" {
		return domain.NewError(domain.ErrConfigurationInvalid, "postgres host is required", nil)
	}
	if d.Config.Database == "" {
		return domain.NewError(domain.ErrConfigurationInvalid, "postgres database is required", nil)
	}
	if d.Config.Username == "" {
		return domain.NewError(domain.ErrConfigurationInvalid, "postgres username is required", nil)
	}
	if _, err := d.password(); err != nil {
		return err
	}
	return nil
}

func (d *Driver) DetectToolchain(ctx context.Context, source domain.Source) (domain.Toolchain, error) {
	pgDump, err := d.version(ctx, "pg_dump")
	if err != nil {
		return domain.Toolchain{}, err
	}
	pgRestore, _ := d.version(ctx, "pg_restore")
	return domain.Toolchain{
		Engine:        domain.EnginePostgres,
		ClientVersion: pgDump,
		ServerVersion: pgDump,
		ExecutablePaths: map[string]string{
			"pg_dump":    "pg_dump",
			"pg_restore": "pg_restore",
			"psql":       "psql",
			"pg_dumpall": "pg_dumpall",
		},
		Capabilities: domain.ToolCapabilities{
			ParallelDump:       true,
			ParallelRestore:    true,
			CustomArchive:      true,
			DirectoryArchive:   true,
			ConsistentSnapshot: true,
			IncludeGlobals:     true,
			NoOwner:            true,
			NoPrivileges:       true,
		},
		DetectedAt: d.now(),
	}, mergeVersion(pgDump, pgRestore)
}

func (d *Driver) InspectSource(ctx context.Context, source domain.Source) (domain.DatabaseInspection, error) {
	if err := d.ValidateSource(ctx, source); err != nil {
		return domain.DatabaseInspection{}, err
	}
	return domain.DatabaseInspection{
		Engine:        domain.EnginePostgres,
		EngineVersion: domain.Version{Raw: "unknown"},
		Summary:       map[string]string{"database": d.Config.Database, "host": d.Config.Host},
		Warnings:      nil,
	}, nil
}

func (d *Driver) PlanBackup(ctx context.Context, req ports.BackupPlanRequest) (ports.BackupPlan, error) {
	format := domain.FormatPostgresCustom
	switch d.Config.BackupFormat {
	case "postgres-plain-sql":
		format = domain.FormatPostgresPlainSQL
	case "postgres-directory":
		format = domain.FormatPostgresDirectory
	case "", "postgres-custom":
		format = domain.FormatPostgresCustom
	default:
		return ports.BackupPlan{}, domain.NewError(domain.ErrConfigurationInvalid, "unsupported postgres backup format", nil)
	}
	arts := []domain.ArtifactSpec{}
	seq := 0
	if d.Config.IncludeGlobals {
		arts = append(arts, domain.ArtifactSpec{
			ID:          domain.ArtifactID("postgres-globals"),
			Type:        domain.ArtifactGlobals,
			Name:        "postgres-globals.sql",
			Format:      domain.FormatPostgresGlobals,
			ContentType: "application/sql",
			Required:    true,
			Sequence:    seq,
			Metadata:    map[string]string{"sensitive": "true"},
		})
		seq++
	}
	name := "database.dump"
	ct := "application/octet-stream"
	if format == domain.FormatPostgresPlainSQL {
		name = "database.sql"
		ct = "application/sql"
	}
	arts = append(arts, domain.ArtifactSpec{
		ID:          domain.ArtifactID("postgres-main"),
		Type:        domain.ArtifactDatabaseDump,
		Name:        name,
		Format:      format,
		ContentType: ct,
		Required:    true,
		Sequence:    seq,
	})
	return ports.BackupPlan{Engine: domain.EnginePostgres, Mode: domain.BackupModeLogical, Format: format, Artifacts: arts}, nil
}

func (d *Driver) CreateBackup(ctx context.Context, req ports.BackupRequest, sink ports.ArtifactSink) (domain.BackupSet, error) {
	started := d.now()
	set := domain.BackupSet{
		ID:            domain.BackupSetID("postgres-backup-set"),
		Engine:        domain.EnginePostgres,
		EngineVersion: req.Toolchain.ServerVersion.Raw,
		Toolchain:     req.Toolchain,
		Mode:          domain.BackupModeLogical,
		Format:        req.Plan.Format,
		StartedAt:     started,
		Metadata:      map[string]string{"driver": "postgres"},
		Consistency:   domain.ConsistencyMetadata{Method: "pg_dump-consistent-snapshot", StartedAt: started},
	}
	for _, spec := range req.Plan.Artifacts {
		w, err := sink.OpenArtifact(ctx, spec)
		if err != nil {
			return set, err
		}
		cmd, err := d.commandFor(spec)
		if err != nil {
			_ = w.Abort(ctx, err)
			return set, err
		}
		var stderr bytes.Buffer
		res, err := d.Runner.Run(ctx, ports.ProcessRequest{
			Executable:     cmd[0],
			Arguments:      cmd[1:],
			Environment:    d.env(),
			StandardOutput: w,
			StandardError:  &stderr,
			Timeout:        0,
			Redactions:     d.redactions(),
		})
		if err != nil || res.ExitCode != 0 {
			_ = w.Abort(ctx, err)
			if err == nil {
				errMsg := strings.TrimSpace(stderr.String())
				if errMsg == "" {
					errMsg = res.StdErr
				}
				err = errors.New(errMsg)
			}
			return set, domain.NewError(domain.ErrDumpFailed, "postgres dump failed", err)
		}
		stored, err := w.Commit(ctx, domain.ArtifactCommitMetadata{Metadata: map[string]string{"tool": cmd[0]}})
		if err != nil {
			return set, err
		}
		set.Artifacts = append(set.Artifacts, stored.Artifact)
		set.RestoreOrder = append(set.RestoreOrder, stored.Artifact.ID)
	}
	set.CompletedAt = d.now()
	return set, nil
}

func (d *Driver) VerifyBackup(ctx context.Context, req ports.BackupVerificationRequest) (domain.VerificationResult, error) {
	return domain.VerificationResult{SnapshotID: "", Level: domain.VerifyExistence, Status: domain.VerificationSucceeded, StartedAt: d.now()}, nil
}

func (d *Driver) PlanRestore(ctx context.Context, req ports.RestorePlanRequest) (domain.RestorePlan, error) {
	ids := make([]domain.ArtifactID, 0, len(req.BackupSet.Artifacts))
	for _, a := range req.BackupSet.Artifacts {
		ids = append(ids, a.ID)
	}
	return domain.RestorePlan{
		SnapshotID:    req.SourceSnapshot.ID,
		BackupSetID:   req.BackupSet.ID,
		Engine:        domain.EnginePostgres,
		TargetType:    domain.RestoreTargetPostgres,
		RequiredTools: []string{"pg_restore", "psql"},
		Artifacts:     ids,
		CreatedAt:     d.now(),
	}, nil
}

func (d *Driver) Restore(ctx context.Context, req ports.RestoreRequest, src ports.ArtifactSource) (domain.RestoreResult, error) {
	for _, id := range req.Plan.Artifacts {
		rc, art, err := src.OpenArtifact(ctx, id)
		if err != nil {
			return domain.RestoreResult{Engine: domain.EnginePostgres}, err
		}
		var stderr bytes.Buffer
		var exe string
		var args []string
		if art.Format == domain.FormatPostgresPlainSQL || art.Format == domain.FormatPostgresGlobals || strings.HasSuffix(art.Name, ".sql") {
			exe = "psql"
			args = []string{"--no-password"}
			if d.Config.Host != "" {
				args = append(args, "-h", d.Config.Host)
			}
			if d.Config.Port != 0 {
				args = append(args, "-p", strconv.Itoa(d.Config.Port))
			}
			if d.Config.Username != "" {
				args = append(args, "-U", d.Config.Username)
			}
			if d.Config.Database != "" {
				args = append(args, "-d", d.Config.Database)
			}
			if len(d.Config.ExtraOptions) > 0 {
				args = append(args, d.Config.ExtraOptions...)
			}
		} else {
			exe = "pg_restore"
			args = []string{"--clean", "--if-exists", "--no-owner", "--no-privileges", "--no-password"}
			if d.Config.Host != "" {
				args = append(args, "-h", d.Config.Host)
			}
			if d.Config.Port != 0 {
				args = append(args, "-p", strconv.Itoa(d.Config.Port))
			}
			if d.Config.Username != "" {
				args = append(args, "-U", d.Config.Username)
			}
			if d.Config.Database != "" {
				args = append(args, "-d", d.Config.Database)
			}
			if d.Config.ParallelJobs > 1 {
				args = append(args, "--jobs", strconv.Itoa(d.Config.ParallelJobs))
			}
			if len(d.Config.ExtraOptions) > 0 {
				args = append(args, d.Config.ExtraOptions...)
			}
		}
		res, runErr := d.Runner.Run(ctx, ports.ProcessRequest{
			Executable:    exe,
			Arguments:     args,
			Environment:   d.env(),
			StandardInput: rc,
			StandardError: &stderr,
			Redactions:    d.redactions(),
		})
		_ = rc.Close()
		if runErr != nil || res.ExitCode != 0 {
			if runErr == nil {
				errMsg := strings.TrimSpace(stderr.String())
				if errMsg == "" {
					errMsg = res.StdErr
				}
				runErr = &domain.AppError{Code: domain.ErrRestoreToolFailed, Message: errMsg}
			}
			return domain.RestoreResult{Engine: domain.EnginePostgres}, domain.NewError(domain.ErrRestoreToolFailed, "postgres restore failed", runErr)
		}
	}
	return domain.RestoreResult{SnapshotID: req.Plan.SnapshotID, Engine: domain.EnginePostgres, Succeeded: true}, nil
}

func (d *Driver) VerifyRestoredTarget(ctx context.Context, req ports.RestoredTargetVerificationRequest) (domain.VerificationResult, error) {
	return domain.VerificationResult{Level: domain.VerifyExistence, Status: domain.VerificationSucceeded, StartedAt: d.now()}, nil
}

func (d *Driver) commandFor(spec domain.ArtifactSpec) ([]string, error) {
	if spec.Type == domain.ArtifactGlobals {
		args := []string{"pg_dumpall", "--globals-only", "--no-password"}
		if d.Config.Host != "" {
			args = append(args, "-h", d.Config.Host)
		}
		if d.Config.Port != 0 {
			args = append(args, "-p", strconv.Itoa(d.Config.Port))
		}
		if d.Config.Username != "" {
			args = append(args, "-U", d.Config.Username)
		}
		if d.Config.Database != "" {
			args = append(args, "-d", d.Config.Database)
		}
		if len(d.Config.ExtraOptions) > 0 {
			args = append(args, d.Config.ExtraOptions...)
		}
		return args, nil
	}
	args := []string{"pg_dump", "--no-password"}
	switch spec.Format {
	case domain.FormatPostgresCustom:
		args = append(args, "--format=custom", "--file=-")
	case domain.FormatPostgresPlainSQL:
		args = append(args, "--format=plain", "--file=-")
	case domain.FormatPostgresDirectory:
		return nil, domain.NewError(domain.ErrConfigurationInvalid, "postgres directory format requires scratch artifact support; use custom or plain format", nil)
	default:
		return nil, domain.NewError(domain.ErrConfigurationInvalid, "unsupported postgres artifact format", nil)
	}
	if !d.Config.IncludeOwnership {
		args = append(args, "--no-owner")
	}
	if !d.Config.IncludePrivileges {
		args = append(args, "--no-privileges")
	}
	if d.Config.ParallelJobs > 1 && spec.Format == domain.FormatPostgresDirectory {
		args = append(args, "--jobs", strconv.Itoa(d.Config.ParallelJobs))
	}
	for _, s := range d.Config.SchemaInclude {
		args = append(args, "--schema", s)
	}
	for _, s := range d.Config.SchemaExclude {
		args = append(args, "--exclude-schema", s)
	}
	for _, t := range d.Config.TableInclude {
		args = append(args, "--table", t)
	}
	for _, t := range d.Config.TableExclude {
		args = append(args, "--exclude-table", t)
	}
	if len(d.Config.ExtraOptions) > 0 {
		args = append(args, d.Config.ExtraOptions...)
	}
	args = append(args, d.dsn())
	return args, nil
}

func (d *Driver) dsn() string {
	return fmt.Sprintf("host=%s port=%d dbname=%s user=%s sslmode=%s connect_timeout=%d", d.Config.Host, d.Config.Port, d.Config.Database, d.Config.Username, nz(d.Config.SSLMode, "prefer"), int(nzDuration(d.Config.ConnectTimeout, 10*time.Second).Seconds()))
}

func (d *Driver) env() []ports.EnvironmentVariable {
	timeout := strconv.Itoa(int(nzDuration(d.Config.ConnectTimeout, 10*time.Second).Seconds()))
	env := []ports.EnvironmentVariable{
		{Name: "PGCONNECT_TIMEOUT", Value: timeout},
	}
	if d.Config.Host != "" {
		env = append(env, ports.EnvironmentVariable{Name: "PGHOST", Value: d.Config.Host})
	}
	if d.Config.Port != 0 {
		env = append(env, ports.EnvironmentVariable{Name: "PGPORT", Value: strconv.Itoa(d.Config.Port)})
	}
	if d.Config.Username != "" {
		env = append(env, ports.EnvironmentVariable{Name: "PGUSER", Value: d.Config.Username})
	}
	if d.Config.Database != "" {
		env = append(env, ports.EnvironmentVariable{Name: "PGDATABASE", Value: d.Config.Database})
	}
	if d.Config.SSLMode != "" {
		env = append(env, ports.EnvironmentVariable{Name: "PGSSLMODE", Value: d.Config.SSLMode})
	}
	pwd, err := d.password()
	if err == nil && pwd != "" {
		env = append(env, ports.EnvironmentVariable{Name: "PGPASSWORD", Value: pwd, Sensitive: true})
	}
	return env
}

func (d *Driver) password() (string, error) {
	r := d.Config.Password
	switch {
	case r.File != "":
		b, err := os.ReadFile(r.File)
		if err != nil {
			return "", domain.NewError(domain.ErrConfigurationInvalid, "postgres password file unreadable", err)
		}
		return strings.TrimSpace(string(b)), nil
	case r.Env != "":
		v, ok := os.LookupEnv(r.Env)
		if !ok {
			return "", domain.NewError(domain.ErrConfigurationInvalid, "postgres password environment secret "+r.Env+" is not set", nil)
		}
		return strings.TrimSpace(v), nil
	default:
		return "", nil
	}
}

func (d *Driver) redactions() []string {
	red := make([]string, 0, 2)
	if pwd, err := d.password(); err == nil && pwd != "" {
		red = append(red, pwd)
	}
	return red
}

func (d *Driver) version(ctx context.Context, exe string) (domain.Version, error) {
	if d.Runner == nil {
		return domain.Version{}, domain.NewError(domain.ErrToolMissing, "process runner is required", nil)
	}
	var out strings.Builder
	res, err := d.Runner.Run(ctx, ports.ProcessRequest{Executable: exe, Arguments: []string{"--version"}, StandardOutput: &out, Timeout: 10 * time.Second})
	if err != nil || res.ExitCode != 0 {
		if err == nil {
			err = errors.New(res.StdErr)
		}
		return domain.Version{}, domain.NewError(domain.ErrToolMissing, exe+" not available", err)
	}
	return parseVersion(out.String()), nil
}

func parseVersion(s string) domain.Version {
	fields := strings.Fields(s)
	raw := s
	for _, f := range fields {
		if len(f) > 0 && f[0] >= '0' && f[0] <= '9' {
			raw = f
			break
		}
	}
	parts := strings.Split(strings.Trim(raw, " \n\t,"), ".")
	v := domain.Version{Raw: strings.TrimSpace(raw)}
	if len(parts) > 0 {
		v.Major, _ = strconv.Atoi(numPrefix(parts[0]))
	}
	if len(parts) > 1 {
		v.Minor, _ = strconv.Atoi(numPrefix(parts[1]))
	}
	if len(parts) > 2 {
		v.Patch, _ = strconv.Atoi(numPrefix(parts[2]))
	}
	return v
}

func numPrefix(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

func mergeVersion(a, b domain.Version) error {
	_ = b
	if a.Raw == "" {
		return domain.NewError(domain.ErrToolMissing, "postgres tools unavailable", nil)
	}
	return nil
}

func nz(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func nzDuration(v, def time.Duration) time.Duration {
	if v == 0 {
		return def
	}
	return v
}

func (d *Driver) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func RedactConfig(cfg Config) string {
	cfg.Password = SecretReference{File: "[REDACTED]", Env: "[REDACTED]"}
	b, _ := json.Marshal(cfg)
	return string(b)
}
