# Integration drills

Real end-to-end recovery drills for the production binary (`CGO_ENABLED=1 go
build ./cmd/dbvault`, same as `make build`). Each drill creates real
infrastructure, takes a real snapshot, destroys the original data, restores,
and proves the result matches. A drill that cannot fail is not a drill: every
assertion here has a failure mode the harness actively checks.

## Drills

| Script | ID | Proves |
|---|---|---|
| `filesystem-roundtrip.sh` | IT-E1 | SQLite -> filesystem repository -> corrupt original -> verified restore. Exercises chunking, zstd, AES-GCM via real key material, ed25519-signed manifests, download + decrypt + verify. |
| `mysql-roundtrip.sh` | IT-M1 | MySQL 8.4 (Docker) -> host `mysqldump` logical snapshot -> DROP all tables -> restore SQL into a second clean MySQL container -> extended table checksum comparison. Includes a negative leg: a wrong password must fail the backup loudly at dump time. |
| `minio-roundtrip.sh` | IT-E2 | SQLite -> live MinIO S3 endpoint (path-style, file-referenced credentials) -> corrupt original -> verified restore from object storage. |

## Requirements

- Go toolchain (production build needs CGO for the SQLite catalogue driver)
- Docker with `mysql:8.4`, `minio/minio`, and `minio/mc` images available
- Host `mysqldump`/`mysql` client 8.x and `sqlite3` CLI

## Running

```bash
./integration/drills/filesystem-roundtrip.sh
./integration/drills/mysql-roundtrip.sh     # ports default 33306/33307, override ITM1_SRC_PORT/ITM1_DST_PORT
./integration/drills/minio-roundtrip.sh     # port default 39000, override ITE2_MINIO_PORT
```

Each run prints `[drill]` progress lines and finishes with `PASS: <id>` or
`FAIL: <reason>`. Workspaces are kept under `/tmp/opencode/dbvault-drills/run/`
for post-mortem; containers are always removed on exit.

## Verification model

SQLite drills assert logical identity: `.dump` fingerprint (sha256), plus
`PRAGMA integrity_check`, row counts, and an injected canary row. Physical file
bytes can legitimately differ after a snapshot roundtrip because SQLite header
counters and page layout are not stable across captures.

MySQL asserts row counts per table, `CHECKSUM TABLE ... EXTENDED` equality
between source and restored servers, and the canary token inside both the
restored SQL artifact and the loaded rows.
