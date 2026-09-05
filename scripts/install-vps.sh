#!/usr/bin/env bash
# ==============================================================================
# DBVault Automated VPS Installer & Updater
# ==============================================================================
# Fresh install:  curl -fsSL https://raw.githubusercontent.com/tobiebenezer/dbvault/main/scripts/install-vps.sh | sudo bash
# In-place update: sudo dbvault-update  (or re-run curl command above)
# Check updates:  sudo dbvault-update --check
# ==============================================================================

set -euo pipefail

# Color formatting
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

PORT="${DBVAULT_PORT:-2633}"
BIND_ADDR="${DBVAULT_BIND:-0.0.0.0}"
DATA_DIR="${DBVAULT_DATA_DIR:-/var/lib/dbvault}"
BIN_DIR="/usr/local/bin"
SERVICE_FILE="/etc/systemd/system/dbvault.service"
GITHUB_REPO="${DBVAULT_REPO:-tobiebenezer/dbvault}"
DOWNLOAD_URL="${DBVAULT_DOWNLOAD_URL:-}"

UPDATE_MODE=false
CHECK_ONLY=false
FORCE=false

for arg in "$@"; do
    case "$arg" in
        --update|-u)
            UPDATE_MODE=true
            ;;
        --check|-c)
            CHECK_ONLY=true
            ;;
        --force|-f)
            FORCE=true
            ;;
        --help|-h)
            echo "Usage: sudo $0 [options]"
            echo ""
            echo "Options:"
            echo "  --update, -u   Update DBVault to the latest release (safe in-place upgrade)"
            echo "  --check, -c    Check if a newer DBVault release is available on GitHub"
            echo "  --force, -f    Force reinstall/update even if already on the same version"
            echo "  --help, -h     Show this help message"
            echo ""
            echo "Environment variables:"
            echo "  DBVAULT_PORT          Listening port (default: 2633)"
            echo "  DBVAULT_BIND          Bind address (default: 0.0.0.0)"
            echo "  DBVAULT_DATA_DIR      Data directory (default: /var/lib/dbvault)"
            echo "  DBVAULT_REPO          GitHub repository (default: tobiebenezer/dbvault)"
            echo "  DBVAULT_DOWNLOAD_URL  Custom download URL for the dbvault binary"
            echo "  DBVAULT_FORCE         Set to 1 to force reinstallation"
            exit 0
            ;;
    esac
done

if [[ "${DBVAULT_FORCE:-0}" == "1" ]]; then
    FORCE=true
fi
if [[ "${DBVAULT_UPDATE:-0}" == "1" ]]; then
    UPDATE_MODE=true
fi

# Helper to read input safely even when piped into bash (curl ... | bash)
prompt_yes_no() {
    local prompt_msg="$1"
    local default_ans="$2"
    local ans=""
    if [ -t 0 ]; then
        read -r -p "$prompt_msg " ans || ans="$default_ans"
    elif [ -e /dev/tty ]; then
        read -r -p "$prompt_msg " ans </dev/tty || ans="$default_ans"
    else
        ans="$default_ans"
    fi
    ans="${ans:-$default_ans}"
    [[ "$ans" =~ ^([yY][eE][sS]|[yY])+$ ]]
}

# ------------------------------------------------------------------------------
# 1. Existing Installation & Version Check
# ------------------------------------------------------------------------------
IS_INSTALLED=false
CURRENT_VERSION=""
if [[ -f "${BIN_DIR}/dbvault" && -x "${BIN_DIR}/dbvault" ]]; then
    IS_INSTALLED=true
    CURRENT_VERSION=$("${BIN_DIR}/dbvault" version 2>/dev/null | awk '{print $2}' | sed 's/^v//' || echo "")
fi

# Resolve latest release from GitHub
LATEST_TAG=""
LATEST_VERSION=""
if command -v curl >/dev/null 2>&1; then
    LATEST_TAG=$(curl -fsSL --connect-timeout 5 -H "Accept: application/vnd.github.v3+json" "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | head -n 1 | sed -E 's/.*"tag_name": "([^"]+)".*/\1/' || true)
    if [[ -z "$LATEST_TAG" ]]; then
        LATEST_TAG=$(curl -sI --connect-timeout 5 "https://github.com/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep -i '^location:' | sed -E 's|.*/tag/([^/\r\n]+).*|\1|' || true)
    fi
    if [[ "$LATEST_TAG" =~ /releases/?$ || "$LATEST_TAG" =~ ^location: ]]; then
        LATEST_TAG=""
    fi
fi
if [[ -n "$LATEST_TAG" ]]; then
    LATEST_VERSION="${LATEST_TAG#v}"
fi

# Handle --check flag (does not require root)
if [[ "$CHECK_ONLY" == true ]]; then
    echo -e "${BOLD}${CYAN}DBVault Version Check${NC}"
    echo "=================================================================="
    echo -e "Installed version: ${BOLD}${CURRENT_VERSION:-Not installed}${NC}"
    echo -e "Latest release:    ${BOLD}${LATEST_VERSION:-Unavailable}${NC}"
    echo "=================================================================="
    if [[ "$IS_INSTALLED" != true ]]; then
        echo -e "${YELLOW}DBVault is not currently installed.${NC}"
        echo "Run 'sudo $0' to install."
    elif [[ -n "$LATEST_VERSION" && "$CURRENT_VERSION" == "$LATEST_VERSION" ]]; then
        echo -e "${GREEN}✓ DBVault is up to date (v${CURRENT_VERSION}).${NC}"
    elif [[ -n "$LATEST_VERSION" ]]; then
        echo -e "${YELLOW}→ An updated release is available: v${CURRENT_VERSION} → v${LATEST_VERSION}${NC}"
        echo "To update, run: sudo dbvault-update (or sudo $0 --update)"
    fi
    exit 0
fi

# ------------------------------------------------------------------------------
# 2. Privilege Check
# ------------------------------------------------------------------------------
if [[ $EUID -ne 0 ]]; then
    echo -e "${RED}[ERROR] This script must be run as root or via sudo.${NC}"
    echo "Please re-run: sudo $0 or curl ... | sudo bash"
    exit 1
fi

# Check if already installed and already at latest version
LOCAL_PACKAGE_EXISTS=false
if [[ -f "./dbvault" && -x "./dbvault" ]]; then
    LOCAL_PACKAGE_EXISTS=true
fi

if [[ "$IS_INSTALLED" == true && "$FORCE" != true && "$LOCAL_PACKAGE_EXISTS" != true ]]; then
    if [[ -n "$CURRENT_VERSION" && -n "$LATEST_VERSION" && "$CURRENT_VERSION" == "$LATEST_VERSION" ]]; then
        echo -e "${GREEN}✓ DBVault is already installed and up to date (v${CURRENT_VERSION}).${NC}"
        echo "  Web console: http://${BIND_ADDR}:${PORT} (or your VPS IP)"
        echo "  Service:     Active (dbvault.service)"
        echo ""
        echo "To force re-installation or refresh binaries, run:"
        echo "  sudo $0 --force"
        exit 0
    fi
fi

# Display appropriate banner
echo -e "${BOLD}${CYAN}"
echo "=================================================================="
if [[ "$IS_INSTALLED" == true ]]; then
    echo "          DBVault Appliance - In-Place Version Update             "
else
    echo "          DBVault Appliance - VPS Automated Installation          "
fi
echo "=================================================================="
echo -e "${NC}"

if [[ "$IS_INSTALLED" == true ]]; then
    echo -e "${BLUE}Updating DBVault on this system:${NC}"
    echo -e "  Current installed version: ${BOLD}v${CURRENT_VERSION:-unknown}${NC}"
    if [[ -n "$LATEST_VERSION" ]]; then
        echo -e "  Target update version:    ${BOLD}v${LATEST_VERSION}${NC}"
    fi
    echo -e "  ${GREEN}✓ Existing configurations, keys, and databases in ${DATA_DIR} will be preserved.${NC}"
    echo ""
fi

# ------------------------------------------------------------------------------
# 3. System Architecture & Binary Resolution
# ------------------------------------------------------------------------------
echo -e "${BLUE}[1/7] Confirming architecture and resolving binary...${NC}"
ARCH=$(uname -m)
GOARCH="amd64"
case "$ARCH" in
    x86_64|amd64)
        GOARCH="amd64"
        echo -e "  ${GREEN}✓ System architecture: $ARCH (amd64)${NC}"
        ;;
    aarch64|arm64)
        GOARCH="arm64"
        echo -e "  ${GREEN}✓ System architecture: $ARCH (arm64)${NC}"
        ;;
    *)
        echo -e "  ${YELLOW}! Warning: Architecture $ARCH. Defaulting to amd64.${NC}"
        ;;
esac

SOURCE_BIN=""
# Check local files first (e.g. if unpacked from release tarball or repo)
TARGET_PATH_REAL=""
if [[ -f "${BIN_DIR}/dbvault" ]]; then
    TARGET_PATH_REAL=$(realpath "${BIN_DIR}/dbvault" 2>/dev/null || true)
fi

if [[ -f "./dbvault" && -x "./dbvault" ]]; then
    LOCAL_REAL=$(realpath "./dbvault" 2>/dev/null || true)
    if [[ "$LOCAL_REAL" != "$TARGET_PATH_REAL" ]]; then
        SOURCE_BIN="./dbvault"
    fi
elif [[ -f "./bin/dbvault" && -x "./bin/dbvault" ]]; then
    LOCAL_REAL=$(realpath "./bin/dbvault" 2>/dev/null || true)
    if [[ "$LOCAL_REAL" != "$TARGET_PATH_REAL" ]]; then
        SOURCE_BIN="./bin/dbvault"
    fi
elif command -v go >/dev/null 2>&1 && [[ -f "./cmd/dbvault/main.go" ]]; then
    echo "  Building dbvault binary from local Go source..."
    CGO_ENABLED=0 go build -ldflags "-s -w" -o ./bin/dbvault ./cmd/dbvault
    SOURCE_BIN="./bin/dbvault"
fi

# If binary not present locally, download from GitHub release
if [[ -z "$SOURCE_BIN" ]]; then
    echo "  Resolving latest release binary from GitHub (${GITHUB_REPO})..."
    TMP_DL_DIR="$(mktemp -d /tmp/dbvault-bin.XXXXXX)"
    TARGET_DL="${TMP_DL_DIR}/dbvault"

    if [[ -n "$DOWNLOAD_URL" ]]; then
        echo "  Downloading from custom URL: ${DOWNLOAD_URL}..."
        curl -fsSL "${DOWNLOAD_URL}" -o "${TARGET_DL}"
    else
        RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/dbvault-linux-${GOARCH}"
        echo "  Attempting to download from ${RELEASE_URL}..."
        if ! curl -fsSL "${RELEASE_URL}" -o "${TARGET_DL}" 2>/dev/null; then
            TAR_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/dbvault-vps-installer.tar.gz"
            echo "  Attempting to download package from ${TAR_URL}..."
            if curl -fsSL "${TAR_URL}" -o "${TMP_DL_DIR}/installer.tar.gz" 2>/dev/null; then
                tar -xzf "${TMP_DL_DIR}/installer.tar.gz" -C "${TMP_DL_DIR}"
                if [[ -f "${TMP_DL_DIR}/dbvault" ]]; then
                    TARGET_DL="${TMP_DL_DIR}/dbvault"
                fi
            else
                echo -e "${RED}[ERROR] Could not download DBVault release binary.${NC}"
                echo "Please check https://github.com/${GITHUB_REPO}/releases or set DBVAULT_DOWNLOAD_URL."
                exit 1
            fi
        fi
    fi
    chmod +x "${TARGET_DL}"
    SOURCE_BIN="${TARGET_DL}"
fi

# Validate binary
if ! "${SOURCE_BIN}" version >/dev/null 2>&1 && ! "${SOURCE_BIN}" -v >/dev/null 2>&1 && ! "${SOURCE_BIN}" --help >/dev/null 2>&1; then
    echo -e "${RED}[ERROR] Resolved binary is corrupted or cannot execute on this system.${NC}"
    exit 1
fi

NEW_BIN_VERSION=$("${SOURCE_BIN}" version 2>/dev/null | awk '{print $2}' || echo "new")
echo -e "  ${GREEN}✓ Resolved DBVault executable (${NEW_BIN_VERSION})${NC}"

# ------------------------------------------------------------------------------
# 4. Prerequisites & Utilities Check
# ------------------------------------------------------------------------------
echo -e "${BLUE}[2/7] Confirming system extraction utilities...${NC}"
PKG_MGR=""
if command -v apt-get >/dev/null 2>&1; then
    PKG_MGR="apt"
elif command -v dnf >/dev/null 2>&1; then
    PKG_MGR="dnf"
elif command -v yum >/dev/null 2>&1; then
    PKG_MGR="yum"
fi

MISSING_TOOLS=()
for tool in curl gzip tar; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        MISSING_TOOLS+=("$tool")
    fi
done

if [[ ${#MISSING_TOOLS[@]} -gt 0 ]]; then
    echo -e "  ${YELLOW}! Missing core utilities: ${MISSING_TOOLS[*]}${NC}"
    echo "  Installing core utilities..."
    if [[ "$PKG_MGR" == "apt" ]]; then
        apt-get update -qq && apt-get install -y -qq "${MISSING_TOOLS[@]}"
    elif [[ "$PKG_MGR" == "dnf" || "$PKG_MGR" == "yum" ]]; then
        "$PKG_MGR" install -y -q "${MISSING_TOOLS[@]}"
    fi
fi
echo -e "  ${GREEN}✓ Core utilities confirmed (curl, gzip, tar)${NC}"

# Database clients
CLIENTS_FOUND=()
if command -v mysqldump >/dev/null 2>&1; then
    CLIENTS_FOUND+=("MySQL/MariaDB (mysqldump)")
else
    if [[ "$IS_INSTALLED" != true && -n "$PKG_MGR" ]]; then
        if prompt_yes_no "  Install MySQL/MariaDB client tools now? [Y/n]" "y"; then
            if [[ "$PKG_MGR" == "apt" ]]; then
                apt-get install -y -qq default-mysql-client || apt-get install -y -qq mariadb-client
            elif [[ "$PKG_MGR" == "dnf" || "$PKG_MGR" == "yum" ]]; then
                "$PKG_MGR" install -y -q mariadb
            fi
            CLIENTS_FOUND+=("MySQL/MariaDB (installed)")
        fi
    fi
fi

if command -v pg_dump >/dev/null 2>&1; then
    CLIENTS_FOUND+=("PostgreSQL (pg_dump)")
else
    if [[ "$IS_INSTALLED" != true && -n "$PKG_MGR" ]]; then
        if prompt_yes_no "  Install PostgreSQL client tools now? [y/N]" "n"; then
            if [[ "$PKG_MGR" == "apt" ]]; then
                apt-get install -y -qq postgresql-client
            elif [[ "$PKG_MGR" == "dnf" || "$PKG_MGR" == "yum" ]]; then
                "$PKG_MGR" install -y -q postgresql
            fi
            CLIENTS_FOUND+=("PostgreSQL (installed)")
        fi
    fi
fi

if command -v sqlite3 >/dev/null 2>&1; then
    CLIENTS_FOUND+=("SQLite (sqlite3)")
fi
echo -e "  ${GREEN}✓ Database dump clients: ${CLIENTS_FOUND[*]:-None (install per your DB engine)}${NC}"

# ------------------------------------------------------------------------------
# 5. Installing Binary & Setting Up Fast Updater
# ------------------------------------------------------------------------------
echo -e "${BLUE}[3/7] Installing binary & update manager...${NC}"
mkdir -p "${BIN_DIR}"

if [[ -f "${BIN_DIR}/dbvault" ]]; then
    # Keep safe backup of previous binary
    cp -f "${BIN_DIR}/dbvault" "${BIN_DIR}/dbvault.bak"
fi

cp -f "${SOURCE_BIN}" "${BIN_DIR}/dbvault"
chmod 0755 "${BIN_DIR}/dbvault"
echo -e "  ${GREEN}✓ Installed executable to ${BIN_DIR}/dbvault${NC}"

# Create convenient `dbvault-update` helper command
cat << EOF_UPDATER > "${BIN_DIR}/dbvault-update"
#!/usr/bin/env bash
set -euo pipefail
if [[ \$EUID -ne 0 ]]; then
    echo "Error: dbvault-update must be run as root (use: sudo dbvault-update)"
    exit 1
fi
REPO="${GITHUB_REPO}"
exec curl -fsSL "https://raw.githubusercontent.com/\${REPO}/main/scripts/install-vps.sh" | bash -s -- "\$@"
EOF_UPDATER
chmod 0755 "${BIN_DIR}/dbvault-update"
echo -e "  ${GREEN}✓ Created updater utility: ${BIN_DIR}/dbvault-update${NC}"

# Ensure data directory exists with strict permissions
mkdir -p "${DATA_DIR}"
chmod 0700 "${DATA_DIR}"

# ------------------------------------------------------------------------------
# 6. Systemd Service Setup & Daemon Refresh
# ------------------------------------------------------------------------------
echo -e "${BLUE}[4/7] Configuring systemd service...${NC}"

cat << EOF_SERVICE > "${SERVICE_FILE}"
[Unit]
Description=DBVault Database Protection Appliance
Documentation=https://github.com/${GITHUB_REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${DATA_DIR}
ExecStart=${BIN_DIR}/dbvault server --data-dir ${DATA_DIR} --listen ${BIND_ADDR}:${PORT}
Restart=always
RestartSec=5s
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF_SERVICE

chmod 0644 "${SERVICE_FILE}"
systemctl daemon-reload
systemctl enable dbvault.service
systemctl restart dbvault.service
echo -e "  ${GREEN}✓ Systemd service configured & restarted (dbvault.service)${NC}"

# ------------------------------------------------------------------------------
# 7. Verifying Daemon Readiness & Health Probe
# ------------------------------------------------------------------------------
echo -e "${BLUE}[5/7] Probing DBVault health...${NC}"

MAX_ATTEMPTS=25
ATTEMPT=1
SERVER_HEALTHY=false

while [[ $ATTEMPT -le $MAX_ATTEMPTS ]]; do
    if curl -s -f "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then
        SERVER_HEALTHY=true
        break
    fi
    sleep 1
    ((ATTEMPT++))
done

if [[ "$SERVER_HEALTHY" != true ]]; then
    echo -e "  ${RED}[ERROR] DBVault did not report healthy after ${MAX_ATTEMPTS} seconds.${NC}"
    echo "  Checking recent service logs:"
    journalctl -u dbvault -n 25 --no-pager
    if [[ -f "${BIN_DIR}/dbvault.bak" ]]; then
        echo -e "${YELLOW}Rollback binary available at ${BIN_DIR}/dbvault.bak${NC}"
    fi
    exit 1
fi
echo -e "  ${GREEN}✓ Health check passed (HTTP 200 from http://127.0.0.1:${PORT}/health)${NC}"

# ------------------------------------------------------------------------------
# 8. Firewall Configuration
# ------------------------------------------------------------------------------
echo -e "${BLUE}[6/7] Checking firewall rules...${NC}"
if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
    ufw allow "${PORT}/tcp" >/dev/null 2>&1 || true
    echo -e "  ${GREEN}✓ UFW firewall rule active for port ${PORT}/tcp${NC}"
elif command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld; then
    firewall-cmd --add-port="${PORT}/tcp" --permanent >/dev/null 2>&1 || true
    firewall-cmd --reload >/dev/null 2>&1 || true
    echo -e "  ${GREEN}✓ Firewalld rule active for port ${PORT}/tcp${NC}"
else
    echo -e "  ${GREEN}✓ Port :${PORT} accessible${NC}"
fi

# Detect Public Server IP
PUBLIC_IP=$(curl -s --max-time 3 https://ifconfig.me 2>/dev/null || curl -s --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}')
PUBLIC_IP="${PUBLIC_IP:-127.0.0.1}"

# ------------------------------------------------------------------------------
# 9. Completion Summary
# ------------------------------------------------------------------------------
echo -e "${BLUE}[7/7] Finalizing...${NC}"
echo ""
echo -e "${BOLD}${GREEN}==================================================================${NC}"
if [[ "$IS_INSTALLED" == true ]]; then
    echo -e "${BOLD}${GREEN}        ✓ DBVault has been successfully updated!                  ${NC}"
else
    echo -e "${BOLD}${GREEN}        ✓ DBVault has been successfully installed & verified!    ${NC}"
fi
echo -e "${BOLD}${GREEN}==================================================================${NC}"
echo ""

if [[ "$IS_INSTALLED" == true ]]; then
    echo -e "${BOLD}1. Update Summary:${NC}"
    echo -e "   Previous Version:  v${CURRENT_VERSION:-unknown}"
    echo -e "   Current Version:   ${BOLD}${NEW_BIN_VERSION}${NC}"
    echo -e "   Storage & State:   ${GREEN}All databases, keys, and schedules preserved${NC}"
    echo ""
    echo -e "${BOLD}2. Web Console Access URL:${NC}"
    echo -e "   ${CYAN}http://${PUBLIC_IP}:${PORT}${NC}"
    echo ""
    echo -e "${BOLD}3. Easy Future Updates:${NC}"
    echo "   Check for updates: sudo dbvault-update --check"
    echo "   Apply updates:     sudo dbvault-update"
else
    # First-run setup token
    SETUP_TOKEN=$(journalctl -u dbvault -n 100 --no-pager 2>/dev/null | grep -i "setup token:" | tail -n 1 | sed 's/.*setup token: //I' | tr -d '\r')
    echo -e "${BOLD}1. Web Console Access URL:${NC}"
    echo -e "   ${CYAN}http://${PUBLIC_IP}:${PORT}${NC}"
    echo ""
    if [[ -n "$SETUP_TOKEN" ]]; then
        echo -e "${BOLD}2. Appliance First-Run Setup Token:${NC}"
        echo -e "   ${YELLOW}${BOLD}${SETUP_TOKEN}${NC}"
        echo -e "   ${NC}(Paste this token into your browser to create your admin account)${NC}"
    else
        echo -e "${BOLD}2. Appliance Setup Token:${NC}"
        echo -e "   Sign in at http://${PUBLIC_IP}:${PORT}/login"
    fi
    echo ""
    echo -e "${BOLD}3. Easy Future Updates:${NC}"
    echo "   Whenever an update is released, simply run:"
    echo -e "   ${CYAN}sudo dbvault-update${NC}"
fi

echo ""
echo -e "${BOLD}Service Management:${NC}"
echo "   Status:  sudo systemctl status dbvault"
echo "   Logs:    sudo journalctl -u dbvault -f"
echo "   Restart: sudo systemctl restart dbvault"
echo "=================================================================="
