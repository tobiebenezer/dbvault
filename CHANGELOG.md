# Changelog

All notable changes to DBVault will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2026-09-05

### Added
- **Automated VPS Installation Script (`scripts/install-vps.sh`)**:
  - One-liner curl installation support (`curl -fsSL https://raw.githubusercontent.com/tobiebenezer/dbvault/main/scripts/install-vps.sh | sudo bash`).
  - Native port configuration defaulting to port **2633** (override via `DBVAULT_PORT`).
  - Production `systemd` daemon management (`dbvault.service`) with auto-start, crash recovery, and journal logging.
  - Pre-flight system checks for OS architecture (`amd64`, `arm64`), core utilities (`curl`, `gzip`, `tar`), and interactive database dump tool resolution (`mysqldump`, `pg_dump`, `sqlite3`).
  - Multi-distro firewall rule configuration supporting both `ufw` and `firewalld`.
  - HTTP readiness health polling against `/health` before finishing installation.
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
- Makefile updated with `install-vps` and `package-vps` targets.
- Documentation refreshed with exact repository paths (`tobiebenezer/dbvault`) and download commands.

---

## [0.1.0] - 2026-08-30

### Added
- Initial alpha architecture for DBVault appliance.
- Multi-engine database driver seams (PostgreSQL, MySQL/MariaDB, SQLite).
- Cryptographic chunking, AEAD encryption, and Ed25519 manifest signing.
- Evidence-backed Parquet lakehouse synchronization and DuckDB analytics engine.
- Embedded single-binary React web console.
- Point-in-time recovery (PITR) foundations and sandbox verification drills.
