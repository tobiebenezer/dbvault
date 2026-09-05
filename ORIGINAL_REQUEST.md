# Original User Request

## 2026-08-30T19:13:00Z

DBVault is a self-contained Go appliance for self-hosted database backup and cryptographically verified recovery, engineered for DevOps professionals who need production-grade backup automation, unbroken point-in-time recovery (PITR), ransomware protection, and automated restore drills without paying for fully managed cloud database services.

Working directory: `/home/sifu/Documents/dbvault`
Integrity mode: development

## Requirements

### R1. Production Multi-Engine Backup & Restore Engine
Implement and validate real backup and restore pipelines for PostgreSQL (logical dump and physical basebackup with streaming WAL archiving), MySQL/MariaDB (dump and binlog streaming), and SQLite (safe online WAL backup). Support streaming envelope encryption (AES-256-GCM) and chunked uploads directly to user-owned object storage (Cloudflare R2, AWS S3, MinIO, Contabo).

### R2. Cryptographic Tamper-Evident Manifests & Immutable Storage (WORM)
Construct deterministic Merkle tree manifests over all backup chunks and WAL log segments with Ed25519 cryptographic signing. Provide support for S3 Object Lock (WORM / Write Once Read Many) compliance and governance retention modes to prevent tampering or ransomware deletion of backups.

### R3. Automated Sandbox Restore Drills & Verification Receipts
Implement an automated restore verification loop that executes recurring drills inside isolated staging environments (ephemeral containers or isolated test instances). Each drill must perform deep database consistency verification (e.g. `pg_amcheck`, page checksum validation, `CHECK TABLE`, row count reconciliation) and generate an immutable, signed recovery certificate detailing exact RTO and RPO metrics.

### R4. Continuous PITR with Unbroken LSN Timeline & Gap Detection
Implement real-time continuous WAL and binlog replication with proactive log sequence number (LSN) verification. Detect missing or corrupt log segments immediately, alerting when the recovery timeline has gaps and guaranteeing point-in-time recovery to any specified second within the retention window.

### R5. Self-Contained Emergency Recovery Bundle
Provide a zero-dependency emergency restore capability that can extract, verify, and restore full databases directly from storage bucket credentials and encrypted manifests even when the primary DBVault appliance server is unavailable.

## Acceptance Criteria

### Engine & Storage Integration
- [ ] End-to-end backup, corruption injection, and restore tests pass against live PostgreSQL, MySQL/MariaDB, and SQLite instances with zero data corruption.
- [ ] Direct streaming backup and restore operations pass contract tests against S3-compatible endpoints (Cloudflare R2, MinIO, AWS S3).

### Integrity & Tamper Detection
- [ ] Any corrupted or modified backup chunk or manifest is cryptographically detected and rejected via Merkle root verification before database restoration begins.
- [ ] Signed manifests verify correctly using public key verification without downloading full chunk payloads.

### Automated Restore Drills & Metrics
- [ ] Scheduled restore drills run unattended in isolated sandboxes, execute automated consistency checks (`pg_amcheck` / table checks), and output structured verification receipts with measured RTO and RPO.
- [ ] Simulated backup failure or data corruption triggers an immediate alert and flags the recovery status as degraded.

### Point-in-Time Recovery
- [ ] Restoration to an arbitrary target timestamp within the WAL/binlog retention window completes successfully with exact state verification at that timestamp.

### Emergency Recovery Tool
- [ ] Standalone restore utility can recover a full database instance directly from storage credentials without requiring an active DBVault server daemon.
