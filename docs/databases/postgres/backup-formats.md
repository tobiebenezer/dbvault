# PostgreSQL Backup Formats

Supported Phase 4 formats:

- `postgres-custom`: default, streamed from `pg_dump --format=custom --file=-`.
- `postgres-plain-sql`: streamed SQL output for portability.
- `postgres-directory`: specified at the API level, with full production directory-member processing left for the integration environment.
- `postgres-globals-sql`: optional `pg_dumpall --globals-only` artifact.

Ownership and privilege restore are disabled by default for portability.
