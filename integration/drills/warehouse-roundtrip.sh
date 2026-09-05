#!/usr/bin/env bash
# IT-W1: MySQL -> governed warehouse sync -> Parquet lakehouse -> verified
# analytics surface.
#
# Proves the full warehouse pipeline against the production binary: a live
# MySQL container is synced through the durable job queue into partitioned
# Parquet files (schema-verified with the DuckDB CLI), the evidence-backed
# catalog serves measured facts, incremental watermark sync appends only new
# rows, the export path masks sensitive columns, the Power BI feed enforces
# scoped connection tokens, and a broken connector fails the job loudly.
set -Eeuo pipefail

source "$(cd "$(dirname "$0")" && pwd)/lib/common.sh"

LABEL="it-w1-warehouse-roundtrip"
WORKDIR="$(new_workdir "$LABEL")"
SRC_PORT="${ITW1_SRC_PORT:-33308}"
SRV_PORT="${ITW1_SRV_PORT:-38081}"
DB_NAME="warehouse_drill"
ROOT_PASS="drill-root-pass-123"
CANARY_EMAIL="canary-$(date +%s)@drill.test"
CANARY2_EMAIL="canary2-$(date +%s)@drill.test"
BASE="http://127.0.0.1:${SRV_PORT}"
DUCKDB_DIR="$DRILL_RUN_ROOT/bin"
DUCKDB_BIN=""

cleanup() {
  if [ -n "${SERVER_PID:-}" ]; then kill "$SERVER_PID" >/dev/null 2>&1 || true; fi
  cleanup_containers
}
trap cleanup EXIT

command -v mysql >/dev/null 2>&1 || drill_fail "mysql client not present on host"
docker image inspect mysql:8.4 >/dev/null 2>&1 || drill_fail "mysql:8.4 image not present locally"

# ensure_duckdb: sync, query and export all fail closed without the DuckDB
# CLI, so the drill needs it. Prefer PATH, then the shared drill cache, then
# download the official CLI once into the cache.
ensure_duckdb() {
  if command -v duckdb >/dev/null 2>&1; then DUCKDB_BIN="$(command -v duckdb)"; return; fi
  if [ -x "$DUCKDB_DIR/duckdb" ]; then DUCKDB_BIN="$DUCKDB_DIR/duckdb"; return; fi
  local arch asset
  arch="$(uname -m)"
  case "$arch" in
    x86_64) asset="duckdb_cli-linux-amd64.zip" ;;
    aarch64 | arm64) asset="duckdb_cli-linux-arm64.zip" ;;
    *) drill_fail "no duckdb CLI and unsupported architecture: $arch" ;;
  esac
  echo "[drill] downloading DuckDB CLI ($asset) into $DUCKDB_DIR"
  mkdir -p "$DUCKDB_DIR"
  curl -fsSL "https://github.com/duckdb/duckdb/releases/download/v1.1.3/$asset" -o /tmp/opencode/duckdb-cli.zip
  (cd "$DUCKDB_DIR" && {
    unzip -o /tmp/opencode/duckdb-cli.zip >/dev/null 2>&1 ||
      python3 -c "import zipfile; zipfile.ZipFile('/tmp/opencode/duckdb-cli.zip').extractall('.')"
  })
  chmod +x "$DUCKDB_DIR/duckdb"
  DUCKDB_BIN="$DUCKDB_DIR/duckdb"
}
ensure_duckdb

build_dbvault
echo "[drill] workdir: $WORKDIR"

# --- 1. Live MySQL source ---------------------------------------------------
cleanup_containers
# Unique container name: this host runs other workloads that may remove or
# collide with fixed names, and a timed-out earlier run can leave the name in
# "removal in progress".
SRC_NAME="dv-itw1-src-$$"
docker rm -f "$SRC_NAME" >/dev/null 2>&1 || true
for i in $(seq 1 30); do
  docker inspect "$SRC_NAME" >/dev/null 2>&1 || break
  sleep 1
done
if docker inspect "$SRC_NAME" >/dev/null 2>&1; then
  echo "$SRC_NAME could not be removed; aborting" >&2
  exit 1
fi
register_container "$SRC_NAME"
if ! docker run -d --name "$SRC_NAME" \
  -e MYSQL_ROOT_PASSWORD="$ROOT_PASS" \
  -e MYSQL_DATABASE="$DB_NAME" \
  -p "127.0.0.1:${SRC_PORT}:3306" \
  mysql:8.4; then
  drill_fail "docker run $SRC_NAME failed (name conflict or daemon error)"
fi
# The mysql:8.4 image ships no container healthcheck on some hosts, so gate
# on a real TCP query instead of .State.Health.
wait_mysql_tcp "$SRC_PORT" root "$ROOT_PASS" 360
docker exec -i "$SRC_NAME" mysql -uroot -p"$ROOT_PASS" "$DB_NAME" <<SQL
CREATE TABLE customers (
  id INT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  email VARCHAR(200) NOT NULL,
  password VARCHAR(100) NOT NULL,
  created_at DATE NOT NULL
);
INSERT INTO customers (name, email, password, created_at) VALUES
  ('Alice', 'alice@drill.test', 'hunter2', CURDATE() - INTERVAL 2 DAY),
  ('Bob', 'bob@drill.test', 's3cret', CURDATE() - INTERVAL 1 DAY),
  ('Canary', '${CANARY_EMAIL}', 'pw-canary', CURDATE() - INTERVAL 1 DAY);
SQL
echo "[drill] mysql source seeded with 3 rows (canary: $CANARY_EMAIL)"

# --- 2. Server with durable job runtime -------------------------------------
# The job runtime (durable queue + worker pool) is only wired outside demo
# mode with at least one schedule spec in the config, so the config carries a
# low-frequency warehouse_sync spec. The config itself must pass strict
# validation (repository/destination/secret providers), mirroring production
# deployment shape.
write_key_material "$WORKDIR/keys" "drill-repo" master
printf '%s' "$ROOT_PASS" > "$WORKDIR/mysql-password"
chmod 600 "$WORKDIR/mysql-password"

cat > "$WORKDIR/config.yaml" <<EOF
version: 1

server:
  listen: 127.0.0.1:${SRV_PORT}
  data_directory: ${WORKDIR}/data
  scratch_directory: ${WORKDIR}/scratch

catalogue:
  driver: sqlite
  path: ${WORKDIR}/catalogue.sqlite

secret_providers:
  - id: drill-files
    driver: file
    file:
      root: ${WORKDIR}/keys

destinations:
  - id: drill-fs
    driver: filesystem
    filesystem:
      root: ${WORKDIR}/repo

repositories:
  - id: drill-repo
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
    retention:
      keep_last: 10
      daily: 14
      weekly: 8
      monthly: 6
      minimum_verified_snapshots: 2
      gc_sweep_interval: 6h

sources:
  - id: drill-mysql
    engine: mysql
    repository: drill-repo
    enabled: true
    mysql:
      host: 127.0.0.1
      port: ${SRC_PORT}
      database: ${DB_NAME}
      username: root
      password:
        file: ${WORKDIR}/mysql-password

schedules:
  - id: drill-warehouse-heartbeat
    operation: warehouse_sync
    source: drill-mysql
    every_seconds: 3600
    enabled: true
EOF

# Schema discovery on the appliance reads ambient MYSQL_* env; extraction goes
# through the stored connector instead.
MYSQL_HOST=127.0.0.1 MYSQL_PORT="$SRC_PORT" MYSQL_USER=root MYSQL_PWD="$ROOT_PASS" \
  PATH="$DUCKDB_DIR:$PATH" "$DBVAULT_BIN" server \
  --config "$WORKDIR/config.yaml" \
  --listen "127.0.0.1:${SRV_PORT}" \
  --data-dir "$WORKDIR/data" \
  > "$WORKDIR/server.log" 2>&1 &
SERVER_PID=$!

for i in $(seq 1 100); do
  if curl -fsS "$BASE/health" >/dev/null 2>&1; then break; fi
  sleep 0.2
  if [ "$i" -eq 100 ]; then
    echo "server did not become healthy" >&2
    cat "$WORKDIR/server.log" >&2 || true
    exit 1
  fi
done

SETUP_TOKEN="$(grep -o 'setup token: [A-F0-9-]*' "$WORKDIR/server.log" | awk '{print $3}')"
[ -n "$SETUP_TOKEN" ] || drill_fail "setup token not found in server log"
COOKIES="$WORKDIR/cookies.txt"
curl -fsS -c "$COOKIES" -H "Content-Type: application/json" \
  -d "{\"setup_token\":\"${SETUP_TOKEN}\",\"username\":\"drill-admin\",\"password\":\"drill-password-123\"}" \
  "$BASE/api/v1/auth/bootstrap" | grep -q "administrator-created"

api() { curl -fsS -b "$COOKIES" "$@"; }
api_json_field() { # json field
  grep -Eo "\"$2\"[: ]*\"?[A-Za-z0-9_-]+\"?" <<<"$1" | head -1 | grep -Eo '[A-Za-z0-9_-]+' | tail -1
}
await_job() { # job_id timeout_seconds
  local id="$1" timeout="$2" waited=0 body
  while true; do
    body="$(api "$BASE/api/v1/jobs/$id")"
    if grep -q '"status":"completed"' <<<"$body"; then printf '%s' "$body"; return 0; fi
    if grep -q '"status":"failed"' <<<"$body"; then
      echo "job $id failed: $body" >&2
      return 1
    fi
    waited=$((waited + 2))
    if [ "$waited" -ge "$timeout" ]; then
      echo "job $id did not settle within ${timeout}s: $body" >&2
      return 1
    fi
    sleep 2
  done
}

# --- 3. Register database + connector ---------------------------------------
DB_RESP="$(api -X POST -H "Content-Type: application/json" \
  -d "{\"name\":\"${DB_NAME}\",\"engine\":\"mysql\"}" \
  "$BASE/api/v1/databases")"
DB_ID="$(api_json_field "$DB_RESP" id)"
[ -n "$DB_ID" ] || drill_fail "database registration failed: $DB_RESP"
echo "[drill] registered database: $DB_ID"

CONN_RESP="$(api -X POST -H "Content-Type: application/json" \
  -d "{\"name\":\"drill-mysql-src\",\"kind\":\"mysql\",\"endpoint\":\"127.0.0.1:${SRC_PORT}\",\"database\":\"${DB_NAME}\",\"username\":\"root\",\"password\":\"${ROOT_PASS}\"}" \
  "$BASE/api/v1/warehouse/connectors")"
CONN_ID="$(api_json_field "$CONN_RESP" id)"
[ -n "$CONN_ID" ] || drill_fail "connector creation failed: $CONN_RESP"
if grep -q "$ROOT_PASS" <<<"$CONN_RESP"; then drill_fail "connector response leaked the plaintext password"; fi

TEST_RESP="$(api -X POST "$BASE/api/v1/warehouse/connectors/test?id=${CONN_ID}")"
grep -q '"status":"connected"' <<<"$TEST_RESP" || drill_fail "connector test failed: $TEST_RESP"
echo "[drill] connector $CONN_ID verified against the live MySQL source"

# --- 4. Full sync through the durable queue ---------------------------------
SYNC_RESP="$(api -X POST -H "Content-Type: application/json" \
  -d "{\"database_id\":\"${DB_ID}\",\"connector_id\":\"${CONN_ID}\"}" \
  "$BASE/api/v1/warehouse/sync")"
JOB_ID="$(api_json_field "$SYNC_RESP" id)"
grep -q '"job_type":"warehouse_sync"' <<<"$SYNC_RESP" || drill_fail "sync did not enqueue a warehouse_sync job: $SYNC_RESP"
await_job "$JOB_ID" 180 >/dev/null
echo "[drill] full sync job $JOB_ID completed"

LAKE_DIR="$WORKDIR/data/product-experience/lakehouse/db=${DB_ID}/tbl=customers"
PARQUET="$LAKE_DIR/data.parquet"
[ -s "$PARQUET" ] || drill_fail "parquet artifact missing: $PARQUET"
[ "$(head -c 4 "$PARQUET")" = "PAR1" ] || drill_fail "artifact is not a Parquet file"
ROWS="$("$DUCKDB_BIN" -noheader -list -c "SELECT COUNT(*) FROM read_parquet('$PARQUET')")"
[ "$ROWS" = "3" ] || drill_fail "parquet row count=$ROWS want=3"
"$DUCKDB_BIN" -noheader -list -c "SELECT COUNT(*) FROM read_parquet('$PARQUET') WHERE email = '${CANARY_EMAIL}'" | grep -qx "1" ||
  drill_fail "canary row missing from parquet artifact"

CATALOG="$(api "$BASE/api/v1/warehouse/catalog")"
grep -q '"schema_verified":true' <<<"$CATALOG" || drill_fail "catalog does not carry verified schema evidence"
grep -q '"row_count":3' <<<"$CATALOG" || drill_fail "catalog row_count evidence missing: expected 3"

# --- 5. Governed query surface ----------------------------------------------
QUERY_RESP="$(api -X POST -H "Content-Type: application/json" \
  -d "{\"database\":\"${DB_ID}\",\"connector_id\":\"${CONN_ID}\",\"query\":\"SELECT COUNT(*) AS n FROM customers\"}" \
  "$BASE/api/v1/warehouse/query")"
grep -q '"row_count":1' <<<"$QUERY_RESP" || drill_fail "live query failed: $QUERY_RESP"
grep -qE '"rows":\[\[("?3"?)\]\]' <<<"$QUERY_RESP" || drill_fail "live query returned unexpected rows: $QUERY_RESP"

GUARD_CODE="$(curl -sS -b "$COOKIES" -X POST -H "Content-Type: application/json" \
  -d '{"database":"'"$DB_ID"'","query":"DROP TABLE customers"}' \
  -o /dev/null -w '%{http_code}' "$BASE/api/v1/warehouse/query")"
[ "$GUARD_CODE" = "400" ] || drill_fail "write query was not rejected (status=$GUARD_CODE)"

# --- 6. Incremental watermark sync ------------------------------------------
docker exec -i "$SRC_NAME" mysql -uroot -p"$ROOT_PASS" "$DB_NAME" <<SQL
INSERT INTO customers (name, email, password, created_at)
  VALUES ('Newcomer', '${CANARY2_EMAIL}', 'pw2', CURDATE());
SQL
INC_RESP="$(api -X POST -H "Content-Type: application/json" \
  -d "{\"database_id\":\"${DB_ID}\",\"incremental\":true,\"watermark_column\":\"created_at\",\"connector_id\":\"${CONN_ID}\"}" \
  "$BASE/api/v1/warehouse/sync")"
INC_JOB="$(api_json_field "$INC_RESP" id)"
await_job "$INC_JOB" 180 >/dev/null

TODAY="$(date -u +%F)"
DELTA="$(find "$LAKE_DIR" -path "*dt=${TODAY}*" -name 'part-*.parquet' | head -1)"
if [ -z "$DELTA" ] || [ ! -s "$DELTA" ]; then drill_fail "incremental dt=$TODAY partition missing"; fi
DELTA_ROWS="$("$DUCKDB_BIN" -noheader -list -c "SELECT COUNT(*) FROM read_parquet('$DELTA')")"
[ "$DELTA_ROWS" = "1" ] || drill_fail "incremental partition rows=$DELTA_ROWS want=1 (watermark must append only new rows)"
"$DUCKDB_BIN" -noheader -list -c "SELECT COUNT(*) FROM read_parquet('$DELTA') WHERE email = '${CANARY2_EMAIL}'" | grep -qx "1" ||
  drill_fail "incremental partition does not contain the new canary row"

CATALOG2="$(api "$BASE/api/v1/warehouse/catalog")"
grep -q '"row_count":4' <<<"$CATALOG2" || drill_fail "catalog evidence did not grow after incremental sync"
echo "[drill] incremental watermark sync appended exactly one new row into dt=$TODAY"

# --- 7. Governed export masks sensitive columns -----------------------------
EXPORT="$WORKDIR/export.csv"
CODE="$(curl -sS -b "$COOKIES" -X POST -H "Content-Type: application/json" \
  -d '{"database":"'"$DB_ID"'","connector_id":"'"$CONN_ID"'","query":"SELECT name, email, password FROM customers","format":"csv"}' \
  -o "$EXPORT" -w '%{http_code}' "$BASE/api/v1/warehouse/export")"
[ "$CODE" = "200" ] || drill_fail "export failed (status=$CODE)"
grep -q "@anonymized.internal" "$EXPORT" || drill_fail "export did not mask emails"
grep -q "\[REDACTED\]" "$EXPORT" || drill_fail "export did not redact passwords"
if grep -q "$CANARY_EMAIL" "$EXPORT"; then drill_fail "export leaked the canary email"; fi
if grep -q "hunter2" "$EXPORT"; then drill_fail "export leaked a password"; fi
grep -q "Alice" "$EXPORT" || drill_fail "export dropped non-sensitive columns"

# --- 8. Power BI feed behind a scoped connection token -----------------------
BI_CODE="$(curl -sS -b "$COOKIES" -o /dev/null -w '%{http_code}' \
  "$BASE/api/v1/bi/powerbi/feed?database=${DB_ID}&table=customers")"
[ "$BI_CODE" = "400" ] || drill_fail "BI feed without credentials must fail closed (status=$BI_CODE)"

BI_RESP="$(api -X POST -H "Content-Type: application/json" \
  -d "{\"name\":\"drill-bi\",\"provider\":\"powerbi\",\"datasets\":[{\"database\":\"${DB_ID}\",\"table\":\"customers\"}],\"expires_in_days\":30}" \
  "$BASE/api/v1/bi/connections")"
BI_TOKEN="$(api_json_field "$BI_RESP" token)"
BI_CONN="$(grep -Eo '"connection":\{"id":"[^"]*"' <<<"$BI_RESP" | head -1 | sed -e 's/.*"id":"//' -e 's/"$//')"
if [ -z "$BI_TOKEN" ] || [ -z "$BI_CONN" ]; then drill_fail "BI connection creation failed: $BI_RESP"; fi

FEED="$WORKDIR/feed.csv"
CODE="$(curl -sS -b "$COOKIES" -H "Authorization: Bearer $BI_TOKEN" \
  -o "$FEED" -w '%{http_code}' \
  "$BASE/api/v1/bi/powerbi/feed?database=${DB_ID}&table=customers&connection_id=${BI_CONN}")"
[ "$CODE" = "200" ] || drill_fail "scoped BI feed failed (status=$CODE)"
grep -q "@anonymized.internal" "$FEED" || drill_fail "BI feed did not mask emails"
if grep -q "$CANARY_EMAIL" "$FEED"; then drill_fail "BI feed leaked the canary email"; fi

# --- 9. Audit trail ----------------------------------------------------------
AUDIT="$(api "$BASE/api/v1/audit/events?limit=300")"
grep -q '"event_type":"warehouse.sync_completed"' <<<"$AUDIT" || drill_fail "sync completion not audited"
grep -q '"event_type":"warehouse.export"' <<<"$AUDIT" || drill_fail "export not audited"

# --- 10. Negative leg: unknown connector fails the job loudly -----------------
NEG_RESP="$(api -X POST -H "Content-Type: application/json" \
  -d "{\"database_id\":\"${DB_ID}\",\"connector_id\":\"whconn-does-not-exist\"}" \
  "$BASE/api/v1/warehouse/sync")"
NEG_JOB="$(api_json_field "$NEG_RESP" id)"
NEG_BODY="$(await_job "$NEG_JOB" 120 2>/dev/null || true)"
if [ -n "$NEG_BODY" ]; then drill_fail "sync with unknown connector must fail, not complete"; fi
NEG_JOB_BODY="$(api "$BASE/api/v1/jobs/$NEG_JOB")"
grep -q '"status":"failed"' <<<"$NEG_JOB_BODY" ||
  drill_fail "unknown-connector sync did not land as a failed job"
grep -q "connector" <<<"$NEG_JOB_BODY" ||
  drill_fail "failed job should name the connector problem"
echo "[drill] unknown connector failed the job loudly, as required"

drill_pass "IT-W1: MySQL -> durable warehouse sync -> verified Parquet lakehouse -> governed query/export/BI feed"
