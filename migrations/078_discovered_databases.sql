CREATE TABLE IF NOT EXISTS discovered_databases (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    engine TEXT NOT NULL,
    host TEXT,
    port INTEGER,
    path TEXT,
    service_name TEXT,
    container_name TEXT,
    confidence REAL NOT NULL,
    evidence_json TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL
) STRICT;
