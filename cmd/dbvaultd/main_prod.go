//go:build !restricted

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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robfig/cron/v3"

	"github.com/dbvault/dbvault/internal/application/backup"
	"github.com/dbvault/dbvault/internal/bootstrap"
	"github.com/dbvault/dbvault/internal/config"
	"github.com/dbvault/dbvault/internal/domain"
)

var (
	backupRuns  = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "dbvault_backup_runs_total", Help: "Total DBVault backup runs."}, []string{"source", "status"})
	activeJobs  = prometheus.NewGauge(prometheus.GaugeOpts{Name: "dbvault_active_jobs", Help: "Active DBVault jobs."})
	lastSuccess = prometheus.NewGauge(prometheus.GaugeOpts{Name: "dbvault_last_success_timestamp_seconds", Help: "Unix timestamp of the last successful backup."})
)

func init() { prometheus.MustRegister(backupRuns, activeJobs, lastSuccess) }

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
	ready := false
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if !ready {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ready\n"))
	})
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: cfg.Server.Listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http.failed", "error", err)
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if hasEnabledSchedule(cfg) {
		startCron(ctx, app, logger)
	}
	ready = true
	logger.Info("dbvaultd.started", "listen", cfg.Server.Listen)
	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	logger.Info("dbvaultd.stopped")
}

func startCron(ctx context.Context, app *bootstrap.Application, logger *slog.Logger) {
	schedule := firstSchedule(app.Config)
	loc, err := time.LoadLocation(schedule.Timezone)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	c := cron.New(cron.WithLocation(loc))
	expr := schedule.Cron
	if expr == "" {
		expr = "0 */6 * * *"
	}
	_, err = c.AddFunc(expr, func() {
		activeJobs.Inc()
		defer activeJobs.Dec()
		src := scheduledSource(app)
		res, err := app.Backup.Create(ctx, backup.Command{Source: src, Trigger: domain.TriggerSchedule})
		if err != nil {
			backupRuns.WithLabelValues(app.Binding.Source.ID, "failed").Inc()
			logger.Error("scheduled_backup.failed", "error", err)
			return
		}
		backupRuns.WithLabelValues(app.Binding.Source.ID, string(res.Run.Status)).Inc()
		if !res.Duplicate {
			lastSuccess.Set(float64(time.Now().Unix()))
		}
		logger.Info("scheduled_backup.finished", "status", res.Run.Status, "snapshot", fmt.Sprint(res.Snapshot.ID))
	})
	if err != nil {
		logger.Error("scheduler.failed", "error", err)
		return
	}
	c.Start()
	go func() { <-ctx.Done(); stopCtx := c.Stop(); <-stopCtx.Done() }()
}

func hasEnabledSchedule(cfg config.Config) bool {
	for _, schedule := range cfg.Schedules {
		if schedule.Enabled == nil || *schedule.Enabled {
			return true
		}
	}
	return cfg.Schedule.Enabled
}
func firstSchedule(cfg config.Config) config.ScheduleConfig {
	for _, schedule := range cfg.Schedules {
		if schedule.Enabled == nil || *schedule.Enabled {
			return schedule
		}
	}
	return config.ScheduleConfig{ID: "legacy", Source: cfg.Source.ID, Operation: "logical_backup", Cron: cfg.Schedule.Cron, Timezone: cfg.Schedule.Timezone}
}
func scheduledSource(app *bootstrap.Application) domain.Source {
	s := app.Binding.Source
	path := ""
	if s.SQLite != nil {
		path = s.SQLite.Path
	}
	return domain.Source{ID: domain.SourceID(s.ID), Name: s.ID, Driver: s.Engine, Enabled: s.IsEnabled(), Path: path, RepositoryID: domain.RepositoryID(s.Repository)}
}
