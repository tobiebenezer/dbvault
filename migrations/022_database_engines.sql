CREATE TABLE IF NOT EXISTS database_engines (
    engine TEXT PRIMARY KEY,
    driver_api INTEGER NOT NULL,
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    descriptor_json TEXT NOT NULL,
    created_at TEXT NOT NULL
) STRICT;
