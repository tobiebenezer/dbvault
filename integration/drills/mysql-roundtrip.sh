#!/usr/bin/env bash
# IT-M1: real MySQL 8.4 -> mysqldump logical snapshot -> destroy schema ->
# restore into a second MySQL container -> checksum comparison.
# Exercises the production binary's mysql/mariadb driver: DSN assembly from a
# file-referenced secret, host mysqldump invocation, dump chunking/encryption
# into the repository, and the SQL-logical restore path.
set -Eeuo pipefail

source "$(cd "$(dirname "$0")" && pwd)/lib/common.sh"

command -v mysqldump >/dev/null || drill_fail "mysqldump not found on PATH"
docker image inspect mysql:8.4 >/dev/null 2>&1 || drill_fail "mysql:8.4 image not present locally"

LABEL="it-m1-mysql-roundtrip"
WORKDIR="$(new_workdir "$LABEL")"
REPO_ID="drill-repo"
SRC_PORT="${ITM1_SRC_PORT:-33306}"
DST_PORT="${ITM1_DST_PORT:-33307}"
CANARY="m1-canary-$(date +%s)"
DB_NAME="dbvault_drill"
DB_USER="dbvault"
DB_PASS="dbvault-pass"
ROOT_PASS="root-pass"

build_dbvault

echo "[drill] workdir: $WORKDIR"

cleanup_containers
docker rm -f dv-itm1-src dv-itm1-dst >/dev/null 2>&1 || true
trap cleanup_containers EXIT

echo "[drill] starting source mysql:8.4 on 127.0.0.1:$SRC_PORT"
register_container dv-itm1-src
docker run -d --name dv-itm1-src \
  -e MYSQL_ROOT_PASSWORD="$ROOT_PASS" \
  -e MYSQL_DATABASE="$DB_NAME" \
  -e MYSQL_USER="$DB_USER" \
  -e MYSQL_PASSWORD="$DB_PASS" \
  -p "127.0.0.1:$SRC_PORT:3306" \
  --health-cmd="mysqladmin ping -h localhost" --health-interval=3s --health-retries=60 --health-start-period=180s \
  mysql:8.4 >/dev/null
wait_container_healthy dv-itm1-src 360
wait_mysql_tcp "$SRC_PORT" root "$ROOT_PASS" 240 || drill_fail "source mysql not accepting queries"; echo "[drill] source ready"

echo "[drill] seeding schema and data (canary=$CANARY)"
docker exec -i dv-itm1-src mysql -uroot -p"$ROOT_PASS" "$DB_NAME" <<SQL
CREATE TABLE customers (
  id INT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  email VARCHAR(190) NOT NULL UNIQUE
);
CREATE TABLE orders (
  id INT AUTO_INCREMENT PRIMARY KEY,
  customer_id INT NOT NULL,
  amount DECIMAL(10,2) NOT NULL,
  note VARCHAR(255),
  FOREIGN KEY (customer_id) REFERENCES customers(id)
);
INSERT INTO customers (name, email) VALUES ('Ada Lovelace', 'ada@example.test'), ('Grace Hopper', 'grace@example.test'), ('Edsger Dijkstra', 'edsger@example.test');
INSERT INTO orders (customer_id, amount, note)
  WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM seq WHERE i < 200)
  SELECT 1 + (i % 3), i * 3.5, '$CANARY' FROM seq;
SQL

EXPECTED_CUSTOMERS="$(docker exec dv-itm1-src mysql -uroot -p"$ROOT_PASS" -N -e "SELECT COUNT(*) FROM customers" "$DB_NAME" 2>/dev/null)"
EXPECTED_ORDERS="$(docker exec dv-itm1-src mysql -uroot -p"$ROOT_PASS" -N -e "SELECT COUNT(*) FROM orders" "$DB_NAME" 2>/dev/null)"
EXPECTED_CHECKSUM="$(docker exec dv-itm1-src mysql -uroot -p"$ROOT_PASS" -N -e "CHECKSUM TABLE customers, orders EXTENDED" "$DB_NAME" 2>/dev/null | awk '{printf "%s=%s,", $1, $2}' | sed 's/,$//')"
echo "[drill] seeded: customers=$EXPECTED_CUSTOMERS orders=$EXPECTED_ORDERS"

write_key_material "$WORKDIR/keys" "$REPO_ID" master
printf '%s' "$DB_PASS" > "$WORKDIR/mysql-password"
chmod 600 "$WORKDIR/mysql-password"

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
    repository: $REPO_ID
    enabled: true
    mysql:
      host: 127.0.0.1
      port: $SRC_PORT
      database: $DB_NAME
      username: $DB_USER
      password:
        file: $WORKDIR/mysql-password
EOF

CFG="$WORKDIR/config.yaml"
run_step "config validate" "$DBVAULT_BIN" config validate --config "$CFG"
run_step "source test (reachability + secret resolution)" "$DBVAULT_BIN" source test --config "$CFG" --id drill-mysql

echo "[drill] negative leg: wrong password must fail loudly at dump time"
printf '%s' "definitely-wrong-$CANARY" > "$WORKDIR/mysql-password-bad"
chmod 600 "$WORKDIR/mysql-password-bad"
sed "s#$WORKDIR/mysql-password\$#$WORKDIR/mysql-password-bad#" "$CFG" > "$WORKDIR/config-bad.yaml"
if "$DBVAULT_BIN" backup-create --config "$WORKDIR/config-bad.yaml" > "$WORKDIR/negative.log" 2>&1; then
  drill_fail "backup succeeded with a wrong database password"
fi
grep -q "backup failed" "$WORKDIR/negative.log" || drill_fail "unexpected failure mode: $(cat "$WORKDIR/negative.log")"
echo "[drill] negative leg ok: $(head -n 2 "$WORKDIR/negative.log" | tail -n 1)"

BACKUP_OUT="$("$DBVAULT_BIN" backup-create --config "$CFG")"
echo "$BACKUP_OUT"
SNAP_ID="$(printf '%s\n' "$BACKUP_OUT" | snapshot_id_from_backup_output)"
[ -n "$SNAP_ID" ] || drill_fail "could not parse snapshot id from backup-create output"
echo "[drill] snapshot: $SNAP_ID"

LISTING="$("$DBVAULT_BIN" backup-list --config "$CFG")"
echo "$LISTING"
grep -q "^$SNAP_ID" <<<"$LISTING" || drill_fail "snapshot missing from backup-list"

echo "[drill] simulating data loss: dropping all tables in the source database"
docker exec -i dv-itm1-src mysql -uroot -p"$ROOT_PASS" "$DB_NAME" <<'SQL'
SET FOREIGN_KEY_CHECKS = 0;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS customers;
SET FOREIGN_KEY_CHECKS = 1;
SQL
REMAINING="$(docker exec dv-itm1-src mysql -uroot -p"$ROOT_PASS" -N -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()" "$DB_NAME" 2>/dev/null)"
[ "$REMAINING" = "0" ] || drill_fail "expected empty schema before restore, found $REMAINING tables"

echo "[drill] starting clean target mysql:8.4 on 127.0.0.1:$DST_PORT"
register_container dv-itm1-dst
docker run -d --name dv-itm1-dst \
  -e MYSQL_ROOT_PASSWORD="$ROOT_PASS" \
  -e MYSQL_DATABASE="$DB_NAME" \
  -p "127.0.0.1:$DST_PORT:3306" \
  --health-cmd="mysqladmin ping -h localhost" --health-interval=3s --health-retries=60 --health-start-period=180s \
  mysql:8.4 >/dev/null
wait_container_healthy dv-itm1-dst 360
wait_mysql_tcp "$DST_PORT" root "$ROOT_PASS" 240 || drill_fail "target mysql not accepting queries"; echo "[drill] target ready"

run_step "restore-plan" "$DBVAULT_BIN" restore-plan --config "$CFG" --snapshot "$SNAP_ID" --target "$WORKDIR/restored.sql"
run_step "restore-run --replace" "$DBVAULT_BIN" restore-run --config "$CFG" --snapshot "$SNAP_ID" --target "$WORKDIR/restored.sql" --replace

[ -s "$WORKDIR/restored.sql" ] || drill_fail "restored SQL artifact is empty"
grep -q "$CANARY" "$WORKDIR/restored.sql" || drill_fail "canary token missing from restored SQL dump"

echo "[drill] loading restored SQL into the target container"
docker exec -i dv-itm1-dst mysql -uroot -p"$ROOT_PASS" "$DB_NAME" < "$WORKDIR/restored.sql"

query_dst() { docker exec dv-itm1-dst mysql -uroot -p"$ROOT_PASS" -N -e "$1" "$DB_NAME" 2>/dev/null; }
[ "$(query_dst "SELECT COUNT(*) FROM customers")" = "$EXPECTED_CUSTOMERS" ] || drill_fail "customers row count mismatch after restore"
[ "$(query_dst "SELECT COUNT(*) FROM orders")" = "$EXPECTED_ORDERS" ] || drill_fail "orders row count mismatch after restore"
RESTORED_CHECKSUM="$(query_dst "CHECKSUM TABLE customers, orders EXTENDED" | awk '{printf "%s=%s,", $1, $2}' | sed 's/,$//')"
[ "$RESTORED_CHECKSUM" = "$EXPECTED_CHECKSUM" ] || drill_fail "extended table checksum mismatch: expected [$EXPECTED_CHECKSUM] got [$RESTORED_CHECKSUM]"
[ "$(query_dst "SELECT COUNT(*) FROM orders WHERE note = '$CANARY'")" = "$EXPECTED_ORDERS" ] || drill_fail "canary rows incomplete after restore"

drill_pass "IT-M1 mysql roundtrip (snapshot=$SNAP_ID)"
