CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    organisation_id TEXT NOT NULL,
    project_id TEXT,
    environment_id TEXT,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    version TEXT NOT NULL,
    protocol_version INTEGER NOT NULL,
    last_seen_at TEXT,
    config_generation INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (organisation_id) REFERENCES organisations(id),
    FOREIGN KEY (project_id) REFERENCES projects(id),
    FOREIGN KEY (environment_id) REFERENCES environments(id)
) STRICT;
CREATE INDEX IF NOT EXISTS idx_agents_tenant ON agents(organisation_id, project_id, environment_id);
