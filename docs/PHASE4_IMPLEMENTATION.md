# Phase 4 Implementation Notes

This repository has been extended with the Phase 4 multi-database platform shape.

## Added

- Database-driver API v1 under `internal/ports/database_driver.go`.
- Multi-artifact backup-set domain models under `internal/domain/database.go`, `artifact.go`, and `restoreplan.go`.
- Hardened process-runner port and `internal/adapters/process/exec` implementation.
- PostgreSQL source adapter seam with:
  - driver descriptor
  - toolchain detection
  - logical backup planning
  - custom/plain/global artifact production through `ArtifactSink`
  - restore planning seam
- MySQL/MariaDB source adapter seam with:
  - variant descriptors
  - toolchain detection
  - logical dump planning
  - artifact production through `ArtifactSink`
  - restore planning seam
- Database-driver registry and memory artifact sink/source for tests and future pipeline integration.
- Manifest v3 data structures for multi-artifact snapshots.
- Phase 4 migrations 022-029.
- Phase 4 Docker Compose integration skeleton.
- Database docs skeleton for PostgreSQL, MySQL and MariaDB.
- CLI `engine list` and `engine inspect` commands.
- Makefile targets for PostgreSQL, MySQL, MariaDB and Phase 4 integration tests.

## What remains for a full production environment

The Phase 4 database adapters are intentionally written against the `ProcessRunner` and `ArtifactSink` ports. In a full CGO/Docker/network environment, the next work is to connect them to the production artifact pipeline so PostgreSQL/MySQL/MariaDB dump streams are chunked, compressed, encrypted, uploaded, signed and published exactly like SQLite snapshots.

The restricted build passes in this environment. Production tests need module download access for the dependencies already listed in `go.mod`.

Run in a full environment:

```bash
go mod download
go mod verify
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go test -race ./...
make test-postgres
make test-mysql
make test-mariadb
```
