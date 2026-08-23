#!/usr/bin/env bash
# IT-E1: SQLite -> filesystem repository -> destroy original -> verified restore.
# Proves the engine-agnostic restore path end to end against the production
# binary: chunked upload, encryption/signing via real key material, manifest,
# download + decrypt + verify on the way back.
set -Eeuo pipefail

source "$(cd "$(dirname "$0")" && pwd)/lib/common.sh"

LABEL="it-e1-fs-roundtrip"
WORKDIR="$(new_workdir "$LABEL")"
REPO_ID="drill-repo"
CANARY="e1-canary-$(date +%s)"

build_dbvault

echo "[drill] workdir: $WORKDIR"
FIXTURE="$WORKDIR/fixture.db"
make_sqlite_fixture "$FIXTURE" "$CANARY"
# Physical bytes can legitimately differ after a snapshot roundtrip (SQLite
# header counters/page layout), so drills assert LOGICAL identity via a dump
# fingerprint plus integrity/row/canary checks.
EXPECTED_DUMP_SHA="$(sqlite3 "$FIXTURE" .dump | sha256sum | awk '{print $1}')"
EXPECTED_ROWS="$(sqlite_fixture_row_count "$FIXTURE")"
echo "[drill] fixture ready: rows=$EXPECTED_ROWS dump_sha256=${EXPECTED_DUMP_SHA:0:12}..."

write_key_material "$WORKDIR/keys" "$REPO_ID" master

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
  - id: drill-fs
    driver: filesystem
    filesystem:
      root: $WORKDIR/repo

repositories:
  - id: $REPO_ID
    mode: single
    primary:
      destination: drill-fs
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

BACKUP_OUT="$("$DBVAULT_BIN" backup-create --config "$CFG")"
echo "$BACKUP_OUT"
SNAP_ID="$(printf '%s\n' "$BACKUP_OUT" | snapshot_id_from_backup_output)"
[ -n "$SNAP_ID" ] || drill_fail "could not parse snapshot id from backup-create output"
echo "[drill] snapshot: $SNAP_ID"

"$DBVAULT_BIN" backup-list --config "$CFG"

echo "[drill] simulating data loss: corrupting original fixture"
dd if=/dev/urandom of="$FIXTURE" bs=1024 count=64 conv=notrunc status=none
if [ "$(sqlite3 "$FIXTURE" "PRAGMA integrity_check;" 2>/dev/null || true)" = "ok" ]; then
  drill_fail "corruption did not actually damage the fixture"
fi

run_step "restore-plan" "$DBVAULT_BIN" restore-plan --config "$CFG" --snapshot "$SNAP_ID" --target "$FIXTURE"
run_step "restore-run --replace" "$DBVAULT_BIN" restore-run --config "$CFG" --snapshot "$SNAP_ID" --target "$FIXTURE" --replace

RESTORED_DUMP_SHA="$(sqlite3 "$FIXTURE" .dump | sha256sum | awk '{print $1}')"
[ "$RESTORED_DUMP_SHA" = "$EXPECTED_DUMP_SHA" ] || drill_fail "restored dump fingerprint mismatch: expected $EXPECTED_DUMP_SHA got $RESTORED_DUMP_SHA"
[ "$(sqlite3 "$FIXTURE" "PRAGMA integrity_check;")" = "ok" ] || drill_fail "integrity_check failed after restore"
[ "$(sqlite_fixture_row_count "$FIXTURE")" = "$EXPECTED_ROWS" ] || drill_fail "row count mismatch after restore"
[ "$(sqlite_fixture_canary "$FIXTURE")" = "$CANARY" ] || drill_fail "canary row missing after restore"

drill_pass "IT-E1 filesystem roundtrip (snapshot=$SNAP_ID)"
