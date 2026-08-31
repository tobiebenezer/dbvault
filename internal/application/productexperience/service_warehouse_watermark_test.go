package productexperience

import (
	"context"
	"testing"
	"time"

	memory "github.com/dbvault/dbvault/internal/adapters/catalogue/memory"
	"github.com/dbvault/dbvault/internal/adapters/warehouse"
	"github.com/dbvault/dbvault/internal/domain"
)

func TestResolveWarehouseSyncModeBoundaries(t *testing.T) {
	prevDone := &domain.WarehouseDataset{
		WatermarkColumn:    "created_at",
		LastWatermarkValue: "2026-08-30",
		LastSyncStatus:     "succeeded",
	}
	tests := []struct {
		name       string
		opts       WarehouseSyncOptions
		prev       *domain.WarehouseDataset
		wantMode   string
		wantColumn string
		wantCursor string
		wantErr    bool
	}{
		{
			// Full mode still resolves the watermark column: the dataset
			// record keeps its incremental configuration across full refreshes.
			name:       "first sync is always full even when incremental requested",
			opts:       WarehouseSyncOptions{Incremental: true, WatermarkColumn: "created_at"},
			prev:       nil,
			wantMode:   "full",
			wantColumn: "created_at",
		},
		{
			name:       "incremental requested but no prior successful sync",
			opts:       WarehouseSyncOptions{Incremental: true, WatermarkColumn: "created_at"},
			prev:       &domain.WarehouseDataset{WatermarkColumn: "created_at"},
			wantMode:   "full",
			wantColumn: "created_at",
		},
		{
			name:       "incremental requested but no recorded watermark value",
			opts:       WarehouseSyncOptions{Incremental: true, WatermarkColumn: "created_at"},
			prev:       &domain.WarehouseDataset{WatermarkColumn: "created_at", LastSyncStatus: "succeeded"},
			wantMode:   "full",
			wantColumn: "created_at",
		},
		{
			name:       "full requested stays full despite usable cursor",
			opts:       WarehouseSyncOptions{WatermarkColumn: "created_at"},
			prev:       prevDone,
			wantMode:   "full",
			wantColumn: "created_at",
		},
		{
			name:       "incremental with recorded cursor",
			opts:       WarehouseSyncOptions{Incremental: true},
			prev:       prevDone,
			wantMode:   "incremental",
			wantColumn: "created_at",
			wantCursor: "2026-08-30",
		},
		{
			name:    "unsafe watermark column fails closed",
			opts:    WarehouseSyncOptions{Incremental: true, WatermarkColumn: "created_at; DROP TABLE x"},
			prev:    &domain.WarehouseDataset{WatermarkColumn: "created_at; DROP TABLE x", LastWatermarkValue: "1", LastSyncStatus: "succeeded"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, col, cursor, err := resolveWarehouseSyncMode(tt.opts, tt.prev)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got mode=%q col=%q", mode, col)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode != tt.wantMode {
				t.Fatalf("mode=%q want %q", mode, tt.wantMode)
			}
			if col != tt.wantColumn {
				t.Fatalf("column=%q want %q", col, tt.wantColumn)
			}
			if cursor != tt.wantCursor {
				t.Fatalf("cursor=%q want %q", cursor, tt.wantCursor)
			}
		})
	}
}

func TestBuildWarehouseExtractQuery(t *testing.T) {
	if got := buildWarehouseExtractQuery("events", "full", "created_at", "2026-08-30"); got != "SELECT * FROM events" {
		t.Fatalf("full query = %q", got)
	}
	got := buildWarehouseExtractQuery("events", "incremental", "created_at", "2026-08-30")
	want := "SELECT * FROM events WHERE created_at > '2026-08-30' ORDER BY created_at"
	if got != want {
		t.Fatalf("incremental query = %q want %q", got, want)
	}
	// Quote escaping keeps a value containing a single quote inert.
	got = buildWarehouseExtractQuery("events", "incremental", "name", "O'Brien")
	if got != "SELECT * FROM events WHERE name > 'O''Brien' ORDER BY name" {
		t.Fatalf("escaped query = %q", got)
	}
	// No usable cursor must never produce a WHERE clause.
	if got := buildWarehouseExtractQuery("events", "incremental", "", ""); got != "SELECT * FROM events" {
		t.Fatalf("incremental without cursor = %q", got)
	}
}

func TestWatermarkGreater(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		typ     string
		want    bool
	}{
		{"integer numeric not lexical 9vs10", "9", "10", "integer", false},
		{"integer numeric 10vs9", "10", "9", "integer", true},
		{"integer equal", "10", "10", "integer", false},
		{"integer padded zero", "010", "9", "integer", true},
		{"integer non numeric falls back lexical", "abc", "abd", "integer", false},
		{"date lexical compare", "2026-08-31", "2026-08-30", "date", true},
		{"datetime T separator", "2026-08-31T00:00:00Z", "2026-08-30T23:59:59Z", "datetime", true},
		{"text lexical", "b", "a", "text", true},
		{"text equal", "b", "b", "text", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := watermarkGreater(tt.a, tt.b, tt.typ); got != tt.want {
				t.Fatalf("watermarkGreater(%q,%q,%q)=%v want %v", tt.a, tt.b, tt.typ, got, tt.want)
			}
		})
	}
}

func TestClassifyWatermarkType(t *testing.T) {
	cols := []warehouse.ColumnMeta{
		{Name: "id", Type: "BIGINT"},
		{Name: "created_on", Type: "DATE"},
		{Name: "created_at", Type: "timestamp with time zone"},
		{Name: "updated", Type: "TIMESTAMP"},
		{Name: "name", Type: "VARCHAR"},
	}
	tests := []struct {
		column string
		want   string
	}{
		{"id", "integer"},
		{"created_on", "date"},
		{"created_at", "datetime"},
		{"updated", "datetime"},
		{"name", "text"},
		{"missing", ""},
	}
	for _, tt := range tests {
		if got := classifyWatermarkType(cols, tt.column); got != tt.want {
			t.Fatalf("classifyWatermarkType(%q)=%q want %q", tt.column, got, tt.want)
		}
	}
}

func TestParseDescribeCSV(t *testing.T) {
	out := []byte("column_name,data_type\nid,BIGINT\ncreated_at,DATE\n")
	cols, err := parseDescribeCSV(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cols) != 2 {
		t.Fatalf("len(cols)=%d want 2", len(cols))
	}
	if cols[0].Name != "id" || cols[0].Type != "BIGINT" {
		t.Fatalf("col0 = %+v", cols[0])
	}
	if cols[1].Name != "created_at" || cols[1].Type != "DATE" {
		t.Fatalf("col1 = %+v", cols[1])
	}
	if _, err := parseDescribeCSV([]byte("column_name,data_type\n")); err == nil {
		t.Fatal("expected error for empty DESCRIBE output")
	}
}

func TestEvidenceTableDatasetServesStoredSchema(t *testing.T) {
	syncedAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	ev := domain.WarehouseDataset{
		DatabaseID:         "db-1",
		DatasetName:        "events",
		SourceEngine:       "postgres",
		WatermarkColumn:    "created_at",
		WatermarkType:      "date",
		LastWatermarkValue: "2026-08-29",
		LastSyncMode:       "incremental",
		Columns: []domain.WarehouseColumn{
			{Name: "id", Type: "BIGINT"},
			{Name: "created_at", Type: "DATE", Nullable: true},
		},
		RowCount:       42,
		BytesRaw:       1000,
		BytesParquet:   250,
		ParquetPaths:   []string{"/lake/db=db-1/tbl=events/dt=2026-08-30/part-1.parquet", "/lake/db=db-1/tbl=events/dt=2026-08-29/part-1.parquet"},
		SchemaVerified: true,
		LastSyncAt:     syncedAt,
		LastSyncStatus: "succeeded",
	}
	got := evidenceTableDataset(warehouse.TableDataset{Name: "events", SyncStatus: "never_synced"}, ev)
	if got.RowCount != 42 || got.UncompressedBytes != 1000 || got.ParquetSizeBytes != 250 {
		t.Fatalf("counts: %+v", got)
	}
	if got.CompressionRatio != 4 {
		t.Fatalf("ratio=%v want 4", got.CompressionRatio)
	}
	if got.PartitionCount != 2 || got.ParquetLocation != ev.ParquetPaths[0] {
		t.Fatalf("partitions: %+v", got)
	}
	if got.SyncStatus != "synced" || !got.LastSyncedAt.Equal(syncedAt) {
		t.Fatalf("sync state: %+v", got)
	}
	if len(got.Columns) != 2 || got.Columns[0].Name != "id" || got.Columns[1].Type != "DATE" {
		t.Fatalf("columns: %+v", got.Columns)
	}
	if got.WatermarkColumn != "created_at" || got.LastWatermarkValue != "2026-08-29" || got.LastSyncMode != "incremental" || !got.SchemaVerified {
		t.Fatalf("watermark/provenance: %+v", got)
	}

	// Evidence without a sync timestamp must not invent a sync time.
	stale := evidenceTableDataset(warehouse.TableDataset{Name: "events", SyncStatus: "never_synced"}, domain.WarehouseDataset{DatasetName: "events", RowCount: 7})
	if stale.SyncStatus != "never_synced" || !stale.LastSyncedAt.IsZero() {
		t.Fatalf("stale evidence invented sync time: %+v", stale)
	}
	if stale.RowCount != 7 {
		t.Fatalf("stale row count: %+v", stale)
	}
}

func TestWarehouseCatalogServesEvidence(t *testing.T) {
	ctx := context.Background()
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := memory.New()
	svc.SetWarehouseEvidence(store)

	syncedAt := time.Now().UTC().Truncate(time.Second)
	ev := domain.WarehouseDataset{
		ID:                 domain.WarehouseDatasetKey("db-orphan", "events"),
		DatabaseID:         "db-orphan",
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
		ParquetPaths:   []string{"/lake/db=db-orphan/tbl=events/dt=2026-08-30/part-1.parquet"},
		SchemaVerified: true,
		LastSyncAt:     syncedAt,
		LastSyncStatus: "succeeded",
	}
	if err := store.UpsertWarehouseDataset(ctx, ev); err != nil {
		t.Fatalf("upsert evidence: %v", err)
	}
	// A database still in the inventory whose live schema no longer (or not
	// yet) reports the table: evidence keeps the dataset visible.
	inventory := domain.WarehouseDataset{
		ID:           domain.WarehouseDatasetKey("db-inv", "audit_log"),
		DatabaseID:   "db-inv",
		DatabaseName: "Inventory DB",
		DatasetName:  "audit_log",
		Columns: []domain.WarehouseColumn{
			{Name: "id", Type: "INTEGER"},
		},
		RowCount:       5,
		BytesRaw:       100,
		BytesParquet:   50,
		SchemaVerified: false,
		LastSyncAt:     syncedAt,
		LastSyncStatus: "succeeded_unverified",
	}
	if err := store.UpsertWarehouseDataset(ctx, inventory); err != nil {
		t.Fatalf("upsert inventory evidence: %v", err)
	}
	svc.mu.Lock()
	svc.customDatabases["db-inv"] = domain.DatabaseResource{ID: "db-inv", Name: "Inventory DB", Engine: "postgres"}
	svc.mu.Unlock()

	catalog := svc.GetWarehouseCatalog(ctx)
	if catalog.TotalTables != 2 || catalog.TotalRows != 47 {
		t.Fatalf("catalog totals: tables=%d rows=%d", catalog.TotalTables, catalog.TotalRows)
	}

	find := func(dbID, table string) *warehouse.TableDataset {
		for i := range catalog.Databases {
			db := &catalog.Databases[i]
			if db.ID != dbID {
				continue
			}
			for j := range db.Tables {
				if db.Tables[j].Name == table {
					return &db.Tables[j]
				}
			}
		}
		return nil
	}

	orphan := find("db-orphan", "events")
	if orphan == nil {
		t.Fatal("evidence-only database missing from catalog")
	}
	if orphan.RowCount != 42 || orphan.ParquetSizeBytes != 250 || orphan.UncompressedBytes != 1000 {
		t.Fatalf("orphan counts: %+v", orphan)
	}
	if !orphan.SchemaVerified || orphan.LastSyncMode != "incremental" || orphan.WatermarkColumn != "created_at" || orphan.LastWatermarkValue != "2026-08-30" {
		t.Fatalf("orphan watermark/provenance: %+v", orphan)
	}
	if orphan.SyncStatus != "synced" || !orphan.LastSyncedAt.Equal(syncedAt) {
		t.Fatalf("orphan sync state: %+v", orphan)
	}
	if len(orphan.Columns) != 2 || orphan.Columns[0].Type != "BIGINT" {
		t.Fatalf("orphan columns invented or wrong: %+v", orphan.Columns)
	}
	if orphan.CompressionRatio != 4 {
		t.Fatalf("orphan ratio=%v want 4", orphan.CompressionRatio)
	}

	inv := find("db-inv", "audit_log")
	if inv == nil {
		t.Fatal("inventory database lost its evidence-only table")
	}
	if inv.RowCount != 5 || inv.SyncStatus != "synced" {
		t.Fatalf("inventory evidence row: %+v", inv)
	}
	if inv.SchemaVerified {
		t.Fatal("unverified evidence must not be reported as verified")
	}
	if len(inv.Columns) != 1 || inv.Columns[0].Name != "id" {
		t.Fatalf("inventory columns: %+v", inv.Columns)
	}
}
