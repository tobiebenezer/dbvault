//go:build !restricted

package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Driver struct {
	PagesPerStep int
	StepDelay    time.Duration
	BusyDelay    time.Duration
	MaximumBusy  time.Duration
}

func New(_ string) *Driver {
	return &Driver{PagesPerStep: 256, StepDelay: 25 * time.Millisecond, BusyDelay: 100 * time.Millisecond, MaximumBusy: 5 * time.Minute}
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
	f, err := os.Open(source.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, 16)
	if _, err := f.Read(header); err != nil {
		return err
	}
	if string(header) != "SQLite format 3\x00" {
		return domain.NewError(domain.ErrSourceNotSQLite, "file does not have SQLite header", nil)
	}
	return nil
}

func (d *Driver) Inspect(ctx context.Context, source domain.Source) (domain.SourceInspection, error) {
	if err := d.Validate(ctx, source); err != nil {
		return domain.SourceInspection{}, err
	}
	st, _ := os.Stat(source.Path)
	db, err := openSQLiteReadOnly(source.Path)
	if err != nil {
		return domain.SourceInspection{}, err
	}
	defer db.Close()
	pageSize := queryInt(db, "PRAGMA page_size;", 4096)
	pageCount := queryInt64(db, "PRAGMA page_count;", st.Size()/int64(pageSize))
	journal := queryString(db, "PRAGMA journal_mode;", "unknown")
	version := queryString(db, "select sqlite_version();", "unknown")
	digest := schemaDigest(db)
	return domain.SourceInspection{EngineVersion: version, FileSize: st.Size(), PageSize: pageSize, PageCount: pageCount, JournalMode: journal, SchemaDigest: digest}, nil
}

func (d *Driver) CreateSnapshot(ctx context.Context, req ports.SnapshotRequest) (ports.SnapshotArtifact, error) {
	if err := d.Validate(ctx, req.Source); err != nil {
		return nil, err
	}
	out := filepath.Join(req.ScratchDirectory, "snapshot.sqlite")
	_ = os.Remove(out)
	if err := d.nativeBackup(ctx, req.Source.Path, out); err != nil {
		return nil, domain.NewError(domain.ErrSnapshotFailed, "native sqlite backup failed", err)
	}
	db, err := sql.Open("sqlite3", out)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if ok := strings.TrimSpace(queryString(db, "PRAGMA quick_check;", "failed")); ok != "ok" {
		return nil, domain.NewError(domain.ErrQuickCheckFailed, ok, nil)
	}
	st, err := os.Stat(out)
	if err != nil {
		return nil, err
	}
	pageSize := queryInt(db, "PRAGMA page_size;", 4096)
	pageCount := queryInt64(db, "PRAGMA page_count;", st.Size()/int64(pageSize))
	meta := ports.SnapshotMetadata{EngineVersion: queryString(db, "select sqlite_version();", "unknown"), PageSize: pageSize, PageCount: pageCount, SchemaDigest: schemaDigest(db)}
	return &Artifact{path: out, size: st.Size(), meta: meta, cleanup: true}, nil
}

func (d *Driver) nativeBackup(ctx context.Context, sourcePath, destPath string) error {
	srcDB, err := openSQLiteReadOnly(sourcePath)
	if err != nil {
		return err
	}
	defer srcDB.Close()
	dstDB, err := sql.Open("sqlite3", destPath)
	if err != nil {
		return err
	}
	defer dstDB.Close()
	srcConn, err := srcDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer srcConn.Close()
	dstConn, err := dstDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer dstConn.Close()
	var backup *sqlite3.SQLiteBackup
	err = dstConn.Raw(func(dst any) error {
		return srcConn.Raw(func(src any) error {
			dc, ok := dst.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("destination is not sqlite3 connection")
			}
			sc, ok := src.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("source is not sqlite3 connection")
			}
			var err error
			backup, err = dc.Backup("main", sc, "main")
			return err
		})
	})
	if err != nil {
		return err
	}
	defer backup.Finish()
	pages := d.PagesPerStep
	if pages <= 0 {
		pages = 256
	}
	stepDelay := d.StepDelay
	if stepDelay <= 0 {
		stepDelay = 25 * time.Millisecond
	}
	busyDelay := d.BusyDelay
	if busyDelay <= 0 {
		busyDelay = 100 * time.Millisecond
	}
	maxBusy := d.MaximumBusy
	if maxBusy <= 0 {
		maxBusy = 5 * time.Minute
	}
	lastProgress := time.Now()
	lastRemaining := -1
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		done, err := backup.Step(pages)
		if err != nil {
			if time.Since(lastProgress) > maxBusy {
				return domain.NewError(domain.ErrSourceBusy, "sqlite source remained busy", err)
			}
			time.Sleep(busyDelay)
			continue
		}
		remaining := backup.Remaining()
		if remaining != lastRemaining {
			lastRemaining = remaining
			lastProgress = time.Now()
		}
		if done {
			break
		}
		time.Sleep(stepDelay)
	}
	return fsyncFileAndDir(destPath)
}

func openSQLiteReadOnly(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&_busy_timeout=5000", path))
}
func queryString(db *sql.DB, q, fallback string) string {
	var out string
	if err := db.QueryRow(q).Scan(&out); err != nil {
		return fallback
	}
	return strings.TrimSpace(out)
}
func queryInt(db *sql.DB, q string, fallback int) int {
	var out int
	if err := db.QueryRow(q).Scan(&out); err != nil {
		return fallback
	}
	return out
}
func queryInt64(db *sql.DB, q string, fallback int64) int64 {
	var out int64
	if err := db.QueryRow(q).Scan(&out); err != nil {
		return fallback
	}
	return out
}
func schemaDigest(db *sql.DB) string {
	rows, err := db.Query(`SELECT type, name, tbl_name, COALESCE(sql,'') FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name, tbl_name`)
	if err != nil {
		return "unknown"
	}
	defer rows.Close()
	h := sha256.New()
	for rows.Next() {
		var t, n, tbl, s string
		_ = rows.Scan(&t, &n, &tbl, &s)
		_, _ = h.Write([]byte(t + "\x00" + n + "\x00" + tbl + "\x00" + s + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}
func fsyncFileAndDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	_ = f.Close()
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
