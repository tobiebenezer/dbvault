#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
docker compose -f deploy/compose/integration.yml up -d
trap 'docker compose -f deploy/compose/integration.yml down -v' EXIT
CGO_ENABLED=1 go test -tags=integration ./integration/...
