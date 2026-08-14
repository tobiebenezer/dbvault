CREATE TABLE IF NOT EXISTS toolchains (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    engine TEXT NOT NULL,
    server_version TEXT NOT NULL,
    client_version TEXT NOT NULL,
    capabilities_json TEXT NOT NULL,
    detected_at TEXT NOT NULL
) STRICT;
