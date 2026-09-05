#!/usr/bin/env bash
# ==============================================================================
# DBVault Automated VPS Installer & Verification Script
# ==============================================================================
# Can be run locally:   sudo ./scripts/install-vps.sh
# Or via curl one-liner: curl -fsSL https://raw.githubusercontent.com/tobiebenezer/dbvault/main/scripts/install-vps.sh | sudo bash
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

echo -e "${BOLD}${CYAN}"
echo "=================================================================="
echo "          DBVault Appliance - VPS Automated Installation          "
echo "=================================================================="
echo -e "${NC}"

# ------------------------------------------------------------------------------
# 1. Root / Sudo Check
# ------------------------------------------------------------------------------
echo -e "${BLUE}[1/8] Confirming privileges...${NC}"
if [[ $EUID -ne 0 ]]; then
    echo -e "${RED}[ERROR] This installation script must be run as root or via sudo.${NC}"
    echo "Please re-run: sudo $0 or curl ... | sudo bash"
    exit 1
fi
echo -e "  ${GREEN}✓ Running with administrative privileges${NC}"

# ------------------------------------------------------------------------------
# 2. System Architecture & Binary Resolution
# ------------------------------------------------------------------------------
echo -e "${BLUE}[2/8] Confirming architecture and resolving binary...${NC}"
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
# Check local files first
if [[ -f "./dbvault" && -x "./dbvault" ]]; then
    SOURCE_BIN="./dbvault"
elif [[ -f "./bin/dbvault" && -x "./bin/dbvault" ]]; then
    SOURCE_BIN="./bin/dbvault"
elif [[ -f "${BIN_DIR}/dbvault" && -x "${BIN_DIR}/dbvault" ]]; then
    SOURCE_BIN="${BIN_DIR}/dbvault"
elif command -v go >/dev/null 2>&1 && [[ -f "./cmd/dbvault/main.go" ]]; then
    echo "  Building dbvault binary from source with Go..."
    CGO_ENABLED=0 go build -ldflags "-s -w" -o ./bin/dbvault ./cmd/dbvault
    SOURCE_BIN="./bin/dbvault"
fi

# If binary still not found, download it
if [[ -z "$SOURCE_BIN" ]]; then
    echo "  Local binary not found. Resolving download source..."
    TMP_DL_DIR="$(mktemp -d /tmp/dbvault-bin.XXXXXX)"
    TARGET_DL="${TMP_DL_DIR}/dbvault"

    if [[ -n "$DOWNLOAD_URL" ]]; then
        echo "  Downloading from custom URL: ${DOWNLOAD_URL}..."
        curl -fsSL "${DOWNLOAD_URL}" -o "${TARGET_DL}"
    else
        # Try fetching from GitHub releases
        RELEASE_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/dbvault-linux-${GOARCH}"
        echo "  Attempting to download from ${RELEASE_URL}..."
        if ! curl -fsSL "${RELEASE_URL}" -o "${TARGET_DL}" 2>/dev/null; then
            # Fallback: check for tarball release
            TAR_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/dbvault-vps-installer.tar.gz"
            echo "  Attempting to download package from ${TAR_URL}..."
            if curl -fsSL "${TAR_URL}" -o "${TMP_DL_DIR}/installer.tar.gz" 2>/dev/null; then
                tar -xzf "${TMP_DL_DIR}/installer.tar.gz" -C "${TMP_DL_DIR}"
                if [[ -f "${TMP_DL_DIR}/dbvault" ]]; then
                    TARGET_DL="${TMP_DL_DIR}/dbvault"
                fi
            else
                echo -e "${RED}[ERROR] Could not find or download the 'dbvault' binary.${NC}"
                echo "Please provide the binary in the current directory or set DBVAULT_DOWNLOAD_URL."
                exit 1
            fi
        fi
    fi
    chmod +x "${TARGET_DL}"
    SOURCE_BIN="${TARGET_DL}"
fi

echo -e "  ${GREEN}✓ Found/resolved DBVault binary: ${SOURCE_BIN}${NC}"

# ------------------------------------------------------------------------------
# 3. Prerequisites & Database Client Utilities Confirmation
# ------------------------------------------------------------------------------
echo -e "${BLUE}[3/8] Confirming database extraction tools & utilities...${NC}"

PKG_MGR=""
if command -v apt-get >/dev/null 2>&1; then
    PKG_MGR="apt"
elif command -v dnf >/dev/null 2>&1; then
    PKG_MGR="dnf"
elif command -v yum >/dev/null 2>&1; then
    PKG_MGR="yum"
fi

# Ensure essential tools: curl, gzip, tar
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

# Check for database dump clients
CLIENTS_FOUND=()
if command -v mysqldump >/dev/null 2>&1; then
    CLIENTS_FOUND+=("MySQL/MariaDB (mysqldump)")
else
    echo -e "  ${YELLOW}! MySQL/MariaDB client not found.${NC}"
    if [[ -n "$PKG_MGR" ]]; then
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
    echo -e "  ${YELLOW}! PostgreSQL client not found.${NC}"
    if [[ -n "$PKG_MGR" ]]; then
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

echo -e "  ${GREEN}✓ Database dump clients available: ${CLIENTS_FOUND[*]:-None (install per your DB engine)}${NC}"

# ------------------------------------------------------------------------------
# 4. Port Availability Confirmation
# ------------------------------------------------------------------------------
echo -e "${BLUE}[4/8] Confirming port :${PORT} availability...${NC}"
PORT_OCCUPIED=false
if command -v ss >/dev/null 2>&1; then
    if ss -tlpn | grep -q ":${PORT} "; then
        PORT_OCCUPIED=true
    fi
elif command -v netstat >/dev/null 2>&1; then
    if netstat -tlpn 2>/dev/null | grep -q ":${PORT} "; then
        PORT_OCCUPIED=true
    fi
elif command -v lsof >/dev/null 2>&1; then
    if lsof -i ":${PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
        PORT_OCCUPIED=true
    fi
fi

if [[ "$PORT_OCCUPIED" == true ]]; then
    echo -e "  ${YELLOW}! Port ${PORT} is currently listening.${NC}"
    if pgrep -x "dbvault" >/dev/null 2>&1; then
        echo -e "  ${YELLOW}An existing DBVault process is currently running. It will be replaced/restarted by systemd.${NC}"
    else
        echo -e "  ${RED}[WARNING] Another process is using port ${PORT}.${NC}"
        if ! prompt_yes_no "  Continue anyway? [y/N]" "n"; then
            exit 1
        fi
    fi
else
    echo -e "  ${GREEN}✓ Port :${PORT} is free and ready${NC}"
fi

# ------------------------------------------------------------------------------
# 5. Installing Binary & Configuring Directories
# ------------------------------------------------------------------------------
echo -e "${BLUE}[5/8] Installing binary and configuring storage directories...${NC}"
mkdir -p "${BIN_DIR}"
cp -f "${SOURCE_BIN}" "${BIN_DIR}/dbvault"
chmod 0755 "${BIN_DIR}/dbvault"
echo -e "  ${GREEN}✓ Installed executable to ${BIN_DIR}/dbvault${NC}"

# Create data directory with secure permissions
mkdir -p "${DATA_DIR}"
chmod 0700 "${DATA_DIR}"
echo -e "  ${GREEN}✓ Created vault data directory: ${DATA_DIR} (mode 0700)${NC}"

# ------------------------------------------------------------------------------
# 6. Setting Up Systemd Service
# ------------------------------------------------------------------------------
echo -e "${BLUE}[6/8] Configuring systemd service...${NC}"

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
echo -e "  ${GREEN}✓ Systemd service configured and started (dbvault.service)${NC}"

# ------------------------------------------------------------------------------
# 7. Verifying Daemon Readiness & Health Probe
# ------------------------------------------------------------------------------
echo -e "${BLUE}[7/8] Confirming DBVault health and API readiness...${NC}"

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
    exit 1
fi
echo -e "  ${GREEN}✓ Health check passed (HTTP 200 from http://127.0.0.1:${PORT}/health)${NC}"

# ------------------------------------------------------------------------------
# 8. Firewall Configuration & Setup Token Extraction
# ------------------------------------------------------------------------------
echo -e "${BLUE}[8/8] Checking firewall and retrieving initial setup token...${NC}"

# Firewall check
if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
    ufw allow "${PORT}/tcp" >/dev/null 2>&1 || true
    echo -e "  ${GREEN}✓ UFW firewall rule added for port ${PORT}/tcp${NC}"
elif command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld; then
    firewall-cmd --add-port="${PORT}/tcp" --permanent >/dev/null 2>&1 || true
    firewall-cmd --reload >/dev/null 2>&1 || true
    echo -e "  ${GREEN}✓ Firewalld rule added for port ${PORT}/tcp${NC}"
fi

# Detect Server IP
PUBLIC_IP=$(curl -s --max-time 3 https://ifconfig.me 2>/dev/null || curl -s --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}')
PUBLIC_IP="${PUBLIC_IP:-127.0.0.1}"

# Extract Setup Token from journalctl
SETUP_TOKEN=$(journalctl -u dbvault -n 100 --no-pager 2>/dev/null | grep -i "setup token:" | tail -n 1 | sed 's/.*setup token: //I' | tr -d '\r')

# ------------------------------------------------------------------------------
# Installation Summary
# ------------------------------------------------------------------------------
echo ""
echo -e "${BOLD}${GREEN}==================================================================${NC}"
echo -e "${BOLD}${GREEN}        ✓ DBVault has been successfully installed & verified!    ${NC}"
echo -e "${BOLD}${GREEN}==================================================================${NC}"
echo ""
echo -e "${BOLD}1. Web Console Access URL:${NC}"
echo -e "   ${CYAN}http://${PUBLIC_IP}:${PORT}${NC}"
echo ""
if [[ -n "$SETUP_TOKEN" ]]; then
    echo -e "${BOLD}2. Appliance First-Run Setup Token:${NC}"
    echo -e "   ${YELLOW}${BOLD}${SETUP_TOKEN}${NC}"
    echo -e "   ${NC}(Paste this token into the web browser to create your admin account)${NC}"
else
    echo -e "${BOLD}2. Appliance Setup Token:${NC}"
    echo -e "   If an admin account is already created, sign in at http://${PUBLIC_IP}:${PORT}/login"
    echo -e "   To view logs: journalctl -u dbvault -f"
fi
echo ""
echo -e "${BOLD}3. System Service Management:${NC}"
echo "   Status:  sudo systemctl status dbvault"
echo "   Logs:    sudo journalctl -u dbvault -f"
echo "   Restart: sudo systemctl restart dbvault"
echo "   Stop:    sudo systemctl stop dbvault"
echo ""
echo "=================================================================="
