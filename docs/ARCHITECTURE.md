# Architecture

DBVault uses hexagonal architecture. The core owns interfaces. Adapters implement those interfaces. Provider-specific logic is isolated under `internal/adapters/storage/*`.

The backup path is:

1. Source driver creates a consistent SQLite snapshot.
2. Chunker reads page-aligned chunks.
3. Digest service computes repository-keyed chunk IDs.
4. Compressor compresses each missing chunk.
5. Encryptor encrypts each compressed chunk.
6. Object store writes immutable chunk objects.
7. Manifest is signed and published.
8. Completion marker commits the snapshot.

The restore path reverses this process using only the remote manifest and required keys.
