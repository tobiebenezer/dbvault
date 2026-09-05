# Project: DBVault

## Architecture
DBVault is a production-grade Go appliance for multi-database backup, cryptographic verification, and point-in-time recovery (PITR).
It follows a Hexagonal Architecture (Ports and Adapters):
- **Core Domain**: Models for Backup, Manifest, MerkleTree, VerificationCertificate, TransactionLog (WAL/Binlog), RecoveryChain, Timeline, ImmutabilityPolicy.
- **Application Services**:
  - `backup`: Orchestrates database capture, chunking, deduplication, encryption, and manifest publication.
  - `restore`: Reconstructs database files from encrypted CAS chunks and manifests.
  - `verification`: Validates Ed25519 signatures, Merkle trees, and chunk integrity.
  - `pitr`: Replays base backups and continuous transaction logs up to a specific timestamp/LSN.
  - `restoredrill`: Executes scheduled unattended restore drills in isolated sandboxes and measures RTO/RPO.
  - `logcollection`: Ingests and monitors WAL/binlog streams, detecting gaps and alerting degraded state.
- **Adapters**:
  - `source`: PostgreSQL (`pg_dump`, `pg_basebackup`, `pg_receivewal`), MySQL/MariaDB (`mysqldump`, `mysqlbinlog`), SQLite (`sqlite3_backup_*`, `VACUUM INTO`).
  - `encryption`: AEAD AES-256-GCM with HKDF key derivation and authenticated data.
  - `manifest`: Deterministic Merkle tree generator and PureEd25519 signer/verifier.
  - `storage`: S3-compatible object storage (AWS S3, Cloudflare R2, MinIO, Contabo) with S3 Object Lock (WORM).
  - `probes`: Deep consistency checkers (`pg_amcheck`, `pg_checksums`, MySQL `CHECK TABLE`, SQLite `quick_check`, row count reconciliation).
- **Entry Points**:
  - `cmd/dbvault`: Main appliance server daemon and administrative CLI.
  - `cmd/dbvault-restore`: Zero-dependency standalone emergency restore tool.

## Feature Inventory
Every feature discovered in the Survey phase across Requirements R1 to R5 is enumerated here with its milestone assignment.

| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | PostgreSQL Logical Dump & Restore | Streamed `pg_dump` custom/tar/sql formats, user/password auth, globals extraction | M2 | R1 (Survey) |
| 2 | PostgreSQL Physical Basebackup | `pg_basebackup` with tar streaming, tablespace support, WAL method handling | M2 | R1 (Survey) |
| 3 | PostgreSQL WAL Streaming & Ingestion | Real-time `pg_receivewal` and archive_command segment collection, regex validation | M2 | R1 (Survey) |
| 4 | MySQL / MariaDB Dump & Restore | `mysqldump` / `mariadb-dump` with single-transaction, quick, routines, triggers, events | M2 | R1 (Survey) |
| 5 | MySQL Binlog Streaming & Ingestion | Real-time binlog stream capture, format validation, server ID scoping | M2 | R1 (Survey) |
| 6 | SQLite Online Safe Backup | Cgo `sqlite3_backup_*` API with step/busy delay, `VACUUM INTO`, WAL snapshotting | M2 | R1 (Survey) |
| 7 | Streaming AEAD AES-256-GCM Encryption | Envelope encryption with random DEK, HKDF key derivation, domain-separated AAD | M1 | R1 (Survey) |
| 8 | S3-Compatible CAS Chunked Uploads | Multipart chunked streaming uploads to Cloudflare R2, MinIO, AWS S3, Contabo | M1 | R1 (Survey) |
| 9 | Deterministic Merkle Tree Generation | Domain-separated HMAC-SHA256 leaf and root digest computation over chunks/WAL | M1 | R2 (Survey) |
| 10 | PureEd25519 Manifest Signing & Verification | Public-key signing of Merkle root; verification without downloading full chunk payloads | M1 | R2 (Survey) |
| 11 | S3 Object Lock WORM Immutability | Compliance and Governance retention modes, legal holds, LockExpiresAt enforcement | M1 | R2 (Survey) |
| 12 | Two-Phase Atomic Manifest Commit | Atomic publishing of `manifest.json`, `manifest.sig`, and `complete.json` | M1 | R2 (Survey) |
| 13 | Ephemeral Sandbox Isolation | Isolated staging directories/ports/containers for restore drills | M4 | R3 (Survey) |
| 14 | PostgreSQL `pg_amcheck` & Checksum Probes | B-tree index, heap block, and page checksum corruption verification | M4 | R3 (Survey) |
| 15 | MySQL `CHECK TABLE` Probes | Extended table check, index consistency, and corruption verification | M4 | R3 (Survey) |
| 16 | SQLite Integrity & Quick Check Probes | `PRAGMA integrity_check` and `PRAGMA quick_check` verification | M4 | R3 (Survey) |
| 17 | Row Count & Schema Reconciliation | Pre/post restore schema hash and table row count reconciliation | M4 | R3 (Survey) |
| 18 | PII Privacy Masking Pipeline | Automatic redaction of sensitive columns during sandbox drills | M4 | R3 (Survey) |
| 19 | Ed25519-Signed Recovery Certificate | Immutable verification receipt with exact measured RTO and RPO metrics | M4 | R3 (Survey) |
| 20 | Continuous LSN & GTID Timeline Tracking | Real-time log sequence number (Postgres LSN) and MySQL GTID interval continuity checks | M3 | R4 (Survey) |
| 21 | Timeline Branch & History Switch Handling | PostgreSQL multi-timeline `.history` file management and timeline switches | M3 | R4 (Survey) |
| 22 | Instant Gap & Corruption Alerting | Detection of missing or corrupted log segments, triggering immediate degraded alert | M3 | R4 (Survey) |
| 23 | Target Timestamp PITR Replay | Point-in-time reconstruction to arbitrary target timestamp within retention window | M3 | R4 (Survey) |
| 24 | Target LSN / Recovery Point Replay | Exact state reconstruction to designated LSN or named recovery point | M3 | R4 (Survey) |
| 25 | Zero-Dependency Emergency Restore Utility | Standalone `dbvault-restore` binary executing without active DBVault daemon | M5 | R5 (Survey) |
| 26 | Standalone S3 & Decryption in Emergency Tool | Direct object storage credential extraction, HKDF key derivation, AES-256-GCM decryption | M5 | R5 (Survey) |
| 27 | Standalone Merkle & Signature Validation | Complete cryptographic verification in emergency utility prior to restore write | M5 | R5 (Survey) |
| 28 | Comprehensive E2E Test Suite (Tiers 1-4) | Opaque-box automated test runner against live Postgres, MySQL, SQLite, and MinIO | Testing Track | Acceptance Criteria |
| 29 | Adversarial Coverage Hardening (Tier 5) | White-box stress tests, chunk bit-flip corruption injection, timeline split attacks | M6 / Testing Track | Acceptance Criteria |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| E2E | E2E Testing Track | Complete opaque-box test runner and test cases across Tiers 1-4; produces `TEST_READY.md` | none | DONE |
| M1 | Crypto & Storage Foundation | AES-256-GCM envelope encryption, Merkle tree manifests, Ed25519 signing/verification, S3 CAS storage & Object Lock | none | DONE |
| M2 | Multi-Engine Backup & Restore | PostgreSQL (dump/basebackup/WAL), MySQL (dump/binlog), SQLite (online backup/WAL) | M1 | PLANNED |
| M3 | Continuous PITR & Gap Detection | LSN/GTID tracking, continuous replication, gap detection & degraded alerting, timestamp-based replay | M1, M2 | PLANNED |
| M4 | Automated Sandbox Restore Drills & Verification | Sandbox execution, deep consistency probes (`pg_amcheck`, `CHECK TABLE`, row count), signed RTO/RPO receipts | M1, M2 | PLANNED |
| M5 | Zero-Dependency Emergency CLI | Standalone `dbvault-restore` tool operating directly from bucket credentials | M1, M2, M3 | PLANNED |
| M6 | E2E Verification & Adversarial Hardening | 100% pass on Tiers 1-4 E2E tests, corruption injection, and Tier 5 adversarial hardening | E2E, M1-M5 | PLANNED |

## Interface Contracts
### Crypto & Manifest ↔ Engines & Storage (`M1 ↔ M2, M3, M4, M5`)
- `ports.Encryptor`: `EncryptChunk(id string, plaintext []byte) ([]byte, error)`, `DecryptChunk(id string, ciphertext []byte) ([]byte, error)`
- `ports.ManifestSigner`: `Sign(manifest *domain.Manifest) (*domain.SignedManifest, error)`, `Verify(signedManifest *domain.SignedManifest, pubKey []byte) (bool, error)`
- `ports.ObjectStore`: `PutObject(ctx, key, reader, size, lockOpts)`, `GetObject(ctx, key)`, `HeadObject(ctx, key)`, `ListObjects(ctx, prefix)`

### Backup & Restore ↔ Database Engines (`M2 ↔ M3, M4, M5`)
- `ports.DatabaseDriver`: `Inspect(ctx) (*domain.DatabaseInfo, error)`, `CreateBackupStream(ctx, opts) (io.ReadCloser, error)`, `RestoreBackupStream(ctx, reader, opts) error`
- `ports.LogCollector`: `CollectLogs(ctx, fromLSN) (<-chan domain.TransactionLog, error)`, `VerifyLogContinuity(logs []domain.TransactionLog) (*domain.TimelineStatus, error)`

### Consistency Probes ↔ Restore Drills (`M4 ↔ M2`)
- `ports.ConsistencyProbe`: `RunConsistencyChecks(ctx, connStr) (*domain.ConsistencyReport, error)`
  - PostgreSQL: verifies page checksums (`pg_checksums`) and runs `pg_amcheck -c -a`
  - MySQL: runs `CHECK TABLE <all_tables> EXTENDED`
  - SQLite: runs `PRAGMA integrity_check` and `PRAGMA quick_check`
  - All: reconciles table schemas and row count sums

### Emergency Restore CLI Contract (`M5`)
- Binary: `dbvault-restore --endpoint <s3_url> --bucket <bucket> --access-key <key> --secret-key <secret> --manifest-key <path> --master-key-hex <hex> --target-dir <dir> --verify-only`
- Zero external runtime dependency (pure Go static executable, no daemon connection).

## Code Layout
- `cmd/dbvault/`: CLI and server entry points.
- `cmd/dbvault-restore/`: Standalone zero-dependency emergency restore CLI.
- `internal/domain/`: Core entities, contracts, error types.
- `internal/application/`:
  - `backup/`, `restore/`, `verification/`, `pitr/`, `pitrdrill/`, `restoredrill/`, `logcollection/`, `recoverywindow/`
- `internal/adapters/`:
  - `encryption/aead/`: AES-256-GCM envelope encryption and key derivation.
  - `manifest/ed25519/`: PureEd25519 signing and envelope serialization.
  - `storage/s3/`: S3 multipart uploads, CAS deduplication, and Object Lock.
  - `source/postgres/`, `source/mysql/`, `source/sqlite/`: Engine adapters and log collectors.
  - `probes/`: Deep consistency checkers (`pg_amcheck`, `CHECK TABLE`, row reconciler).
- `test/e2e/`: E2E test suite (Tiers 1-4, test runner, corruption injection harnesses).
