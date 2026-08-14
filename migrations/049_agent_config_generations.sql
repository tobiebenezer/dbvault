CREATE TABLE IF NOT EXISTS agent_config_generations (
    agent_id TEXT NOT NULL,
    generation INTEGER NOT NULL,
    config_digest TEXT NOT NULL,
    config_json TEXT NOT NULL,
    signed_envelope_json TEXT NOT NULL,
    accepted_at TEXT,
    created_at TEXT NOT NULL,
    PRIMARY KEY (agent_id, generation),
    FOREIGN KEY (agent_id) REFERENCES agents(id)
) STRICT;
