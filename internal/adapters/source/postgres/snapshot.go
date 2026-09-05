package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

// SnapshotSource adapts the PostgreSQL driver to the ports.SourceDriver interface
// consumed by the standalone backup runtime.
type SnapshotSource struct {
	Driver *Driver
}

func NewSnapshotSource(d *Driver) *SnapshotSource {
	return &SnapshotSource{Driver: d}
}

func (s *SnapshotSource) Name() string {
	return "postgres"
}

func (s *SnapshotSource) Validate(ctx context.Context, source domain.Source) error {
	return s.Driver.ValidateSource(ctx, source)
}

func (s *SnapshotSource) Inspect(ctx context.Context, source domain.Source) (domain.SourceInspection, error) {
	if err := s.Validate(ctx, source); err != nil {
		return domain.SourceInspection{}, err
	}
	version := "unknown"
	if toolchain, err := s.Driver.DetectToolchain(ctx, source); err == nil {
		version = toolchain.ClientVersion.Raw
	}
	return domain.SourceInspection{EngineVersion: version}, nil
}

func (s *SnapshotSource) CreateSnapshot(ctx context.Context, req ports.SnapshotRequest) (ports.SnapshotArtifact, error) {
	if err := s.Validate(ctx, req.Source); err != nil {
		return nil, err
	}
	filename := "database.dump"
	format := domain.FormatPostgresCustom
	if s.Driver.Config.BackupFormat == "postgres-plain-sql" {
		filename = "database.sql"
		format = domain.FormatPostgresPlainSQL
	}
	out := filepath.Join(req.ScratchDirectory, filename)
	f, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	spec := domain.ArtifactSpec{
		ID:          domain.ArtifactID("postgres-main"),
		Type:        domain.ArtifactDatabaseDump,
		Name:        filename,
		Format:      format,
		ContentType: "application/octet-stream",
		Required:    true,
		Sequence:    0,
	}
	cmd, err := s.Driver.commandFor(spec)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(out)
		return nil, err
	}
	hasher := sha256.New()
	var stderr bytes.Buffer
	res, runErr := s.Driver.Runner.Run(ctx, ports.ProcessRequest{
		Executable:     cmd[0],
		Arguments:      cmd[1:],
		Environment:    s.Driver.env(),
		StandardOutput: io.MultiWriter(f, hasher),
		StandardError:  &stderr,
		Redactions:     s.Driver.redactions(),
	})
	closeErr := f.Close()
	digest := hex.EncodeToString(hasher.Sum(nil))
	if runErr == nil && res.ExitCode != 0 {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = res.StdErr
		}
		runErr = &domain.AppError{Code: domain.ErrDumpFailed, Message: errMsg}
	}
	if runErr != nil || closeErr != nil {
		_ = os.Remove(out)
		if closeErr != nil && runErr == nil {
			runErr = closeErr
		}
		return nil, domain.NewError(domain.ErrDumpFailed, "postgres logical dump failed", runErr)
	}
	st, err := os.Stat(out)
	if err != nil {
		return nil, err
	}
	version := "unknown"
	if toolchain, err := s.Driver.DetectToolchain(ctx, req.Source); err == nil {
		version = toolchain.ServerVersion.Raw
	}
	meta := ports.SnapshotMetadata{
		EngineVersion: version,
		PageSize:      1,
		PageCount:     st.Size(),
		SchemaDigest:  digest,
	}
	return &dumpArtifact{path: out, size: st.Size(), meta: meta}, nil
}

type dumpArtifact struct {
	path string
	size int64
	meta ports.SnapshotMetadata
}

func (a *dumpArtifact) Path() string                     { return a.path }
func (a *dumpArtifact) Size() int64                      { return a.size }
func (a *dumpArtifact) Metadata() ports.SnapshotMetadata { return a.meta }
func (a *dumpArtifact) Close() error                     { return os.Remove(a.path) }
