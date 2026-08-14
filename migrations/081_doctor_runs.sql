CREATE TABLE IF NOT EXISTS doctor_runs (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    checks_json TEXT NOT NULL,
    generated_at TEXT NOT NULL
) STRICT;
