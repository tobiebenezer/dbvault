CREATE TABLE IF NOT EXISTS artifact_chunks (
    artifact_id TEXT NOT NULL,
    chunk_id TEXT NOT NULL,
    repository_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    logical_offset INTEGER NOT NULL,
    plaintext_size INTEGER NOT NULL,
    PRIMARY KEY (artifact_id, sequence)
) STRICT;
