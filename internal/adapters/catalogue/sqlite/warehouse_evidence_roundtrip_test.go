package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

func warehouseFixture() domain.WarehouseDataset {
	return domain.WarehouseDataset{
		ID:                 domain.WarehouseDatasetKey("db-1", "events"),
		DatabaseID:         "db-1",
		DatabaseName:       "App DB",
		DatasetName:        "events",
		SourceEngine:       "postgres",
		WatermarkColumn:    "created_at",
		WatermarkType:      "date",
		LastWatermarkValue: "2026-08-30",
		LastSyncMode:       "incremental",
		Columns: []domain.WarehouseColumn{
			{Name: "id", Type: "BIGINT"},
			{Name: "created_at", Type: "DATE", Nullable: true},
		},
		RowCount:       42,
		BytesRaw:       1000,
		BytesParquet:   250,
		ParquetPaths:   []string{"/lake/db=db-1/tbl=events/dt=2026-08-30/part-1.parquet"},
		SchemaVerified: true,
		LastSyncAt:     time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
		LastSyncStatus: "succeeded",
	}
}

// TestWarehouseEvidencePersistsAcrossReopen proves the catalogue adapter
// durably stores warehouse sync evidence: a record written by one process must
// still be readable after the store is closed and reopened, because the sync
// pipeline writes evidence in one run and the catalog API serves it in later
// runs.
func TestWarehouseEvidencePersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "catalogue.db")

	c, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ds := warehouseFixture()
	if err := c.UpsertWarehouseDataset(ctx, ds); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	got, ok, err := reopened.GetWarehouseDataset(ctx, "db-1", "events")
	if err != nil || !ok {
		t.Fatalf("get after reopen: ok=%v err=%v", ok, err)
	}
	if got.ID != ds.ID || got.DatabaseID != "db-1" || got.DatasetName != "events" {
		t.Fatalf("identity mismatch: %+v", got)
	}
	if got.RowCount != 42 || got.BytesRaw != 1000 || got.BytesParquet != 250 {
		t.Fatalf("measurements mismatch: %+v", got)
	}
	if got.WatermarkColumn != "created_at" || got.LastWatermarkValue != "2026-08-30" || got.LastSyncMode != "incremental" {
		t.Fatalf("watermark state mismatch: %+v", got)
	}
	if !got.SchemaVerified || got.LastSyncStatus != "succeeded" || !got.LastSyncAt.Equal(ds.LastSyncAt) {
		t.Fatalf("provenance mismatch: %+v", got)
	}
	if len(got.Columns) != 2 || got.Columns[0].Name != "id" || got.Columns[0].Type != "BIGINT" || got.Columns[1].Nullable != true {
		t.Fatalf("schema mismatch: %+v", got.Columns)
	}
	if len(got.ParquetPaths) != 1 || got.ParquetPaths[0] != ds.ParquetPaths[0] {
		t.Fatalf("parquet paths mismatch: %+v", got.ParquetPaths)
	}

	list, err := reopened.ListWarehouseDatasets(ctx, "db-1")
	if err != nil {
		t.Fatalf("list after reopen: %v", err)
	}
	if len(list) != 1 || list[0].DatasetName != "events" {
		t.Fatalf("list after reopen: %+v", list)
	}

	// The catalogue update path: a later sync replaces the prior record.
	ds.RowCount = 43
	ds.LastWatermarkValue = "2026-08-31"
	if err := reopened.UpsertWarehouseDataset(ctx, ds); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	got, _, err = reopened.GetWarehouseDataset(ctx, "db-1", "events")
	if err != nil {
		t.Fatalf("get after re-upsert: %v", err)
	}
	if got.RowCount != 43 || got.LastWatermarkValue != "2026-08-31" {
		t.Fatalf("upsert did not replace record: %+v", got)
	}
}

func TestWarehouseEvidenceMissingDataset(t *testing.T) {
	ctx := context.Background()
	c, err := Open(filepath.Join(t.TempDir(), "catalogue.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if _, ok, err := c.GetWarehouseDataset(ctx, "nope", "nothing"); ok || err != nil {
		t.Fatalf("missing dataset: ok=%v err=%v", ok, err)
	}
}
