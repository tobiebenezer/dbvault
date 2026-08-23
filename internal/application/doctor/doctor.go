// Package doctor performs truthful preflight health checks for the dbvault
// binary: config resolution, catalogue SQLite open+migrate, storage backend
// reachability and scratch disk free-space watermark. Run never mutates
// repository data; it only reads/creates the local catalogue file.
package doctor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dbvault/dbvault/internal/bootstrap"
	"github.com/dbvault/dbvault/internal/config"
)

// defaultMinFreeBytes is the scratch free-space floor when
// doctor.min_free_bytes is unset: enough headroom for one more chunk staging
// pass on a small VPS without making the check brittle.
const defaultMinFreeBytes = int64(1 << 30) // 1 GiB

type Check struct {
	Name   string
	Status string // "ok" or "fail"
	Detail string
}

func (c Check) OK() bool { return c.Status == "ok" }

type Report struct {
	Checks []Check
}

func (r Report) Healthy() bool {
	for _, c := range r.Checks {
		if !c.OK() {
			return false
		}
	}
	return true
}

// Run executes every check against the config at configPath. A config that
// fails to load aborts the remaining checks (there is nothing to check
// against); all other failures are collected so one run lists every problem.
func Run(ctx context.Context, configPath string, logger *slog.Logger) (Report, error) {
	if logger == nil {
		logger = slog.Default()
	}
	report := Report{}

	cfg, err := config.Load(configPath)
	if err != nil {
		report.Checks = append(report.Checks, Check{Name: "config", Status: "fail", Detail: err.Error()})
		return report, nil
	}
	report.Checks = append(report.Checks, Check{Name: "config", Status: "ok", Detail: configPath})

	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	app, err := bootstrap.Build(checkCtx, cfg, logger)
	if err != nil {
		report.Checks = append(report.Checks,
			Check{Name: "catalogue", Status: "fail", Detail: fmt.Sprintf("bootstrap failed (sqlite open/migrate or storage setup): %v", err)},
			Check{Name: "storage", Status: "fail", Detail: "not reachable because bootstrap failed"},
		)
		return report, nil
	}
	defer app.Close(context.Background())
	report.Checks = append(report.Checks, Check{Name: "catalogue", Status: "ok", Detail: "sqlite opened and migrated at " + cfg.Catalogue.Path})

	sctx, scancel := context.WithTimeout(ctx, 15*time.Second)
	defer scancel()
	if err := app.ObjectStore.Validate(sctx); err != nil {
		report.Checks = append(report.Checks, Check{Name: "storage", Status: "fail", Detail: err.Error()})
	} else {
		report.Checks = append(report.Checks, Check{Name: "storage", Status: "ok", Detail: "backend reachable"})
	}

	minFree := cfg.Doctor.MinFreeBytes
	if minFree <= 0 {
		minFree = defaultMinFreeBytes
	}
	if c, err := scratchCheck(cfg.Server.ScratchDirectory, minFree); err != nil {
		report.Checks = append(report.Checks, Check{Name: "scratch", Status: "fail", Detail: err.Error()})
	} else {
		report.Checks = append(report.Checks, c)
	}
	return report, nil
}

// scratchCheck reports the free bytes on the filesystem holding dir against
// the watermark.
func scratchCheck(dir string, minFree int64) (Check, error) {
	if dir == "" {
		return Check{}, fmt.Errorf("server.scratch_directory is empty")
	}
	probe := dir
	if _, err := os.Stat(probe); err != nil {
		// The directory may not exist yet before first boot; probe its parent.
		probe = filepath.Dir(probe)
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(probe, &st); err != nil {
		return Check{}, fmt.Errorf("statfs %s: %w", probe, err)
	}
	free := int64(st.Bavail) * int64(st.Bsize)
	detail := fmt.Sprintf("%d free bytes on %q, watermark %d", free, probe, minFree)
	if free < minFree {
		return Check{}, fmt.Errorf("only %s free below watermark: %s", human(free), detail)
	}
	return Check{Name: "scratch", Status: "ok", Detail: detail}, nil
}

func human(n int64) string {
	switch {
	case n >= 1<<40:
		return fmt.Sprintf("%.1fTiB", float64(n)/(1<<40))
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMiB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
