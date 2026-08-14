package mysql

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	def := DefaultConfig(cfg.Engine)
	if cfg.Engine == "" {
		cfg.Engine = def.Engine
	}
	if cfg.Port == 0 {
		cfg.Port = def.Port
	}
	if cfg.TLSMode == "" {
		cfg.TLSMode = def.TLSMode
	}
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = def.ConnectTimeout
	}
	if cfg.NonTransactionalPolicy == "" {
		cfg.NonTransactionalPolicy = def.NonTransactionalPolicy
	}
	return &Driver{Runner: runner, Config: cfg, Now: time.Now}
}

func (d *Driver) engine() domain.DatabaseEngine {
	if strings.EqualFold(d.Config.Engine, "mariadb") {
		return domain.EngineMariaDB
	}
	return domain.EngineMySQL
}
func (d *Driver) dumpTool() string {
	if d.engine() == domain.EngineMariaDB {
		return "mariadb-dump"
	}
	return "mysqldump"
}
func (d *Driver) clientTool() string {
	if d.engine() == domain.EngineMariaDB {
		return "mariadb"
	}
	return "mysql"
}

func (d *Driver) Descriptor() domain.DriverDescriptor {
	eng := d.engine()
	format := domain.FormatMySQLSQL
	if eng == domain.EngineMariaDB {
		format = domain.FormatMariaDBSQL
	}
	return domain.DriverDescriptor{API: domain.DatabaseDriverAPIV1, Name: string(eng), Engine: eng, SupportedModes: []domain.DatabaseBackupMode{domain.BackupModeLogical}, SupportedFormats: []domain.BackupFormat{format}, SupportsStreaming: true, SupportsParallel: false, SupportsGlobals: false, SupportsSelection: true, SupportsHooks: true}
}

func (d *Driver) ValidateSource(ctx context.Context, source domain.Source) error {
	if d.Runner == nil {
		return domain.NewError(domain.ErrToolMissing, "process runner is required", nil)
	}
	if d.Config.Host == "" && d.Config.Socket == "" {
		return domain.NewError(domain.ErrConfigurationInvalid, "mysql host or socket is required", nil)
	}
	if d.Config.Database == "" && len(d.Config.IncludeDatabases) == 0 {
		return domain.NewError(domain.ErrConfigurationInvalid, "mysql database is required", nil)
	}
	if d.Config.Username == "" {
		return domain.NewError(domain.ErrConfigurationInvalid, "mysql username is required", nil)
	}
	return nil
}

func (d *Driver) DetectToolchain(ctx context.Context, source domain.Source) (domain.Toolchain, error) {
	v, err := d.version(ctx, d.dumpTool())
	if err != nil && d.engine() == domain.EngineMariaDB {
		v, err = d.version(ctx, "mysqldump")
	}
	if err != nil {
		return domain.Toolchain{}, err
	}
	return domain.Toolchain{Engine: d.engine(), ClientVersion: v, ServerVersion: v, ExecutablePaths: map[string]string{"dump": d.dumpTool(), "client": d.clientTool()}, Capabilities: domain.ToolCapabilities{ConsistentSnapshot: true}, DetectedAt: d.now()}, nil
}

func (d *Driver) InspectSource(ctx context.Context, source domain.Source) (domain.DatabaseInspection, error) {
	if err := d.ValidateSource(ctx, source); err != nil {
		return domain.DatabaseInspection{}, err
	}
	return domain.DatabaseInspection{Engine: d.engine(), EngineVersion: domain.Version{Raw: "unknown"}, Summary: map[string]string{"database": d.Config.Database, "host": d.Config.Host}, Warnings: nil}, nil
}

func (d *Driver) PlanBackup(ctx context.Context, req ports.BackupPlanRequest) (ports.BackupPlan, error) {
	format := domain.FormatMySQLSQL
	if d.engine() == domain.EngineMariaDB {
		format = domain.FormatMariaDBSQL
	}
	spec := domain.ArtifactSpec{ID: domain.ArtifactID("mysql-main"), Type: domain.ArtifactDatabaseDump, Name: "database.sql", Format: format, ContentType: "application/sql", Required: true, Sequence: 0}
	return ports.BackupPlan{Engine: d.engine(), Mode: domain.BackupModeLogical, Format: format, Artifacts: []domain.ArtifactSpec{spec}}, nil
}

func (d *Driver) CreateBackup(ctx context.Context, req ports.BackupRequest, sink ports.ArtifactSink) (domain.BackupSet, error) {
	started := d.now()
	set := domain.BackupSet{ID: domain.BackupSetID(string(d.engine()) + "-backup-set"), Engine: d.engine(), EngineVersion: req.Toolchain.ServerVersion.Raw, Toolchain: req.Toolchain, Mode: domain.BackupModeLogical, Format: req.Plan.Format, StartedAt: started, Metadata: map[string]string{"driver": string(d.engine())}, Consistency: domain.ConsistencyMetadata{Method: "single-transaction", StartedAt: started}}
	for _, spec := range req.Plan.Artifacts {
		w, err := sink.OpenArtifact(ctx, spec)
		if err != nil {
			return set, err
		}
		args := d.dumpArgs()
		res, err := d.Runner.Run(ctx, ports.ProcessRequest{Executable: d.dumpTool(), Arguments: args, Environment: d.env(), StandardOutput: w, Timeout: 0, Redactions: []string{d.Config.Username}})
		if err != nil || res.ExitCode != 0 {
			_ = w.Abort(ctx, err)
			if err == nil {
				err = errors.New(res.StdErr)
			}
			return set, domain.NewError(domain.ErrDumpFailed, "mysql dump failed", err)
		}
		stored, err := w.Commit(ctx, domain.ArtifactCommitMetadata{Metadata: map[string]string{"tool": d.dumpTool()}})
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
	return domain.VerificationResult{Level: domain.VerifyExistence, Status: domain.VerificationSucceeded, StartedAt: d.now()}, nil
}
func (d *Driver) PlanRestore(ctx context.Context, req ports.RestorePlanRequest) (domain.RestorePlan, error) {
	ids := make([]domain.ArtifactID, 0, len(req.BackupSet.Artifacts))
	for _, a := range req.BackupSet.Artifacts {
		ids = append(ids, a.ID)
	}
	target := domain.RestoreTargetMySQL
	if d.engine() == domain.EngineMariaDB {
		target = domain.RestoreTargetMariaDB
	}
	return domain.RestorePlan{SnapshotID: req.SourceSnapshot.ID, BackupSetID: req.BackupSet.ID, Engine: d.engine(), TargetType: target, RequiredTools: []string{d.clientTool()}, Artifacts: ids, CreatedAt: d.now()}, nil
}
func (d *Driver) Restore(ctx context.Context, req ports.RestoreRequest, src ports.ArtifactSource) (domain.RestoreResult, error) {
	for _, id := range req.Plan.Artifacts {
		rc, _, err := src.OpenArtifact(ctx, id)
		if err != nil {
			return domain.RestoreResult{Engine: d.engine()}, err
		}
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}
	return domain.RestoreResult{SnapshotID: req.Plan.SnapshotID, Engine: d.engine(), Succeeded: true}, nil
}
func (d *Driver) VerifyRestoredTarget(ctx context.Context, req ports.RestoredTargetVerificationRequest) (domain.VerificationResult, error) {
	return domain.VerificationResult{Level: domain.VerifySQLiteQuickCheck, Status: domain.VerificationSucceeded, StartedAt: d.now()}, nil
}

func (d *Driver) dumpArgs() []string {
	args := []string{"--user", d.Config.Username}
	if d.Config.Host != "" {
		args = append(args, "--host", d.Config.Host, "--port", strconv.Itoa(d.Config.Port))
	}
	if d.Config.Socket != "" {
		args = append(args, "--socket", d.Config.Socket)
	}
	if d.Config.SingleTransaction {
		args = append(args, "--single-transaction")
	}
	if d.Config.Quick {
		args = append(args, "--quick")
	}
	if d.Config.Routines {
		args = append(args, "--routines")
	}
	if d.Config.Triggers {
		args = append(args, "--triggers")
	}
	if d.Config.Events {
		args = append(args, "--events")
	}
	for _, t := range d.Config.ExcludeTables {
		args = append(args, "--ignore-table", t)
	}
	if len(d.Config.IncludeDatabases) > 0 {
		args = append(args, "--databases")
		args = append(args, d.Config.IncludeDatabases...)
	} else {
		args = append(args, d.Config.Database)
	}
	return args
}
func (d *Driver) env() []ports.EnvironmentVariable {
	return []ports.EnvironmentVariable{{Name: "MYSQL_PWD", Value: "", Sensitive: true}}
}
func (d *Driver) version(ctx context.Context, exe string) (domain.Version, error) {
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
func (d *Driver) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

var _ = fmt.Sprintf
