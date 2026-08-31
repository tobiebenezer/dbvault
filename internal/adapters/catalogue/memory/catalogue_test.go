package memory

import (
	"context"
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
		ParquetPaths:   []string{"/lake/db=db-1/tbl=events/data.parquet"},
		SchemaVerified: true,
		LastSyncAt:     time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC),
		LastSyncStatus: "succeeded",
	}
}

func TestWarehouseEvidenceRoundTrip(t *testing.T) {
	ctx := context.Background()
	c := New()
	ds := warehouseFixture()
	if err := c.UpsertWarehouseDataset(ctx, ds); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, ok, err := c.GetWarehouseDataset(ctx, "db-1", "events")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.ID != ds.ID || got.RowCount != 42 || !got.LastSyncAt.Equal(ds.LastSyncAt) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if len(got.Columns) != 2 || got.Columns[1].Nullable != true {
		t.Fatalf("columns: %+v", got.Columns)
	}

	// Upserting the same key replaces the prior record.
	ds.RowCount = 43
	ds.LastWatermarkValue = "2026-08-31"
	if err := c.UpsertWarehouseDataset(ctx, ds); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	got, _, err = c.GetWarehouseDataset(ctx, "db-1", "events")
	if err != nil {
		t.Fatalf("get after upsert: %v", err)
	}
	if got.RowCount != 43 || got.LastWatermarkValue != "2026-08-31" {
		t.Fatalf("upsert did not replace: %+v", got)
	}

	list, err := c.ListWarehouseDatasets(ctx, "")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list all len=%d want 1", len(list))
	}
	list, err = c.ListWarehouseDatasets(ctx, "db-other")
	if err != nil || len(list) != 0 {
		t.Fatalf("list scoped: len=%d err=%v", len(list), err)
	}
}

func TestWarehouseEvidenceGetMissing(t *testing.T) {
	ctx := context.Background()
	c := New()
	if _, ok, err := c.GetWarehouseDataset(ctx, "nope", "nothing"); ok || err != nil {
		t.Fatalf("missing dataset: ok=%v err=%v", ok, err)
	}
}
