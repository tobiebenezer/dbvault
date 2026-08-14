//go:build restricted

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dbvault/dbvault/internal/application/backup"
	"github.com/dbvault/dbvault/internal/bootstrap"
	"github.com/dbvault/dbvault/internal/config"
	"github.com/dbvault/dbvault/internal/domain"
)

func main() {
	cfgPath := flag.String("config", "", "config path")
	flag.Parse()
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app, err := bootstrap.Build(context.Background(), cfg, logger)
	if err != nil {
		logger.Error("startup.failed", "error", err)
		os.Exit(1)
	}
	defer app.Close(context.Background())
	mux := http.NewServeMux()
	ready := false
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if !ready {
			http.Error(w, "not ready", 503)
			return
		}
		_, _ = w.Write([]byte("ready\n"))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("# dbvault dependency-free metrics scaffold\ndbvault_active_jobs 0\n"))
	})
	srv := &http.Server{Addr: cfg.Server.Listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http.failed", "error", err)
		}
	}()
	ready = true
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if len(cfg.Schedules) > 0 || cfg.Schedule.Enabled {
		go runScheduler(ctx, app, logger)
	}
	logger.Info("dbvaultd.started", "listen", cfg.Server.Listen)
	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	logger.Info("dbvaultd.stopped")
}
func runScheduler(ctx context.Context, app *bootstrap.Application, logger *slog.Logger) {
	every := time.Duration(app.Config.Schedule.EverySeconds) * time.Second
	if every <= 0 {
		every = 6 * time.Hour
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			src := scheduledSource(app)
			res, err := app.Backup.Create(ctx, backup.Command{Source: src, Trigger: domain.TriggerSchedule})
			if err != nil {
				logger.Error("scheduled_backup.failed", "error", err)
				continue
			}
			if res.Duplicate {
				logger.Info("scheduled_backup.duplicate", "snapshot", res.Snapshot.ID)
			} else {
				logger.Info("scheduled_backup.committed", "snapshot", res.Snapshot.ID)
			}
		}
	}
}

func scheduledSource(app *bootstrap.Application) domain.Source {
	s := app.Binding.Source
	path := ""
	if s.SQLite != nil {
		path = s.SQLite.Path
	}
	return domain.Source{ID: domain.SourceID(s.ID), Name: s.ID, Driver: s.Engine, Enabled: s.IsEnabled(), Path: path, RepositoryID: domain.RepositoryID(s.Repository)}
}
