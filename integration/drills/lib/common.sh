#!/usr/bin/env bash
# Shared helpers for DBVault integration drills.
# Drills always run the PRODUCTION binary variant (no build tags) so the real
# driver, key-provider, storage and manifest code paths are exercised.

set -Eeuo pipefail

DBV_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
DRILLS_LIB="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DRILL_RUN_ROOT="${DRILL_RUN_ROOT:-/tmp/opencode/dbvault-drills}"
BIN_DIR="$DRILL_RUN_ROOT/bin"
DBVAULT_BIN="$BIN_DIR/dbvault-prod"
DRILL_CONTAINERS=()

resolve_go() {
  if [ -x /tmp/opencode/go/bin/go ]; then printf '%s' /tmp/opencode/go/bin/go; else command -v go; fi
}

build_dbvault() {
  local g
  g="$(resolve_go)"
  mkdir -p "$BIN_DIR"
  echo "[drill] building production dbvault binary (CGO_ENABLED=1, matches 'make build')"
  (cd "$DBV_ROOT" && CGO_ENABLED=1 "$g" build -o "$DBVAULT_BIN" ./cmd/dbvault)
}

new_workdir() {
  local d="$DRILL_RUN_ROOT/run/$1-$(date +%s%N)"
  mkdir -p "$d"
  printf '%s' "$d"
}

register_container() { DRILL_CONTAINERS+=("$1"); }

cleanup_containers() {
  local c
  for c in ${DRILL_CONTAINERS[@]+"${DRILL_CONTAINERS[@]}"}; do
    docker rm -f "$c" >/dev/null 2>&1 || true
  done
}

wait_container_healthy() { # name timeout_seconds
  local name="$1" timeout="$2" waited=0 st
  while true; do
    st="$(docker inspect -f '{{.State.Health.Status}}' "$name" 2>/dev/null || true)"
    if [ "$st" = "healthy" ]; then return 0; fi
    waited=$((waited + 2))
    if [ "$waited" -ge "$timeout" ]; then
      echo "[drill] container $name not healthy after ${timeout}s; last logs:" >&2
      docker logs --tail 30 "$name" >&2 || true
      return 1
    fi
    sleep 2
  done
}

wait_http_ok() { # url timeout_seconds
  local url="$1" timeout="$2" waited=0
  while true; do
    if curl -fsS -o /dev/null "$url" 2>/dev/null; then return 0; fi
    waited=$((waited + 2))
    if [ "$waited" -ge "$timeout" ]; then return 1; fi
    sleep 2
  done
}

write_key_material() { # keys_dir repository_id active_key
  local g
  g="$(resolve_go)"
  mkdir -p "$1"
  chmod 700 "$1"
  (cd "$DRILLS_LIB" && "$g" run ./keygen "$1" "$2" "$3")
}

# make_sqlite_fixture <db-path> <canary-token>
# Creates a deterministic two-table SQLite database with a WAL checkpoint so a
# single file holds the complete logical content.
make_sqlite_fixture() {
  sqlite3 "$1" >/dev/null <<SQL
PRAGMA journal_mode=WAL;
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL, amount REAL NOT NULL);
INSERT INTO meta VALUES ('canary', '$2');
INSERT INTO items (name, amount)
  WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM seq WHERE i < 250)
  SELECT 'widget-' || i, i * 1.25 FROM seq;
INSERT INTO meta VALUES ('item_count', (SELECT COUNT(*) FROM items));
PRAGMA wal_checkpoint(TRUNCATE);
SQL
}

sqlite_fixture_row_count() { sqlite3 "$1" "SELECT COUNT(*) FROM items;"; }
sqlite_fixture_canary() { sqlite3 "$1" "SELECT value FROM meta WHERE key = 'canary';"; }

snapshot_id_from_backup_output() { # accepts output on stdin or as $1 string via heredoc caller
  grep -Eo '(backup committed|duplicate snapshot): \S+' | awk '{print $NF}' | tail -n 1
}

run_step() { # label cmd args...
  local label="$1"
  shift
  echo "[drill] $label"
  "$@"
}

# wait_mysql_tcp gates on a real query over TCP from the host. The official
# image healthcheck answers alive against the temporary init server too, so
# 'healthy' alone can fire before the final server restart completes.
wait_mysql_tcp() { # port user password timeout_seconds
  local port="$1" user="$2" pass="$3" timeout="${4:-240}" waited=0
  while true; do
    if mysql --protocol=TCP -h127.0.0.1 -P"$port" -u"$user" -p"$pass" -N -e 'SELECT 1' >/dev/null 2>&1; then
      return 0
    fi
    waited=$((waited + 2))
    if [ "$waited" -ge "$timeout" ]; then return 1; fi
    sleep 2
  done
}

drill_pass() {
  echo "PASS: $1"
}

drill_fail() {
  echo "FAIL: $1" >&2
  exit 1
}
