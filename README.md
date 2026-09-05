# DBVault

**Know that you can recover.**

DBVault is a self-contained Go database backup and verified-recovery appliance. It is designed to install as one binary, serve its own web console, connect databases to user-owned storage, run backups, run restore drills, and show whether a database is actually protected.

> **Alpha status:** DBVault is currently a v0.1-alpha foundation. It is suitable for demos, local development, staging tests, and controlled pilots. Do not use it as the only backup system for irreplaceable production data yet.

---

## What DBVault is trying to become

```text
One Go binary
    ↓
Embedded web UI
    ↓
Backup and restore engine
    ↓
Agent and control-plane foundations
    ↓
User-owned storage such as R2, Contabo, S3 or MinIO
    ↓
Restore drills
    ↓
Clear protection status
```

The product goal is not merely to create backups. The goal is to prove recovery.

## Automated VPS Installation (Port 2633)

DBVault is built to run natively as a hardened `systemd` background service on any standard Linux VPS (Ubuntu, Debian, CentOS, AlmaLinux, Rocky). By default, it listens on port **2633**.

### Option A: One-Liner Web Installer (Fastest)

Run this single command on your VPS to automatically download, install, and start DBVault:

```bash
curl -fsSL https://raw.githubusercontent.com/tobiebenezer/dbvault/main/scripts/install-vps.sh | sudo bash
```

*To specify a custom port (e.g. 2633 is default, or another custom port):*
```bash
curl -fsSL https://raw.githubusercontent.com/tobiebenezer/dbvault/main/scripts/install-vps.sh | sudo DBVAULT_PORT=2633 bash
```

### Option B: Pre-built Release Package (Manual or Air-Gapped)

1. **Download the latest release tarball**:
   ```bash
   curl -fsSLO https://github.com/tobiebenezer/dbvault/releases/latest/download/dbvault-vps-installer.tar.gz
   ```
   *(Or build your own bundle locally with `make package-vps` and upload to your VPS)*

2. **Extract and run automated installer**:
   ```bash
   mkdir -p dbvault-installer && tar -xzf dbvault-vps-installer.tar.gz -C dbvault-installer
   cd dbvault-installer
   sudo ./install.sh
   ```

### Option C: If You Cloned the Repo on the VPS

```bash
make install-vps
```

---

## Updating to a New Release

DBVault supports seamless in-place updates. Upgrading replaces the executable and restarts the background service while **preserving all existing databases, credentials, AEAD AES-256 master keys, and backup schedules** in `/var/lib/dbvault`.

### 1. Check if an Update is Available
```bash
sudo dbvault-update --check
# or
dbvault update --check
```

### 2. Apply the Latest Release (In-Place Upgrade)
Run the dedicated updater:
```bash
sudo dbvault-update
```
Or re-run the web installer (it automatically detects an existing installation and applies the update safely):
```bash
curl -fsSL https://raw.githubusercontent.com/tobiebenezer/dbvault/main/scripts/install-vps.sh | sudo bash
```

> **Safe Rollback**: Every update automatically creates a backup of your previous binary at `/usr/local/bin/dbvault.bak` and verifies health check probes before committing.

---

### What the Automated Installer Confirms for You

When you run `install.sh`, it walks through an automated pre-flight checklist:
- **Root & Architecture**: Verifies root/sudo permissions and detects 64-bit Linux (`amd64` / `arm64`).
- **Core Utilities**: Confirms `curl`, `gzip`, and `tar` are installed (auto-installs them if missing).
- **Database Dump Clients**: Checks for `mysqldump`, `pg_dump`, and `sqlite3`, offering one-click installation for missing tools.
- **Port Availability**: Confirms port **2633** is free and ready.
- **Daemon Setup**: Writes and enables `/etc/systemd/system/dbvault.service` with auto-restart on boot and crash recovery.
- **Readiness Probe**: Polls `http://127.0.0.1:2633/health` until DBVault returns `200 OK`.
- **Firewall Rules**: Automatically detects and opens port `2633` in `ufw` or `firewalld`.
- **Setup Token & Access URL**: Prints your Web Console link and one-time Setup Token right in your terminal.

---

## Security: Setup Token vs. Master Encryption Key

DBVault uses a two-tier security model to protect your server and your database backups:

### 1. The Setup Token (Appliance Ownership)
* **What it does**: When you launch DBVault on a public VPS, anyone scanning your IP could attempt to open port 2633. The Setup Token is a one-time secret printed to your SSH terminal during installation. It guarantees that **only you** (the person with SSH access to the machine) can claim ownership and create the administrator account.
* **How to find it**: Printed at the end of `./install.sh`, or retrieved anytime with:
  ```bash
  sudo journalctl -u dbvault --no-pager | grep "setup token:"
  ```
* **Lifespan**: Used once on the first-run web screen, then immediately invalidated.

### 2. The Master Encryption Key (Data Protection)
* **What it does**: DBVault operates on zero-knowledge encryption. Every snapshot is compressed with gzip and encrypted with **AEAD AES-256-GCM** using this key before being written to disk or uploaded to Cloudflare R2. Even if your cloud storage bucket is compromised, no one can read your raw data without this key.
* **How to retrieve and back it up**:
  - **In the Web Console**: Go to **Settings** → **Master Key & DR Kit** → **View & Backup Master Key**. Download the Emergency Disaster Recovery sheet for your password manager (1Password / Bitwarden).
  - **On the VPS Disk**: Stored with strict `0600` permissions at `/var/lib/dbvault/master.key`.
* **Lifespan**: Permanent. Save a copy in your password manager!

---

## Fast demo

```bash
make web-build
CGO_ENABLED=0 go build -tags=restricted -o bin/dbvault ./cmd/dbvault
bin/dbvault server --demo --listen 127.0.0.1:8080 --data-dir /tmp/dbvault-demo
```

Open:

```text
http://127.0.0.1:8080
```

---

## Docker demo

```bash
docker compose -f deploy/compose/demo.yml up --build
```

Open:

```text
http://127.0.0.1:8080
```

Docker is optional. Native systemd remains the preferred path for early real VPS pilots because DBVault may need local filesystem, socket and native database-tool access.

---

## Alpha smoke test

```bash
make alpha-smoke
```

This verifies the install layout, embedded UI, overview API, protection API, recovery timeline, doctor, policy simulation, support bundle and recovery bundle routes.

For the broader alpha check:

```bash
make alpha-check
```

---

## Install-layout test

This does not touch the host system:

```bash
bin/dbvault install \
  --root /tmp/dbvault-install \
  --domain backup.example.test \
  --self-signed
```

Inspect:

```text
/tmp/dbvault-install/etc/dbvault/dbvault.yaml
/tmp/dbvault-install/etc/systemd/system/dbvault.service
/tmp/dbvault-install/var/lib/dbvault/setup-token
```

---

## Current install paths

| Path | Status | Use for |
|---|---:|---|
| `scripts/install-vps.sh` (port 2633) | Ready | Production VPS daemon (Ubuntu, Debian, CentOS, AlmaLinux) |
| `make package-vps` | Ready | 3.3MB self-contained offline installer archive |
| `dbvault server --demo` | Works | Product demo and UI exploration |
| `dbvault install --root ...` | Works | Safe install-layout testing |
| Docker Compose demo | Works when Docker is available | Contributors and demos |
| Kubernetes | Scaffolded | Future platform work |

---

## Current product state

Implemented foundations include:

- Core SQLite backup/restore components
- Repository, chunking, encryption and manifest foundations
- Multi-engine PostgreSQL/MySQL/MariaDB seams
- PITR and log-collection domain scaffolding
- Tenant/control-plane/agent scaffolding
- Kubernetes CRD/operator scaffolding
- Self-contained Go appliance server
- Embedded source-backed web console
- Setup token flow
- Product-experience APIs
- Protection-status scaffold
- Recovery timeline scaffold
- Preflight doctor scaffold
- Policy simulation scaffold
- Alerts scaffold
- Sandbox restore lifecycle scaffold
- Support and recovery bundle scaffolds
- Warehouse & analytics: evidence-backed Parquet lakehouse sync (full + watermark incremental), governed query/export and scoped BI feed
- Docker demo packaging
- Alpha runbook and smoke tests

---

## What is not production-ready yet

The following must be completed before DBVault can protect real customer data as a primary backup system:

- Production dependency download and `go.sum` generation
- CGO SQLite production validation
- Real Cloudflare R2 contract tests
- Real Contabo contract tests
- Real PostgreSQL dump/restore tests
- Real MySQL/MariaDB dump/restore tests
- Real restore drills connected to catalogue evidence
- Protection status calculated from real backup/restore evidence
- Recovery timeline calculated from real snapshot and WAL/binlog history
- Browser automation with Playwright or equivalent
- Accessibility automation
- Installer and agent-enrolment security review
- Signed releases, SBOM and vulnerability scan

---

## Useful commands

```bash
make package-vps      # Build standalone 3.3MB Linux VPS installer bundle
make install-vps      # Build and run automated installer on current machine
make web-test
make web-build
make test-restricted
make build-restricted
make test-phase8c
make alpha-smoke
make alpha-package
```

Production profile, in an internet-enabled environment:

```bash
./scripts/bootstrap-production-deps.sh
make build
make test-race
```

---

## Documentation

Start here:

- `docs/alpha/ALPHA_0_1_RUNBOOK.md`
- `docs/alpha/ALPHA_0_1_CHECKLIST.md`
- `docs/alpha/INSTALL.md`
- `docs/alpha/DOCKER.md`
- `docs/phase8/self-contained-appliance.md`
- `docs/product/outstanding-product-experience.md`
- `docs/configuration/connection-wiring.md`
- `docs/warehouse.md`

---

## Configuration model

The runtime graph is explicit:

```text
source.repository
  -> repository.primary / mirrors / replicas
  -> destination
  -> secret provider references
```

Useful commands:

```bash
dbvault config validate --config /etc/dbvault/dbvault.yaml
dbvault config print-effective --config /etc/dbvault/dbvault.yaml
dbvault source test --config /etc/dbvault/dbvault.yaml --id application-postgres
dbvault destination test --config /etc/dbvault/dbvault.yaml --id r2-primary
dbvault repository explain --config /etc/dbvault/dbvault.yaml --id production
```

---

## Product promise

Backups are promises. DBVault tests the promise.

One Go binary. Your database. Your storage. Verified restores.

