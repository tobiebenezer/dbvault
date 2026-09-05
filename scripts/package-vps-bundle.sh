#!/usr/bin/env bash
# ==============================================================================
# DBVault VPS Package Builder
# ==============================================================================
# Builds the production web UI and self-contained Linux amd64 binary,
# and bundles them with the automated installation script into a single archive.
# ==============================================================================

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
VERSION="${DBVAULT_VERSION:-0.1.0-alpha}"
PACKAGE_NAME="dbvault-vps-installer.tar.gz"

echo "==> Building web assets..."
npm --prefix "${ROOT_DIR}/web" run build

echo "==> Compiling standalone Linux amd64 binary (with embedded UI)..."
mkdir -p "${ROOT_DIR}/bin" "${DIST_DIR}"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o "${ROOT_DIR}/bin/dbvault" \
    "${ROOT_DIR}/cmd/dbvault"

echo "==> Preparing VPS deployment package..."
TMP_PKG_DIR="$(mktemp -d /tmp/dbvault-pkg.XXXXXX)"
trap 'rm -rf "${TMP_PKG_DIR}"' EXIT

mkdir -p "${TMP_PKG_DIR}/dbvault-installer"
cp "${ROOT_DIR}/bin/dbvault" "${TMP_PKG_DIR}/dbvault-installer/dbvault"
cp "${ROOT_DIR}/scripts/install-vps.sh" "${TMP_PKG_DIR}/dbvault-installer/install.sh"
chmod +x "${TMP_PKG_DIR}/dbvault-installer/install.sh"
chmod +x "${TMP_PKG_DIR}/dbvault-installer/dbvault"

# Create archive
tar -czf "${DIST_DIR}/${PACKAGE_NAME}" -C "${TMP_PKG_DIR}/dbvault-installer" .

echo ""
echo "=================================================================="
echo " ✓ VPS Deployment package created successfully!"
echo " Package: ${DIST_DIR}/${PACKAGE_NAME} ($(du -h "${DIST_DIR}/${PACKAGE_NAME}" | awk '{print $1}'))"
echo "=================================================================="
echo ""
echo "Next steps to install on your VPS:"
echo ""
echo "1. Upload package to your VPS:"
echo "   scp ${DIST_DIR}/${PACKAGE_NAME} user@<your-vps-ip>:~/"
echo ""
echo "2. SSH into your VPS:"
echo "   ssh user@<your-vps-ip>"
echo ""
echo "3. Extract and run automated installer:"
echo "   mkdir -p ~/dbvault-installer && tar -xzf ${PACKAGE_NAME} -C ~/dbvault-installer"
echo "   cd ~/dbvault-installer"
echo "   sudo ./install.sh"
echo ""
echo "=================================================================="
