# Changelog

All notable changes to DBVault will be documented in this file.

## [0.2.3] - 2026-09-06

### Added
- **Autonomous privilege elevation for PostgreSQL backups (opt-in)**: databases can now configure a separate "elevation role" in the console (Databases → database → Tables & Storage Footprint → Privilege Elevation). When `pg_dump` fails with `permission denied for table/sequence/schema ...` and elevation is enabled, DBVault autonomously grants the backup role read access via the elevation role, retries the dump once, and **always revokes the grants afterwards** — including when the dump fails again, with 3 revoke retries and a loud security log if revocation cannot be confirmed. The denied-object list comes from a full privilege audit (tables + sequences + schemas, not just the parsed error); identifiers are sanitized (only plain, unquoted names are granted) and the elevation role must differ from the backup role. Without elevation configured, the manual remediation hint from 0.2.2 still applies.
- Elevation config is stored per database in `custom_elevation.json` (0600, alongside other secrets) with a dedicated API (`GET`/`POST /api/v1/databases/{id}/elevation`; the GET never returns the stored password, and a blank password on re-save keeps the stored one).

---

## [0.2.2] - 2026-09-06

### Fixed
- **PostgreSQL backup failures now explain the fix**: when `pg_dump` fails with `permission denied for table ...`, DBVault no longer surfaces only the raw error. Both live-console backups and core-engine snapshots now run a read-only privilege audit against the database, list every table the backup role cannot SELECT from, and show copy-paste remediation SQL (`GRANT USAGE/SELECT`, `ALTER DEFAULT PRIVILEGES`) with the actual role name. On managed hosting where grants are impossible, the message points to the database's table-exclusion setting instead. Non-permission failures keep the original error message unchanged.

---

## [0.2.1] - 2026-09-06

### Fixed
- **Storage connection test jobs no longer hang in pending**: `destination_test` jobs queued from the web console in production (live) mode had no worker handler, so R2/S3 connection tests stayed pending forever. Tests now run the real endpoint probe (bucket validation, canary put/get/list/delete), progress through `probe → put → get → delete → complete` stages, and complete or fail with the actual storage error.
- Destination test outcomes now update the destination's health status (`healthy`/`error`) and last-checked timestamp in the console.
- Env-configured storage destinations (`DBVAULT_R2_*`, `R2_*`, `AWS_*`) are now probed via the queue when no UI-saved configuration exists.

---

## [0.2.0] - 2026-09-05

### Added
- **Automated VPS Installation & Safe In-Place Updater (`scripts/install-vps.sh`)**:
  - One-liner curl installation support (`curl -fsSL https://raw.githubusercontent.com/tobiebenezer/dbvault/main/scripts/install-vps.sh | sudo bash`).
  - Safe in-place update engine: automatically detects existing installations, compares versions against GitHub releases, and performs atomic binary upgrades while preserving all existing databases, Master Keys, and schedules in `/var/lib/dbvault`.
  - Automatic creation of `/usr/local/bin/dbvault.bak` rollback safety copy before applying updates.
  - Dedicated `dbvault-update` helper script installed to `/usr/local/bin/dbvault-update` (supports `sudo dbvault-update` and `--check`).
  - CLI `dbvault version` and `dbvault update --check` integration with live GitHub release inspection.
  - Native port configuration defaulting to port **2633** (override via `DBVAULT_PORT`).
  - Production `systemd` daemon management (`dbvault.service`) with auto-start, crash recovery, and journal logging.
  - Pre-flight system checks for OS architecture (`amd64`, `arm64`), core utilities (`curl`, `gzip`, `tar`), and interactive database dump tool resolution (`mysqldump`, `pg_dump`, `sqlite3`).
  - Multi-distro firewall rule configuration supporting both `ufw` and `firewalld`.
  - HTTP readiness health polling against `/health` before finishing installation or update.
  - Automated detection and terminal output of appliance Setup Token and Web Console access URL.
- **Standalone VPS Deployment Bundler (`scripts/package-vps-bundle.sh`)**:
  - `make package-vps` target compiling standalone Linux binary with embedded web UI and packaging it into `dist/dbvault-vps-installer.tar.gz`.
- **Hardened Recurring Scheduler Engine**:
  - Implemented standard 5-field cron parsing with support for cron descriptors (`@hourly`, `@daily`, `@weekly`) and step expressions (`*/N`).
  - Atomic disk persistence (`s.savePersistedDataLocked()`) for schedule mutations (`CreateSchedule`, `UpdateSchedule`, `DeleteSchedule`, `TriggerScheduleNow`).
  - Active background scheduler auto-runner (`StartSchedulerTicker`) evaluating registered schedules on a 15-second loop.
  - Automatic backup job dispatch and execution tracking (`healthy` upon completion, `failed` on error).
- **Zero-Knowledge Cloudflare R2 / S3 Production Storage**:
  - Integrated production AWS SDK S3 client supporting Cloudflare R2, MinIO, Contabo, and AWS S3 object storage.
  - High-performance gzip stream compression prior to encryption.
  - End-to-end AEAD AES-256-GCM snapshot encryption and decryption.
  - Live MySQL database snapshot creation, cryptographic chunking, and direct cloud bucket synchronization.
- **Brand Identity & Visual Design**:
  - Integrated "The Infinity Core" custom 3D isometric dark-mode logo across the platform.
  - Embedded logo assets in Web SPA header, authentication login screen, setup admin wizard, and browser favicon.
- **Restore Safety & Availability Controls**:
  - Fail-safe restore locking preventing execution when no backups exist for a database.
  - Clear UI guidance and disabled action states when database protection has not yet been initialized.
- **Appliance Security & DR Recovery**:
  - Two-tier security model distinguishing appliance ownership claim (one-time Setup Token) from data encryption (AES-256 Master Key).
  - Disaster Recovery kit export with downloadable emergency recovery sheet.

### Changed
- Production build flags now enable full cloud storage adapters by default.
- Published GitHub release `v0.2.0` with downloadable assets (`dbvault-linux-amd64`, `dbvault-vps-installer.tar.gz`) so the one-liner VPS installer resolves binaries from GitHub Releases.
- Makefile updated with `install-vps` and `package-vps` targets.
- Documentation refreshed with exact repository paths (`tobiebenezer/dbvault`) and download commands.
- Streamlined all 14 web console views: removed redundant descriptions, explanatory sub-texts, and card subtitles for a clean, minimalist UI.
- Sanitized repository documentation: pruned 49 obsolete development phase reports, milestone checklists, and temporary session logs.

---

## [0.1.0] - 2026-08-30

### Added
- Initial alpha architecture for DBVault appliance.
- Multi-engine database driver seams (PostgreSQL, MySQL/MariaDB, SQLite).
- Cryptographic chunking, AEAD encryption, and Ed25519 manifest signing.
- Evidence-backed Parquet lakehouse synchronization and DuckDB analytics engine.
- Embedded single-binary React web console.
- Point-in-time recovery (PITR) foundations and sandbox verification drills.
