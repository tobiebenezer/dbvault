# Phase 2B implementation status

This branch implements the Phase 2B dependency-enablement layer.

## What changed

- Production and restricted build profiles are separated with Go build tags.
- Restricted substitutes are now marked with `//go:build restricted`.
- Production catalogue uses `database/sql` with `github.com/mattn/go-sqlite3`.
- Production SQLite source adapter uses the native SQLite Online Backup API through `SQLiteConn.Backup`, `Step`, `Remaining` and `Finish`.
- Production compression uses real Zstandard from `github.com/klauspost/compress/zstd`.
- Production object storage uses AWS SDK for Go v2 with custom endpoint support for MinIO, Cloudflare R2 and Contabo.
- Bootstrap now loads persistent encryption and Ed25519 signing keys from the file key provider.
- The old configuration-derived encryption key and runtime-generated signing key are removed from the production build.
- `dbvaultd` production build uses robfig cron and Prometheus collectors.
- MinIO, R2 and Contabo integration-test entrypoints were added.
- Makefile now has restricted, production, integration and external-provider test targets.

## Local restricted verification

This environment has no network access for downloading Go modules and no CGO toolchain guarantee, so I verified the isolated restricted profile:

```bash
CGO_ENABLED=0 go test -tags=restricted ./...
```

## Production verification to run in a full environment

```bash
go mod download
go mod verify
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go test -race ./...
make test-integration
```

External provider tests:

```bash
make test-r2
make test-contabo
```

## Key generation

Before running production backup commands:

```bash
dbvault key generate --repository repo_local --key-id local-key --dir ./data/keys
```

The production bootstrap refuses to create backups with missing keys.
