#!/usr/bin/env bash
set -euo pipefail

ROOT="${DBVAULT_SMOKE_ROOT:-/tmp/dbvault-alpha-smoke}"
PORT="${DBVAULT_SMOKE_PORT:-18101}"
BIN="${DBVAULT_BIN:-./bin/dbvault}"

rm -rf "$ROOT"
mkdir -p "$ROOT"

if [[ ! -x "$BIN" ]]; then
  mkdir -p bin
  CGO_ENABLED=0 go build -tags=restricted -o "$BIN" ./cmd/dbvault
fi

"$BIN" install --root "$ROOT/install" --domain alpha.local --self-signed > "$ROOT/install.log"

grep -qi "setup URL" "$ROOT/install.log"
grep -qi "setup token" "$ROOT/install.log"

"$BIN" server --listen "127.0.0.1:${PORT}" --data-dir "$ROOT/server" --demo > "$ROOT/server.log" 2>&1 &
PID=$!
trap 'kill "$PID" >/dev/null 2>&1 || true' EXIT

for i in $(seq 1 80); do
  if curl -fsS "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
  if [[ "$i" -eq 80 ]]; then
    echo "server did not become healthy" >&2
    cat "$ROOT/server.log" >&2 || true
    exit 1
  fi
done

curl -fsS "http://127.0.0.1:${PORT}/" | grep -qi "DBVault"
curl -fsS "http://127.0.0.1:${PORT}/assets/app.js" | grep -q "DBVault"
curl -fsS "http://127.0.0.1:${PORT}/api/v1/overview" | grep -q "databases"
curl -fsS "http://127.0.0.1:${PORT}/api/v1/protection-summary" | grep -q "status"
curl -fsS "http://127.0.0.1:${PORT}/api/v1/sources/production-postgres/recovery-timeline" | grep -q "source_id"
curl -fsS -X POST "http://127.0.0.1:${PORT}/api/v1/doctor/run" | grep -q "checks"
curl -fsS -X POST -H "Content-Type: application/json" -d '{"source_id":"production-postgres","repository_id":"production","policy":{"backup_frequency":"6h","daily_retention":7,"weekly_retention":4,"monthly_retention":3,"replica_count":2,"restore_drill":"weekly"}}' "http://127.0.0.1:${PORT}/api/v1/policies/simulate" | grep -q "estimated"
curl -fsS -X POST "http://127.0.0.1:${PORT}/api/v1/support-bundles" | grep -q "support"
curl -fsS -X POST "http://127.0.0.1:${PORT}/api/v1/recovery-bundles" | grep -q "recovery"

echo "alpha smoke OK: install, embedded UI, demo API, doctor, simulation and bundles are reachable"
