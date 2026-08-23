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

# The API surface is behind the auth boundary: bootstrap the initial
# administrator with the demo setup token, then reuse the session cookie.
SETUP_TOKEN="$(grep -o 'setup token: [A-F0-9-]*' "$ROOT/server.log" | awk '{print $3}')"
if [[ -z "$SETUP_TOKEN" ]]; then
  echo "setup token not found in server log" >&2
  exit 1
fi
curl -fsS -c "$ROOT/cookies.txt" -H "Content-Type: application/json" \
  -d "{\"setup_token\":\"${SETUP_TOKEN}\",\"username\":\"smoke-admin\",\"password\":\"smoke-password-123\"}" \
  "http://127.0.0.1:${PORT}/api/v1/auth/bootstrap" | grep -q "administrator-created"

AUTH=(-b "$ROOT/cookies.txt")
fetch() { # fetch <path> <out-file> [curl args...]
  local path="$1" out="$2"
  shift 2
  curl -fsS "${AUTH[@]}" "$@" "http://127.0.0.1:${PORT}${path}" -o "$out"
}

fetch "/" "$ROOT/index.html"
grep -qi "DBVault" "$ROOT/index.html"
fetch "/assets/app.js" "$ROOT/app.js"
grep -q "DBVault" "$ROOT/app.js"
fetch "/api/v1/overview" "$ROOT/overview.json"
grep -q "databases" "$ROOT/overview.json"
fetch "/api/v1/protection-summary" "$ROOT/protection.json"
grep -q "status" "$ROOT/protection.json"
fetch "/api/v1/sources/production-postgres/recovery-timeline" "$ROOT/timeline.json"
grep -q "source_id" "$ROOT/timeline.json"
fetch "/api/v1/doctor/run" "$ROOT/doctor.json" -X POST
grep -q "checks" "$ROOT/doctor.json"
fetch "/api/v1/policies/simulate" "$ROOT/policy.json" -X POST -H "Content-Type: application/json" \
  -d '{"source_id":"production-postgres","repository_id":"production","policy":{"backup_frequency":"6h","daily_retention":7,"weekly_retention":4,"monthly_retention":3,"replica_count":2,"restore_drill":"weekly"}}'
grep -q "estimated" "$ROOT/policy.json"
fetch "/api/v1/support-bundles" "$ROOT/support.json" -X POST
grep -q "support" "$ROOT/support.json"
fetch "/api/v1/recovery-bundles" "$ROOT/recovery.json" -X POST
grep -q "recovery" "$ROOT/recovery.json"

echo "alpha smoke OK: install, embedded UI, demo API, doctor, simulation and bundles are reachable"
