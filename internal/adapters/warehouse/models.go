package warehouse

import (
	"time"
)

// QueryRequest defines an analytical SQL query execution request.
type QueryRequest struct {
	Query     string `json:"query"`
	Engine    string `json:"engine,omitempty"` // "duckdb", "clickhouse", "auto"
	Limit     int    `json:"limit,omitempty"`
	Database  string `json:"database,omitempty"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

// ColumnMeta describes a column in the query result.
type ColumnMeta struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// QueryResult holds the structured tabular output of an analytical query.
type QueryResult struct {
	Columns      []ColumnMeta     `json:"columns"`
	Rows         [][]any          `json:"rows"`
	RowCount     int              `json:"row_count"`
	ExecutionMs  int64            `json:"execution_ms"`
	BytesScanned int64            `json:"bytes_scanned"`
	Engine       string           `json:"engine"`
	ExecutedAt   time.Time        `json:"executed_at"`
	Cached       bool             `json:"cached"`
	Error        string           `json:"error,omitempty"`
}

// TableDataset represents a columnar Parquet dataset in the lakehouse.
type TableDataset struct {
	Name                string       `json:"name"`
	Database            string       `json:"database"`
	EngineSource        string       `json:"engine_source"`
	Columns             []ColumnMeta `json:"columns"`
	RowCount            int64        `json:"row_count"`
	ParquetSizeBytes    int64        `json:"parquet_size_bytes"`
	UncompressedBytes   int64        `json:"uncompressed_bytes"`
	CompressionRatio    float64      `json:"compression_ratio"`
	ParquetLocation     string       `json:"parquet_location"`
	PartitionKey        string       `json:"partition_key,omitempty"`
	PartitionCount      int          `json:"partition_count"`
	LastSyncedAt        time.Time    `json:"last_synced_at"`
	SyncStatus          string       `json:"sync_status"` // "synced", "syncing", "pending"
}

// DatabaseDataset represents a database namespace in the data warehouse.
type DatabaseDataset struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	EngineSource        string         `json:"engine_source"`
	Tables              []TableDataset `json:"tables"`
	TotalRows           int64          `json:"total_rows"`
	TotalParquetBytes   int64          `json:"total_parquet_bytes"`
	TotalRawBytes       int64          `json:"total_raw_bytes"`
	CompressionRatio    float64        `json:"compression_ratio"`
	LastSyncAt          time.Time      `json:"last_sync_at"`
}

// WarehouseCatalog describes the entire analytical lakehouse storage.
type WarehouseCatalog struct {
	Databases              []DatabaseDataset `json:"databases"`
	TotalDatabases         int               `json:"total_databases"`
	TotalTables            int               `json:"total_tables"`
	TotalRows              int64             `json:"total_rows"`
	TotalParquetBytes      int64             `json:"total_parquet_bytes"`
	TotalRawBytes          int64             `json:"total_raw_bytes"`
	OverallCompressionRatio float64           `json:"overall_compression_ratio"`
	StorageEngine          string            `json:"storage_engine"` // "Cloudflare R2 + Embedded DuckDB"
	GeneratedAt            time.Time         `json:"generated_at"`
}

// WarehouseConnector represents an external OLAP sync target (e.g. ClickHouse, Postgres-OLAP).
type WarehouseConnector struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Type             string    `json:"type"` // "duckdb_embedded", "clickhouse", "postgres_olap", "s3_parquet"
	Endpoint         string    `json:"endpoint"`
	DatabaseName     string    `json:"database_name"`
	Status           string    `json:"status"` // "connected", "degraded", "disabled"
	LatencyMs        int64     `json:"latency_ms"`
	AutoSync         bool      `json:"auto_sync"`
	SyncIntervalMins int       `json:"sync_interval_mins"`
	LastSyncAt       time.Time `json:"last_sync_at"`
	CreatedAt        time.Time `json:"created_at"`
}
