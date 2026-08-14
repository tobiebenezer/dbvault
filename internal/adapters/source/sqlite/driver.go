//go:build restricted

package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Driver struct{ SQLite3Path string }

func New(sqlite3Path string) *Driver {
	if sqlite3Path == "" {
		sqlite3Path = "sqlite3"
	}
	return &Driver{SQLite3Path: sqlite3Path}
}
func (d *Driver) Name() string { return "sqlite" }

func (d *Driver) Validate(ctx context.Context, source domain.Source) error {
	st, err := os.Stat(source.Path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return domain.NewError(domain.ErrSourceNotSQLite, "source is a directory", nil)
	}
	return nil
}

func (d *Driver) Inspect(ctx context.Context, source domain.Source) (domain.SourceInspection, error) {
	if err := d.Validate(ctx, source); err != nil {
		return domain.SourceInspection{}, err
	}
	st, _ := os.Stat(source.Path)
	pageSize := d.queryInt(ctx, source.Path, "PRAGMA page_size;", 4096)
	pageCount := d.queryInt64(ctx, source.Path, "PRAGMA page_count;", st.Size()/int64(pageSize))
	journal := d.queryString(ctx, source.Path, "PRAGMA journal_mode;", "unknown")
	version := d.queryString(ctx, source.Path, "select sqlite_version();", "unknown")
	digest := d.schemaDigest(ctx, source.Path)
	return domain.SourceInspection{EngineVersion: version, FileSize: st.Size(), PageSize: pageSize, PageCount: pageCount, JournalMode: journal, SchemaDigest: digest}, nil
}

func (d *Driver) CreateSnapshot(ctx context.Context, req ports.SnapshotRequest) (ports.SnapshotArtifact, error) {
	if err := d.Validate(ctx, req.Source); err != nil {
		return nil, err
	}
	out := filepath.Join(req.ScratchDirectory, "snapshot.sqlite")
	args := []string{req.Source.Path, fmt.Sprintf(".backup '%s'", strings.ReplaceAll(out, "'", "''"))}
	cmd := exec.CommandContext(ctx, d.SQLite3Path, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, domain.NewError(domain.ErrSnapshotFailed, stderr.String(), err)
	}
	if ok := d.queryString(ctx, out, "PRAGMA quick_check;", "failed"); strings.TrimSpace(ok) != "ok" {
		return nil, domain.NewError(domain.ErrQuickCheckFailed, ok, nil)
	}
	st, err := os.Stat(out)
	if err != nil {
		return nil, err
	}
	pageSize := d.queryInt(ctx, out, "PRAGMA page_size;", 4096)
	pageCount := d.queryInt64(ctx, out, "PRAGMA page_count;", st.Size()/int64(pageSize))
	meta := ports.SnapshotMetadata{EngineVersion: d.queryString(ctx, out, "select sqlite_version();", "unknown"), PageSize: pageSize, PageCount: pageCount, SchemaDigest: d.schemaDigest(ctx, out)}
	return &Artifact{path: out, size: st.Size(), meta: meta, cleanup: true}, nil
}

func (d *Driver) queryString(ctx context.Context, db, sql, fallback string) string {
	cmd := exec.CommandContext(ctx, d.SQLite3Path, "-readonly", db, sql)
	b, err := cmd.Output()
	if err != nil {
		return fallback
	}
	return strings.TrimSpace(string(b))
}
func (d *Driver) queryInt(ctx context.Context, db, sql string, fallback int) int {
	s := d.queryString(ctx, db, sql, "")
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fallback
	}
	return v
}
func (d *Driver) queryInt64(ctx context.Context, db, sql string, fallback int64) int64 {
	s := d.queryString(ctx, db, sql, "")
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return fallback
	}
	return v
}
func (d *Driver) schemaDigest(ctx context.Context, db string) string {
	q := "SELECT type||'|'||name||'|'||tbl_name||'|'||coalesce(sql,'') FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name,tbl_name;"
	cmd := exec.CommandContext(ctx, d.SQLite3Path, "-readonly", db, q)
	b, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
