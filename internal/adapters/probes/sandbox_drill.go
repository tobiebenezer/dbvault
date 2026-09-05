package probes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SandboxDrill orchestrates a full ephemeral sandbox restore drill against a
// local PostgreSQL instance. It creates a temporary database, runs the
// provided restore function, executes consistency probes, and always drops
// the sandbox when done.
type SandboxDrill struct {
	Host     string
	Port     int
	User     string
	Password string
	// BaseDB is the source database to clone for the sandbox schema baseline.
	BaseDB string
	// UseAmcheck enables pg_amcheck during the probe phase.
	UseAmcheck bool
}

// DrillResult records the outcome of a sandbox drill.
type DrillResult struct {
	DrillID       string
	SandboxDB     string
	StartedAt     time.Time
	CompletedAt   time.Time
	RTOSeconds    float64
	Passed        bool
	AmcheckPassed bool
	AmcheckOutput string
	Tables        []TableStat
	TotalRows     int64
	SchemaDigest  string
	Errors        []string
}

// Run executes the sandbox drill lifecycle:
//  1. Create an ephemeral sandbox database
//  2. Call restoreFn(ctx, sandboxDB) to stream backup data into it
//  3. Run consistency probes
//  4. Drop the sandbox database
//
// The drill result is always returned, even on partial failures.
func (d *SandboxDrill) Run(ctx context.Context, restoreFn func(ctx context.Context, sandboxDB string) error) (DrillResult, error) {
	start := time.Now()
	randBytes := make([]byte, 3)
	_, _ = rand.Read(randBytes)
	sandboxDB := fmt.Sprintf("%s_sandbox_%s", d.BaseDB, hex.EncodeToString(randBytes))

	result := DrillResult{
		DrillID:   "drill_" + hex.EncodeToString(randBytes),
		SandboxDB: sandboxDB,
		StartedAt: start,
	}

	// Step 1: create sandbox database
	if err := d.createSandbox(ctx, sandboxDB); err != nil {
		result.Errors = append(result.Errors, "create sandbox: "+err.Error())
		result.CompletedAt = time.Now()
		result.RTOSeconds = result.CompletedAt.Sub(start).Seconds()
		return result, err
	}

	// Ensure cleanup always runs.
	defer func() {
		// Use a fresh context in case the original was cancelled.
		cleanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = d.dropSandbox(cleanCtx, sandboxDB)
	}()

	// Step 2: restore into sandbox
	if err := restoreFn(ctx, sandboxDB); err != nil {
		result.Errors = append(result.Errors, "restore: "+err.Error())
		result.CompletedAt = time.Now()
		result.RTOSeconds = result.CompletedAt.Sub(start).Seconds()
		return result, err
	}

	// Step 3: run consistency probes
	probe := &PostgresProbe{
		Host:       d.Host,
		Port:       d.Port,
		User:       d.User,
		Password:   d.Password,
		Database:   sandboxDB,
		UseAmcheck: d.UseAmcheck,
	}
	report, err := probe.Run(ctx)
	if err != nil {
		result.Errors = append(result.Errors, "probe: "+err.Error())
		result.CompletedAt = time.Now()
		result.RTOSeconds = result.CompletedAt.Sub(start).Seconds()
		return result, err
	}

	result.CompletedAt = time.Now()
	result.RTOSeconds = result.CompletedAt.Sub(start).Seconds()
	result.AmcheckPassed = report.AmcheckPassed
	result.AmcheckOutput = report.AmcheckOutput
	result.Tables = report.Tables
	result.TotalRows = report.TotalRows
	result.SchemaDigest = report.SchemaDigest
	result.Errors = append(result.Errors, report.Errors...)
	result.Passed = report.AmcheckPassed && len(report.Errors) == 0
	return result, nil
}

func (d *SandboxDrill) createSandbox(ctx context.Context, sandboxDB string) error {
	q := fmt.Sprintf(`CREATE DATABASE "%s"`, sandboxDB)
	return d.adminPsql(ctx, q)
}

func (d *SandboxDrill) dropSandbox(ctx context.Context, sandboxDB string) error {
	// Terminate all connections to the sandbox first
	termQ := fmt.Sprintf(
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid()`,
		sandboxDB,
	)
	_ = d.adminPsql(ctx, termQ)

	q := fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, sandboxDB)
	return d.adminPsql(ctx, q)
}

// adminPsql connects to the postgres system database to run admin DDL.
func (d *SandboxDrill) adminPsql(ctx context.Context, query string) error {
	port := "5432"
	if d.Port != 0 {
		port = fmt.Sprintf("%d", d.Port)
	}
	args := []string{
		"-h", d.Host,
		"-p", port,
		"-U", d.User,
		"-d", "postgres",
		"-t", "-A",
		"-c", query,
	}
	cmd := exec.CommandContext(ctx, "psql", args...)
	if d.Password != "" {
		cmd.Env = []string{"PGPASSWORD=" + d.Password}
	}
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql admin error: %w: %s", err, errBuf.String())
	}
	return nil
}
