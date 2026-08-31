package domain

import "time"

// WarehouseColumn is one column of a synchronized warehouse dataset as
// observed in the written Parquet artifact (verified) or, when verification
// was impossible, as reported by the extraction path (explicitly labeled
// unverified via WarehouseDataset.SchemaVerified).
type WarehouseColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// WarehouseDataset is the per-dataset warehouse sync evidence persisted in the
// catalogue: real schema, measured row counts and byte sizes, written Parquet
// paths, and the incremental watermark state. Nothing in this record is
// invented; every numeric field traces to a measurement taken during sync.
type WarehouseDataset struct {
	ID                 string            `json:"id"`
	DatabaseID         string            `json:"database_id"`
	DatabaseName       string            `json:"database_name,omitempty"`
	DatasetName        string            `json:"dataset_name"`
	SourceEngine       string            `json:"source_engine,omitempty"`
	WatermarkColumn    string            `json:"watermark_column,omitempty"`     // '' = full-refresh only
	WatermarkType      string            `json:"watermark_type,omitempty"`       // date | datetime | integer | text
	LastWatermarkValue string            `json:"last_watermark_value,omitempty"` // normalized, lexically/numerically comparable per type
	LastSyncMode       string            `json:"last_sync_mode,omitempty"`       // full | incremental
	Columns            []WarehouseColumn `json:"columns"`
	RowCount           int64             `json:"row_count"`
	BytesRaw           int64             `json:"bytes_raw"`
	BytesParquet       int64             `json:"bytes_parquet"`
	ParquetPaths       []string          `json:"parquet_paths"`
	SchemaVerified     bool              `json:"schema_verified"`
	LastSyncAt         time.Time         `json:"last_sync_at"`
	LastSyncStatus     string            `json:"last_sync_status,omitempty"` // succeeded | succeeded_unverified
}

// WarehouseDatasetKey composes the catalogue unique key for one dataset
// (UNIQUE(database_id, dataset_name) in the warehouse_datasets table).
func WarehouseDatasetKey(databaseID, datasetName string) string {
	return databaseID + ":" + datasetName
}
