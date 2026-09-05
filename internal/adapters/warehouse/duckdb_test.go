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

// TestParseJSONObjectsInOrderPreservesKeyOrder locks the column-order
// contract: result columns must appear in the exact order the engine emitted
// them. Decoding into map[string]any randomized that order per run and once
// broke CSV exports mid-column.
func TestParseJSONObjectsInOrderPreservesKeyOrder(t *testing.T) {
	// Keys deliberately NOT alphabetical: zeta first, alpha last.
	in := []byte(`[
		{"zeta": 1, "middle": "x", "alpha": true},
		{"zeta": 2, "middle": "y", "alpha": false}
	]`)
	keys, objs, err := parseJSONObjectsInOrder(in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{"zeta", "middle", "alpha"}
	if len(keys) != len(want) {
		t.Fatalf("keys=%v want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("key order=%v want %v (order must follow JSON appearance, not map iteration)", keys, want)
		}
	}
	if len(objs) != 2 {
		t.Fatalf("rows=%d want 2", len(objs))
	}
	if got := rawToAny(objs[1]["middle"]); got != "y" {
		t.Errorf("value decode=%v want y", got)
	}
	if got := rawToAny(objs[0]["zeta"]); got != float64(1) {
		t.Errorf("number decode=%v want 1", got)
	}
	if got := rawToAny(objs[0]["alpha"]); got != true {
		t.Errorf("bool decode=%v want true", got)
	}
}

// TestParseJSONObjectsInOrderEmptyAndMalformed covers the degenerate shapes
// both CLIs can emit.
func TestParseJSONObjectsInOrderEmptyAndMalformed(t *testing.T) {
	keys, objs, err := parseJSONObjectsInOrder([]byte("[]"))
	if err != nil || keys != nil || len(objs) != 0 {
		t.Fatalf("empty array: keys=%v objs=%v err=%v", keys, objs, err)
	}
	if _, _, err := parseJSONObjectsInOrder([]byte(`{"not":"an array"}`)); err == nil {
		t.Fatal("object input must error")
	}
	if _, _, err := parseJSONObjectsInOrder([]byte(`[{"a":`)); err == nil {
		t.Fatal("truncated input must error")
	}
	if v := rawToAny(nil); v != nil {
		t.Fatalf("rawToAny(nil)=%v want nil", v)
	}
}
