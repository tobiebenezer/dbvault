CREATE TABLE IF NOT EXISTS warehouse_datasets (
    id                   TEXT PRIMARY KEY,
    database_id          TEXT NOT NULL,
    dataset_name         TEXT NOT NULL,
    source_engine        TEXT NOT NULL DEFAULT '',             -- mysql | postgres | clickhouse | ...
    watermark_column     TEXT NOT NULL DEFAULT '',             -- '' = full-refresh only
    watermark_type       TEXT NOT NULL DEFAULT '',             -- date | datetime | integer | text
    last_watermark_value TEXT NOT NULL DEFAULT '',             -- normalized, comparable per type
    last_sync_mode       TEXT NOT NULL DEFAULT '',             -- full | incremental
    columns_json         TEXT NOT NULL DEFAULT '[]',           -- observed schema: name/type/nullability
    row_count            INTEGER NOT NULL DEFAULT 0,           -- measured (Parquet COUNT(*), else extraction count)
    bytes_raw            INTEGER NOT NULL DEFAULT 0,           -- measured staged extraction bytes
    bytes_parquet        INTEGER NOT NULL DEFAULT 0,           -- measured written artifact bytes
    parquet_paths_json   TEXT NOT NULL DEFAULT '[]',           -- actual paths written
    schema_verified      INTEGER NOT NULL DEFAULT 0,           -- 1 = schema read back from Parquet artifact
    last_sync_at         TEXT NOT NULL DEFAULT '',
    last_sync_status     TEXT NOT NULL DEFAULT '',             -- succeeded | succeeded_unverified
    UNIQUE(database_id, dataset_name)
);
CREATE INDEX IF NOT EXISTS idx_wh_datasets_db ON warehouse_datasets(database_id);
