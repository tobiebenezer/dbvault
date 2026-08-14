CREATE TABLE IF NOT EXISTS setup_state (
    id TEXT PRIMARY KEY,
    current_step TEXT NOT NULL,
    completed_steps_json TEXT NOT NULL,
    draft_config_json TEXT,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;
