package warehouse

import (
	"context"
	"testing"
)

func TestDuckDBEngineAndQueryExecution(t *testing.T) {
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
	t.Logf("Query executed in %dms with %d rows", res.ExecutionMs, res.RowCount)
}

func TestParquetTransformer(t *testing.T) {
	pt := NewParquetTransformer("./test_lakehouse")
	cols := []ColumnMeta{{Name: "id", Type: "BIGINT"}, {Name: "email", Type: "VARCHAR"}}
	res, err := pt.TransformTable(context.Background(), "cribx_test", "users", cols, 10000, 2000000)
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}
	if res.ParquetSizeBytes <= 0 || res.CompressionRatio < 1.0 {
		t.Errorf("invalid compression result: %+v", res)
	}
}
