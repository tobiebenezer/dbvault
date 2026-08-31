package warehouse

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// warehouseCLIAvailable reports whether any real query-engine CLI exists on
// this host. Tests that require a working engine skip when none is installed;
// tests for the fail-closed path only run when none is installed.
func warehouseCLIAvailable() bool {
	for _, bin := range []string{"duckdb", "sqlite3"} {
		if _, err := exec.LookPath(bin); err == nil {
			return true
		}
	}
	return false
}

func TestDuckDBEngineAndQueryExecution(t *testing.T) {
	if !warehouseCLIAvailable() {
		t.Skip("no duckdb or sqlite3 CLI in PATH; query execution requires a real engine")
	}
	engine, err := NewDuckDBEngine()
	if err != nil {
		t.Fatalf("failed to initialize DuckDB engine: %v", err)
	}
	defer engine.Close()

	// Seed test data
	cols := []ColumnMeta{
		{Name: "id", Type: "INTEGER"},
		{Name: "name", Type: "TEXT"},
		{Name: "amount", Type: "REAL"},
	}
	rows := [][]any{
		{1, "Acme Corp", 4200.5},
		{2, "TechFlow", 890.0},
		{3, "Solaris AI", 6500.25},
	}
	if err := engine.SeedDataset("test_orders", cols, rows); err != nil {
		t.Fatalf("failed to seed dataset: %v", err)
	}

	// Run aggregation query
	res, err := engine.ExecuteQuery(context.Background(), QueryRequest{
		Query: "SELECT count(*) as total_orders, sum(amount) as total_volume FROM test_orders;",
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(res.Rows) == 0 {
		t.Fatalf("expected rows, got 0")
	}
	if res.RowCount != 1 {
		t.Errorf("expected 1 row, got %d", res.RowCount)
	}
	t.Logf("Query executed in %dms with %d rows via %s", res.ExecutionMs, res.RowCount, res.Engine)
}

// TestExecuteQueryFailsClosedWithoutEngine locks in the honesty guarantee:
// with no query-engine CLI installed, ExecuteQuery must return an explicit
// error naming the missing CLIs instead of silently returning wrong results.
func TestExecuteQueryFailsClosedWithoutEngine(t *testing.T) {
	if warehouseCLIAvailable() {
		t.Skip("duckdb or sqlite3 CLI present; fail-closed path is unreachable")
	}
	engine, err := NewDuckDBEngine()
	if err != nil {
		t.Fatalf("failed to initialize DuckDB engine: %v", err)
	}
	defer engine.Close()

	res, err := engine.ExecuteQuery(context.Background(), QueryRequest{
		Query: "SELECT count(*) FROM test_orders;",
	})
	if err == nil {
		t.Fatalf("expected explicit error when no query engine CLI is available, got result: %+v", res)
	}
	if !strings.Contains(err.Error(), "no query engine available") {
		t.Errorf("error should state that no query engine is available, got: %v", err)
	}
	if !strings.Contains(err.Error(), "duckdb") || !strings.Contains(err.Error(), "sqlite3") {
		t.Errorf("error should name the missing CLIs (duckdb, sqlite3), got: %v", err)
	}
}

func TestEngineStatusReflectsHost(t *testing.T) {
	engine, err := NewDuckDBEngine()
	if err != nil {
		t.Fatalf("failed to initialize DuckDB engine: %v", err)
	}
	defer engine.Close()

	want := "unavailable"
	if warehouseCLIAvailable() {
		want = "available"
	}
	if got := engine.Status(); got != want {
		t.Errorf("Status() = %q, want %q", got, want)
	}
}
