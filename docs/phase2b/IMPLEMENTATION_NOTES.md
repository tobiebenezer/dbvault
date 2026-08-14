# Phase 2B implementation notes

This repository now has explicit production and restricted build profiles.

Production profile:

```bash
CGO_ENABLED=1 go mod download
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go build ./cmd/dbvault ./cmd/dbvaultd
```

Restricted profile, for offline environments only:

```bash
CGO_ENABLED=0 go test -tags=restricted ./...
```

Phase 2B production files are guarded with `//go:build !restricted` and use real dependencies for:

- SQLite catalogue through `github.com/mattn/go-sqlite3`
- Native SQLite Online Backup API
- Zstandard through `github.com/klauspost/compress/zstd`
- S3-compatible object storage through AWS SDK for Go v2
- Strict YAML through `go.yaml.in/yaml/v3`
- Cron scheduling through `github.com/robfig/cron/v3`
- Prometheus metrics through `github.com/prometheus/client_golang`

The restricted build keeps the old JSON catalogue, CLI SQLite snapshotter, fake zstd and S3 stub isolated behind `//go:build restricted`.
