#!/usr/bin/env bash
set -euo pipefail

VERSION="${DBVAULT_VERSION:-0.1.0-alpha}"
OUT="${DBVAULT_RELEASE_DIR:-dist}"
mkdir -p "$OUT"

npm --prefix web test
npm --prefix web run build
CGO_ENABLED=0 go build -tags=restricted -ldflags "-s -w -X main.version=${VERSION}" -o bin/dbvault ./cmd/dbvault
CGO_ENABLED=0 go build -tags=restricted -ldflags "-s -w -X main.version=${VERSION}" -o bin/dbvault-agent ./cmd/dbvault-agent

TAR="${OUT}/dbvault-${VERSION}-linux-amd64-restricted.tar.gz"
tar -czf "$TAR" \
  bin/dbvault \
  bin/dbvault-agent \
  README.md \
  docs/alpha/ALPHA_0_1_RUNBOOK.md \
  docs/alpha/ALPHA_0_1_CHECKLIST.md \
  configs/dbvault.phase8.example.yaml \
  deploy/systemd/dbvault.service

sha256sum "$TAR" > "${TAR}.sha256"
cat > "${OUT}/release-manifest-${VERSION}.json" <<JSON
{
  "name": "DBVault",
  "version": "${VERSION}",
  "channel": "alpha",
  "artifact": "$(basename "$TAR")",
  "checksum_file": "$(basename "$TAR").sha256",
  "ui_embedded": true,
  "production_ready": false
}
JSON

echo "created $TAR"
