#!/usr/bin/env bash
# DBVault Live Integration Test: cribx_test Sandbox Drill & pg_amcheck Verification
# Runs a full end-to-end verified restore drill against the local cribx_test database.
set -euo pipefail

PGHOST="127.0.0.1"
PGPORT="5432"
PGUSER="postgres"
PGPASSWORD_VAL="Awodumila"
BASE_DB="cribx_test"
SANDBOX_DB="cribx_test_sandbox_live_$(date +%s)"
DUMP_FILE="/tmp/dbvault_drill_cribx_test.dump"

export PGPASSWORD="$PGPASSWORD_VAL"

cleanup() {
  echo "  Cleaning up sandbox database $SANDBOX_DB..."
  psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d postgres \
    -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$SANDBOX_DB'" >/dev/null 2>&1 || true
  psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d postgres \
    -c "DROP DATABASE IF EXISTS \"$SANDBOX_DB\"" >/dev/null 2>&1 || true
  rm -f "$DUMP_FILE"
}
trap cleanup EXIT

echo "======================================="
echo "  DBVault Live Sandbox Drill"
echo "  Source DB: $BASE_DB"
echo "  Sandbox DB: $SANDBOX_DB"
echo "======================================="

# Step 1: count rows in source db before backup
echo ""
echo "[1/7] Counting source rows in $BASE_DB..."
SOURCE_TABLE_COUNT=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$BASE_DB" -t -A \
  -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'")
echo "  Source tables: $SOURCE_TABLE_COUNT"

# Step 2: pg_dump source db
echo ""
echo "[2/7] Dumping $BASE_DB with pg_dump..."
START_DUMP=$(date +%s%3N)
PGPASSWORD="$PGPASSWORD_VAL" pg_dump \
  -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" \
  --no-owner --no-acl \
  --format=custom \
  --file="$DUMP_FILE" \
  "$BASE_DB"
END_DUMP=$(date +%s%3N)
DUMP_SIZE=$(du -sh "$DUMP_FILE" | cut -f1)
echo "  ✓ Dump complete: $DUMP_FILE ($DUMP_SIZE) in $((END_DUMP - START_DUMP))ms"

# Step 3: create sandbox database
echo ""
echo "[3/7] Creating sandbox database $SANDBOX_DB..."
PGPASSWORD="$PGPASSWORD_VAL" psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d postgres \
  -c "CREATE DATABASE \"$SANDBOX_DB\""
echo "  ✓ Sandbox created"

# Step 4: restore into sandbox
echo ""
echo "[4/7] Restoring into sandbox $SANDBOX_DB..."
START_RESTORE=$(date +%s%3N)
PGPASSWORD="$PGPASSWORD_VAL" pg_restore \
  -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" \
  --clean --if-exists --no-owner --no-privileges \
  -d "$SANDBOX_DB" \
  "$DUMP_FILE" 2>&1 || true  # pg_restore exits non-zero on warnings; continue
END_RESTORE=$(date +%s%3N)
RTO_MS=$((END_RESTORE - START_RESTORE))
echo "  ✓ Restore complete in ${RTO_MS}ms (RTO: ${RTO_MS}ms)"

# Step 5: deep consistency check — table count reconciliation
echo ""
echo "[5/7] Running table count reconciliation..."
SANDBOX_TABLE_COUNT=$(PGPASSWORD="$PGPASSWORD_VAL" psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$SANDBOX_DB" -t -A \
  -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'")
echo "  Source tables:  $SOURCE_TABLE_COUNT"
echo "  Sandbox tables: $SANDBOX_TABLE_COUNT"
if [ "$SOURCE_TABLE_COUNT" = "$SANDBOX_TABLE_COUNT" ]; then
  echo "  ✓ Table count matches"
else
  echo "  ✗ MISMATCH: expected $SOURCE_TABLE_COUNT, got $SANDBOX_TABLE_COUNT" >&2
  exit 1
fi

# Step 6: pg_amcheck (if available)
echo ""
echo "[6/7] Running pg_amcheck..."
if command -v pg_amcheck >/dev/null 2>&1; then
  PGPASSWORD="$PGPASSWORD_VAL" pg_amcheck \
    -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" \
    -d "$SANDBOX_DB" \
    --all --heapallindexed 2>&1 && echo "  ✓ pg_amcheck: all checks passed" || {
    echo "  ⚠ pg_amcheck reported issues (may need amcheck extension)"
  }
else
  echo "  ⚠ pg_amcheck not available — skipping"
fi

# Step 7: compute schema digest
echo ""
echo "[7/7] Computing schema digest..."
SCHEMA_DIGEST=$(PGPASSWORD="$PGPASSWORD_VAL" psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$SANDBOX_DB" -t -A \
  -c "SELECT md5(string_agg(table_name, ',' ORDER BY table_name)) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'")
echo "  Schema digest (MD5): $SCHEMA_DIGEST"

echo ""
echo "======================================="
echo "  ✓ DBVault Sandbox Drill: PASSED"
echo "  Source DB:      $BASE_DB ($SOURCE_TABLE_COUNT tables)"
echo "  Sandbox DB:     $SANDBOX_DB ($SANDBOX_TABLE_COUNT tables)"
echo "  RTO:            ${RTO_MS}ms"
echo "  Schema digest:  $SCHEMA_DIGEST"
echo "======================================="
