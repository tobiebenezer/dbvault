# DBVault Install Guide

## Automated VPS Installation (Recommended)

To install DBVault as a hardened `systemd` daemon on your Linux VPS (Ubuntu/Debian/CentOS/AlmaLinux):

### 1. One-Liner Web Installer (Fastest)
```bash
curl -fsSL https://raw.githubusercontent.com/tobiebenezer/dbvault/main/scripts/install-vps.sh | sudo bash
```

Custom port override (default is **2633**):
```bash
curl -fsSL https://raw.githubusercontent.com/tobiebenezer/dbvault/main/scripts/install-vps.sh | sudo DBVAULT_PORT=2633 bash
```

### 2. Standalone Release Package Download
Download the pre-compiled installer bundle from GitHub releases:
```bash
curl -fsSLO https://github.com/tobiebenezer/dbvault/releases/latest/download/dbvault-vps-installer.tar.gz
mkdir -p dbvault-installer && tar -xzf dbvault-vps-installer.tar.gz -C dbvault-installer
cd dbvault-installer
sudo ./install.sh
```

---

## Demo mode

```bash
make web-build
CGO_ENABLED=0 go build -tags=restricted -o bin/dbvault ./cmd/dbvault
bin/dbvault server --demo --listen 127.0.0.1:8080 --data-dir /tmp/dbvault-demo
```

Open:

```text
http://127.0.0.1:8080
```

## Install layout dry run

```bash
bin/dbvault install --root /tmp/dbvault-install --domain backup.example.test --self-signed
```

This creates the same files a real system install would create, but under `/tmp/dbvault-install`.

## Real install direction

The intended production command is:

```bash
sudo dbvault install --domain backup.example.com
```

For alpha testing, use `--root` first and inspect the generated files before installing on a real host.

## Uninstall plan

```bash
dbvault uninstall
```

By default, uninstall must preserve `/etc/dbvault` and `/var/lib/dbvault`.

A destructive purge requires an explicit purge flag.
