package mysql

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

// SnapshotSource adapts the v1 MySQL driver (PlanBackup/CreateBackup) to the
// ports.SourceDriver seam consumed by the standalone backup runtime. A logical
// dump is written into scratch and surfaced as a plain snapshot artifact with
// page size 1, so chunk offsets are byte offsets and repository round-trips
// reconstruct the dump verbatim.
type SnapshotSource struct {
	Driver *Driver
}

func NewSnapshotSource(d *Driver) *SnapshotSource { return &SnapshotSource{Driver: d} }

func (s *SnapshotSource) Name() string { return string(s.Driver.engine()) }

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
	out := filepath.Join(req.ScratchDirectory, "database.sql")
	f, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	hasher := sha256.New()
	var stderr bytes.Buffer
	res, runErr := s.Driver.Runner.Run(ctx, ports.ProcessRequest{
		Executable:     s.Driver.dumpTool(),
		Arguments:      s.Driver.dumpArgs(),
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
		return nil, domain.NewError(domain.ErrDumpFailed, "mysql logical dump failed", runErr)
	}
	st, err := os.Stat(out)
	if err != nil {
		return nil, err
	}
	version := "unknown"
	if toolchain, err := s.Driver.DetectToolchain(ctx, req.Source); err == nil {
		version = toolchain.ServerVersion.Raw
	}
	meta := ports.SnapshotMetadata{EngineVersion: version, PageSize: 1, PageCount: st.Size(), SchemaDigest: digest}
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
