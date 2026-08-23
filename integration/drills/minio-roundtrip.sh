#!/usr/bin/env bash
# IT-E2: SQLite -> S3-compatible object storage (real MinIO server) ->
# destroy original -> verified restore.
# Exercises the production binary's s3 storage adapter against a live S3 API
# endpoint with path-style addressing and file-referenced credentials.
set -Eeuo pipefail

source "$(cd "$(dirname "$0")" && pwd)/lib/common.sh"

command -v sqlite3 >/dev/null || drill_fail "sqlite3 not found on PATH"
docker image inspect minio/minio:latest >/dev/null 2>&1 || drill_fail "minio/minio image not present locally"

LABEL="it-e2-minio-roundtrip"
WORKDIR="$(new_workdir "$LABEL")"
REPO_ID="drill-repo"
MINIO_PORT="${ITE2_MINIO_PORT:-39000}"
BUCKET="dbvault-integration"
ACCESS="integration-access"
SECRET="integration-secret"
CANARY="e2-canary-$(date +%s)"

build_dbvault

echo "[drill] workdir: $WORKDIR"

cleanup_containers
docker rm -f dv-ite2-minio >/dev/null 2>&1 || true
trap cleanup_containers EXIT

echo "[drill] starting MinIO on 127.0.0.1:$MINIO_PORT"
register_container dv-ite2-minio
docker run -d --name dv-ite2-minio \
  -e MINIO_ROOT_USER="$ACCESS" \
  -e MINIO_ROOT_PASSWORD="$SECRET" \
  -p "127.0.0.1:$MINIO_PORT:9000" \
  minio/minio server /data >/dev/null
wait_http_ok "http://127.0.0.1:$MINIO_PORT/minio/health/live" 90 \
  || drill_fail "MinIO did not become healthy"

echo "[drill] creating bucket $BUCKET"
(cd "$DRILLS_LIB" && "$(resolve_go)" run ./mkbucket "http://127.0.0.1:$MINIO_PORT" us-east-1 "$BUCKET" "$ACCESS" "$SECRET") \
  || drill_fail "failed to create bucket"

FIXTURE="$WORKDIR/fixture.db"
make_sqlite_fixture "$FIXTURE" "$CANARY"
EXPECTED_DUMP_SHA="$(sqlite3 "$FIXTURE" .dump | sha256sum | awk '{print $1}')"
EXPECTED_ROWS="$(sqlite_fixture_row_count "$FIXTURE")"
echo "[drill] fixture ready: rows=$EXPECTED_ROWS dump_sha256=${EXPECTED_DUMP_SHA:0:12}..."

write_key_material "$WORKDIR/keys" "$REPO_ID" master
printf '%s' "$ACCESS" > "$WORKDIR/s3-access-key"
printf '%s' "$SECRET" > "$WORKDIR/s3-secret-key"
chmod 600 "$WORKDIR/s3-access-key" "$WORKDIR/s3-secret-key"

cat > "$WORKDIR/config.yaml" <<EOF
version: 1

server:
  data_directory: $WORKDIR/data
  scratch_directory: $WORKDIR/scratch

catalogue:
  driver: sqlite
  path: $WORKDIR/catalogue.sqlite

secret_providers:
  - id: drill-files
    driver: file
    file:
      root: $WORKDIR/keys

destinations:
  - id: drill-minio
    driver: s3
    profile: minio
    s3:
      endpoint: http://127.0.0.1:$MINIO_PORT
      region: us-east-1
      bucket: $BUCKET
      tls:
        allow_insecure_http: true
      addressing:
        path_style: true
      credentials:
        access_key_id:
          file: $WORKDIR/s3-access-key
        secret_access_key:
          file: $WORKDIR/s3-secret-key

repositories:
  - id: $REPO_ID
    mode: single
    primary:
      destination: drill-minio
    encryption:
      key_provider: drill-files
      active_key: master
    signing:
      key_provider: drill-files
      key_id: signing-local
    compression:
      algorithm: zstd
      level: 6
      minimum_savings_percent: 5
    chunking:
      sqlite:
        strategy: page-aligned
        target_size: 64KiB
    retention:
      keep_last: 10
      daily: 14
      weekly: 8
      monthly: 6
      minimum_verified_snapshots: 2
      gc_sweep_interval: 6h

sources:
  - id: drill-sqlite
    engine: sqlite
    repository: $REPO_ID
    enabled: true
    sqlite:
      path: $FIXTURE
EOF

CFG="$WORKDIR/config.yaml"
run_step "config validate" "$DBVAULT_BIN" config validate --config "$CFG"
run_step "destination test (live S3 API)" "$DBVAULT_BIN" destination test --config "$CFG" --id drill-minio

BACKUP_OUT="$("$DBVAULT_BIN" backup-create --config "$CFG")"
echo "$BACKUP_OUT"
SNAP_ID="$(printf '%s\n' "$BACKUP_OUT" | snapshot_id_from_backup_output)"
[ -n "$SNAP_ID" ] || drill_fail "could not parse snapshot id from backup-create output"
echo "[drill] snapshot: $SNAP_ID"

echo "[drill] counting repository objects in the bucket via aws-sdk"
OBJECTS="$(cd "$DRILLS_LIB" && "$(resolve_go)" run ./lsbucket "http://127.0.0.1:$MINIO_PORT" "$BUCKET" "$ACCESS" "$SECRET" | wc -l)"
[ "${OBJECTS:-0}" -ge 3 ] || drill_fail "expected repository objects in MinIO bucket, found ${OBJECTS:-0}"
echo "[drill] bucket holds $OBJECTS objects"

echo "[drill] simulating data loss: corrupting original fixture"
dd if=/dev/urandom of="$FIXTURE" bs=1024 count=64 conv=notrunc status=none
if [ "$(sqlite3 "$FIXTURE" "PRAGMA integrity_check;" 2>/dev/null || true)" = "ok" ]; then
  drill_fail "corruption did not actually damage the fixture"
fi

run_step "restore-run --replace from object storage" \
  "$DBVAULT_BIN" restore-run --config "$CFG" --snapshot "$SNAP_ID" --target "$WORKDIR/restored.db" --replace

RESTORED_DUMP_SHA="$(sqlite3 "$WORKDIR/restored.db" .dump | sha256sum | awk '{print $1}')"
[ "$RESTORED_DUMP_SHA" = "$EXPECTED_DUMP_SHA" ] || drill_fail "restored dump fingerprint mismatch: expected $EXPECTED_DUMP_SHA got $RESTORED_DUMP_SHA"
[ "$(sqlite3 "$WORKDIR/restored.db" "PRAGMA integrity_check;")" = "ok" ] || drill_fail "integrity_check failed after restore"
[ "$(sqlite_fixture_row_count "$WORKDIR/restored.db")" = "$EXPECTED_ROWS" ] || drill_fail "row count mismatch after restore"
[ "$(sqlite_fixture_canary "$WORKDIR/restored.db")" = "$CANARY" ] || drill_fail "canary row missing after restore"

drill_pass "IT-E2 MinIO roundtrip (snapshot=$SNAP_ID)"
