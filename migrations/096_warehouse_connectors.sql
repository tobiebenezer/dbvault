CREATE TABLE IF NOT EXISTS warehouse_connectors (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    kind       TEXT NOT NULL,               -- clickhouse | postgres | mysql
    endpoint   TEXT NOT NULL,               -- host:port or URL (kind-dependent)
    database   TEXT NOT NULL DEFAULT '',
    username   TEXT NOT NULL DEFAULT '',
    secret_ref TEXT NOT NULL DEFAULT '',    -- SecretReference JSON (env/file/provider pointer); never plaintext password
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
