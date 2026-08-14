#!/bin/sh
set -eu
mkdir -p dist
if [ ! -x bin/dbvault ]; then
  echo "missing bin/dbvault; run make build-appliance first" >&2
  exit 1
fi
sha256sum bin/dbvault > dist/checksums.txt
cat > dist/manifest.json <<MANIFEST
{"format":"dbvault-release-manifest","version":"dev","artifacts":[{"name":"dbvault","os":"linux","arch":"$(go env GOARCH)","sha256":"$(sha256sum bin/dbvault | awk '{print $1}')"}]}
MANIFEST
tar -cf dist/dbvault-offline-linux-$(go env GOARCH).tar bin/dbvault dist/manifest.json dist/checksums.txt deploy/systemd/dbvault.service 2>/dev/null || tar -cf dist/dbvault-offline-linux-$(go env GOARCH).tar bin/dbvault dist/manifest.json dist/checksums.txt
