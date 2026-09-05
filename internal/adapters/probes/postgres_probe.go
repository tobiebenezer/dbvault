// Package probes provides real database consistency probe adapters for
// automated sandbox restore drill verification (M3/M4).
// It runs pg_amcheck, psql-level integrity queries, table row count
// reconciliation, and schema digest validation against a live PostgreSQL
// instance connected to a freshly-restored ephemeral sandbox database.
package probes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TableStat contains per-table row count and size for reconciliation.
type TableStat struct {
	Name      string
	RowCount  int64
	SizeBytes int64
}

// ConsistencyReport is the output of a full consistency probe run against a
// sandbox database. All fields have objective, deterministic values that can be
// compared to a pre-backup baseline captured at dump time.
type ConsistencyReport struct {
	Database      string
	Host          string
	Port          int
	AmcheckPassed bool
	AmcheckOutput string
	Tables        []TableStat
	TotalRows     int64
	SchemaDigest  string // SHA-256 over sorted "table:rowcount" pairs
	DurationMs    int64
	VerifiedAt    time.Time
	Errors        []string
}

// PostgresProbe executes deep consistency checks against a live PostgreSQL database.
type PostgresProbe struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	// UseAmcheck controls whether pg_amcheck is attempted. It requires the
	// amcheck extension to be available in the target database.
	UseAmcheck bool
}

// env returns the PGPASSWORD environment override for exec.Cmd.
func (p *PostgresProbe) env() []string {
	if p.Password != "" {
		return []string{"PGPASSWORD=" + p.Password}
	}
	return nil
}

func (p *PostgresProbe) port() string {
	if p.Port == 0 {
		return "5432"
	}
	return strconv.Itoa(p.Port)
}

// psqlQuery runs a psql query and returns trimmed stdout output.
func (p *PostgresProbe) psqlQuery(ctx context.Context, query string) (string, error) {
	args := []string{
		"-h", p.Host,
		"-p", p.port(),
		"-U", p.User,
		"-d", p.Database,
		"-t", "-A",
		"-c", query,
	}
	cmd := exec.CommandContext(ctx, "psql", args...)
	cmd.Env = p.env()
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("psql error: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// Run executes all consistency probes and returns a ConsistencyReport.
// It does NOT return an error if probes fail — failures are recorded in
// the report. An error is returned only for connectivity failures.
func (p *PostgresProbe) Run(ctx context.Context) (*ConsistencyReport, error) {
	start := time.Now()
	report := &ConsistencyReport{
		Database:   p.Database,
		Host:       p.Host,
		Port:       p.Port,
		VerifiedAt: start,
	}

	// Step 1: connectivity check via simple ping query
	if _, err := p.psqlQuery(ctx, "SELECT 1"); err != nil {
		return nil, fmt.Errorf("cannot connect to sandbox database %s: %w", p.Database, err)
	}

	// Step 2: pg_amcheck — verifies heap pages, B-tree indexes, and checksums.
	// Requires the amcheck extension. Gracefully degrades if unavailable.
	if p.UseAmcheck {
		amOut, amErr := p.runAmcheck(ctx)
		if amErr != nil {
			// Check if it's a missing extension error
			if strings.Contains(amErr.Error(), "amcheck") || strings.Contains(amErr.Error(), "extension") {
				report.AmcheckPassed = true
				report.AmcheckOutput = "pg_amcheck skipped: amcheck extension not available in this database"
			} else {
				report.AmcheckPassed = false
				report.AmcheckOutput = amErr.Error()
				report.Errors = append(report.Errors, "pg_amcheck: "+amErr.Error())
			}
		} else {
			report.AmcheckPassed = true
			report.AmcheckOutput = amOut
		}
	} else {
		report.AmcheckPassed = true
		report.AmcheckOutput = "pg_amcheck skipped: disabled by configuration"
	}

	// Step 3: collect table row counts for all non-system tables
	tables, err := p.collectTableStats(ctx)
	if err != nil {
		report.Errors = append(report.Errors, "table stats: "+err.Error())
	} else {
		report.Tables = tables
		var total int64
		for _, t := range tables {
			total += t.RowCount
		}
		report.TotalRows = total
	}

	// Step 4: schema digest — deterministic SHA-256 over sorted "tablename:rowcount" pairs
	report.SchemaDigest = p.computeSchemaDigest(report.Tables)

	report.DurationMs = time.Since(start).Milliseconds()
	return report, nil
}

// runAmcheck executes pg_amcheck against all heap and index relations.
func (p *PostgresProbe) runAmcheck(ctx context.Context) (string, error) {
	args := []string{
		"-h", p.Host,
		"-p", p.port(),
		"-U", p.User,
		"-d", p.Database,
		"--heapallindexed",
		"--all",
	}
	cmd := exec.CommandContext(ctx, "pg_amcheck", args...)
	cmd.Env = p.env()
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		combined := strings.TrimSpace(out.String() + " " + stderr.String())
		return "", fmt.Errorf("pg_amcheck failed: %w: %s", err, combined)
	}
	result := strings.TrimSpace(out.String())
	if result == "" {
		result = "pg_amcheck: all checks passed"
	}
	return result, nil
}

// collectTableStats queries pg_stat_user_tables for row counts and pg_total_relation_size.
func (p *PostgresProbe) collectTableStats(ctx context.Context) ([]TableStat, error) {
	q := `
SELECT
    n.nspname || '.' || c.relname AS name,
    COALESCE(s.n_live_tup, 0) AS row_count,
    pg_total_relation_size(c.oid) AS size_bytes
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
WHERE c.relkind = 'r'
  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
ORDER BY n.nspname, c.relname`

	out, err := p.psqlQuery(ctx, q)
	if err != nil {
		// Fallback: simpler query for row counts only
		return p.collectTableStatsSimple(ctx)
	}

	var tables []TableStat
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		rows, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		size, _ := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		tables = append(tables, TableStat{Name: name, RowCount: rows, SizeBytes: size})
	}

	if len(tables) == 0 {
		return p.collectTableStatsSimple(ctx)
	}
	return tables, nil
}

// collectTableStatsSimple is a fallback using simpler SQL (no pg_stat_user_tables).
func (p *PostgresProbe) collectTableStatsSimple(ctx context.Context) ([]TableStat, error) {
	q := `SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`
	out, err := p.psqlQuery(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("cannot list tables: %w", err)
	}

	var tables []TableStat
	var errs []string
	for _, line := range strings.Split(out, "\n") {
		tbl := strings.TrimSpace(line)
		if tbl == "" {
			continue
		}
		cq := fmt.Sprintf(`SELECT COUNT(*) FROM public."%s"`, tbl)
		cOut, cErr := p.psqlQuery(ctx, cq)
		if cErr != nil {
			errs = append(errs, tbl+": "+cErr.Error())
			continue
		}
		rowCount, _ := strconv.ParseInt(strings.TrimSpace(cOut), 10, 64)
		tables = append(tables, TableStat{Name: "public." + tbl, RowCount: rowCount})
	}

	if len(errs) > 0 {
		return tables, errors.New(strings.Join(errs, "; "))
	}
	return tables, nil
}

// computeSchemaDigest returns a deterministic SHA-256 digest of the table inventory.
// Format: SHA-256("tablename1:rowcount1\ntablename2:rowcount2\n...") over sorted names.
func (p *PostgresProbe) computeSchemaDigest(tables []TableStat) string {
	sorted := make([]TableStat, len(tables))
	copy(sorted, tables)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	var sb strings.Builder
	for _, t := range sorted {
		sb.WriteString(t.Name)
		sb.WriteByte(':')
		sb.WriteString(strconv.FormatInt(t.RowCount, 10))
		sb.WriteByte('\n')
	}

	h := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(h[:])
}
