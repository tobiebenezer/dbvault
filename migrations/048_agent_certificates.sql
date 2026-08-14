CREATE TABLE IF NOT EXISTS agent_certificates (
    serial TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    public_key_digest TEXT NOT NULL,
    not_before TEXT NOT NULL,
    not_after TEXT NOT NULL,
    revoked_at TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY (agent_id) REFERENCES agents(id)
) STRICT;
