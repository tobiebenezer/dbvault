# Phase 2 Implementation Notes

This repository now includes a Phase 2 foundation built around the existing hexagonal architecture.

Implemented in this dependency-free build:

- YAML-like configuration loader and validator
- Composition-root bootstrap package
- Durable catalogue adapter under `internal/adapters/catalogue/sqlite`
- Persistent lease manager
- Ed25519 manifest signer
- File key provider
- Zstandard adapter seam
- Expanded CLI commands
- `dbvaultd` daemon with health, readiness and metrics endpoints
- Verification service for manifest/signature/object existence
- Repository scanner scaffold
- Garbage-collection service scaffold
- Webhook notifier
- Phase 2 example config

Important production notes:

- The durable catalogue adapter currently persists the catalogue model to a JSON file at the configured `dbvault.sqlite` path so this zip compiles without external SQLite driver dependencies. Replace its internals with `database/sql` plus a real SQLite driver to meet the strict production catalogue requirement.
- The SQLite source adapter still uses the `sqlite3` CLI snapshot path in this dependency-free build. The package boundary is ready for a native Online Backup API implementation.
- The S3-compatible adapter remains a provider-profile and capability scaffold. Wiring AWS SDK for Go v2 should be done in `internal/adapters/storage/s3` without changing the domain/application layer.
- The `zstd` adapter exposes the Phase 2 seam but uses standard-library gzip internally because no zstd module is vendored.

Useful commands:

```bash
go test ./...
go build ./cmd/dbvault ./cmd/dbvaultd
./dbvault config validate --config configs/dbvault.phase2.example.yaml
./dbvault key generate --dir ./data/keys --repository repo_local --key-id local-key
./dbvaultd --config configs/dbvault.phase2.example.yaml
```
