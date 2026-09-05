# DBVault E2E Test Infrastructure & Test Architecture

This document defines the End-to-End (E2E) Test Architecture, test runner invocations, pass/fail semantics, and verification framework for DBVault across all 29 system features specified in `PROJECT.md`.

---

## 1. Test Architecture Overview

DBVault's testing framework is structured into a rigorous 4-Tier hierarchical architecture:

```
+-----------------------------------------------------------------------------+
|                         Tier 4: Real-World Scenarios                        |
|  Live PostgreSQL (5432), Live MySQL (3306), SQLite 3, S3/MinIO WORM Mock,   |
|  Full Backup/Restore/PITR Drills, Corruption Injection & Emergency Restore  |
+-----------------------------------------------------------------------------+
                                       ▲
+-----------------------------------------------------------------------------+
|                     Tier 3: Cross-Feature Combinations                      |
|  Postgres Basebackup + WAL Stream + AES-GCM + S3 WORM + Restore Drill       |
|  MySQL Dump + Binlog Ingest + Corrupted Chunk + Merkle Rejection            |
|  SQLite WAL + Quick Check + Standalone Emergency Restore                    |
+-----------------------------------------------------------------------------+
                                       ▲
+-----------------------------------------------------------------------------+
|                        Tier 2: Boundary & Edge Cases                        |
|  Empty DBs, Extreme Tables, Chunk Bitflips, Bad Master Keys, Invalid LSNs,  |
|  Lease Expirations, Negative Offsets, Truncated Nonces, WORM Violations     |
+-----------------------------------------------------------------------------+
                                       ▲
+-----------------------------------------------------------------------------+
|                           Tier 1: Feature Tests                             |
|  >=5 Dedicated Unit/Contract Tests for each of the 29 Features in PROJECT.md|
|  (Total: >= 145 Feature-Specific Test Cases)                                |
+-----------------------------------------------------------------------------+
```

---

## 2. Tier Details & Coverage Specifications

### Tier 1: Feature-Level Verification (29 Features $\times \ge 5$ Tests)
Every feature catalogued in `PROJECT.md` is tested with at least 5 isolated, authoritative test cases:
1. **F01: PostgreSQL Logical Dump & Restore** (custom, directory, plain SQL formats, table selection, schema extraction).
2. **F02: PostgreSQL Physical Basebackup** (tar stream parsing, tablespace mapping, WAL method include/stream, manifest verification).
3. **F03: PostgreSQL WAL Streaming & Ingestion** (segment naming regex, LSN timeline validation, archive_command ingestion, gap detection).
4. **F04: MySQL / MariaDB Dump & Restore** (single-transaction consistency, routine/trigger extraction, GTID header parsing, socket/TCP connections).
5. **F05: MySQL Binlog Streaming & Ingestion** (binlog magic bytes, format description events, server ID scoping, continuous segment archiving).
6. **F06: SQLite Online Safe Backup** (online backup API, `VACUUM INTO`, WAL snapshotting, page size preservation, quick_check probe).
7. **F07: Streaming AEAD AES-256-GCM Encryption** (HKDF subkey derivation, 256-bit entropy requirement, random nonce generation, domain-separated AAD verification, tampering rejection).
8. **F08: S3-Compatible CAS Chunked Uploads** (multipart chunking, Content-Addressable Storage deduplication, ETag calculation, profile configurations).
9. **F09: Deterministic Merkle Tree Generation** (domain-separated HMAC-SHA256 leaf and root digests, tree ordering determinism, chunk digest calculation).
10. **F10: PureEd25519 Manifest Signing & Verification** (public-key signing, signature verification, detached payload verification, wrong-key rejection).
11. **F12: S3 Object Lock WORM Immutability** (Compliance/Governance retention modes, Legal Holds, expiration timestamp enforcement, overwrite rejection).
12. **F12: Two-Phase Atomic Manifest Commit** (`manifest.json` staging, `manifest.sig` signature commit, atomic `complete.json` marker validation).
13. **F13: Ephemeral Sandbox Isolation** (isolated temporary directory creation, process namespace isolation, cleanup lifecycle, collision prevention).
14. **F14: PostgreSQL `pg_amcheck` & Checksum Probes** (B-tree index validation, heap page checksum verification, corruption detection reporting).
15. **F15: MySQL `CHECK TABLE` Probes** (`CHECK TABLE ... EXTENDED`, index consistency validation, error status parsing).
16. **F16: SQLite Integrity & Quick Check Probes** (`PRAGMA integrity_check`, `PRAGMA quick_check`, schema table verification).
17. **F17: Row Count & Schema Reconciliation** (pre/post restore table schema hash matching, row count sum reconciliation, mismatch alerting).
18. **F18: PII Privacy Masking Pipeline** (sensitive column identification, rule-based hashing/redaction, deterministic sandbox masking).
19. **F19: Ed25519-Signed Recovery Certificate** (immutable verification certificate generation, RTO/RPO calculation, Ed25519 cryptographic signing).
20. **F20: Continuous LSN & GTID Timeline Tracking** (continuous LSN range continuity checks, MySQL GTID set intervals, gap identification).
21. **F21: Timeline Branch & History Switch Handling** (Postgres timeline switch detection, `.history` file management, branched replay).
22. **F22: Instant Gap & Corruption Alerting** (missing segment detection, degraded status transition, webhook alert payload dispatch).
23. **F23: Target Timestamp PITR Replay** (point-in-time calculation, base backup + WAL sequence planning, exact second replay).
24. **F24: Target LSN / Recovery Point Replay** (named restore point replay, LSN target position stop, consistent cut validation).
25. **F25: Zero-Dependency Emergency Restore Utility** (standalone binary execution, local filesystem/target restoration, no running daemon required).
26. **F26: Standalone S3 & Decryption in Emergency Tool** (direct S3 object download, HKDF key derivation, AEAD chunk decryption).
27. **F27: Standalone Merkle & Signature Validation** (standalone Ed25519 signature check and Merkle root verification before any write).
28. **F28: Comprehensive E2E Test Suite (Tiers 1-4)** (opaque-box test execution, automated test runner, structured test output).
29. **F29: Adversarial Coverage Hardening (Tier 5)** (bit-flip corruption injection, timeline split attacks, replay attacks, truncated payloads).

### Tier 2: Boundary & Corner Cases
- **Empty Databases**: 0-byte SQLite files, empty Postgres/MySQL schemas with zero tables.
- **Large Chunks / Scale**: multi-megabyte chunks, boundary chunk sizes (exact page size multiples, off-by-one bytes).
- **Corrupted Chunks**: single bit flips in ciphertext, truncated nonces, malformed AAD tags.
- **Cryptographic Boundaries**: master keys with insufficient entropy (<32 bytes), corrupt Ed25519 public keys, altered manifest digests.
- **Invalid LSNs & Timestamps**: negative LSN positions, timestamps in the distant future or prior to base backup creation.

### Tier 3: Cross-Feature Combinations
- **Postgres Physical + WAL + AES-GCM + S3 Object Lock + Restore Drill**: Full end-to-end base backup creation with encrypted chunk uploads, continuous WAL ingestion, WORM retention policy enforcement, sandbox restoration, and pg_amcheck consistency verification.
- **MySQL Dump + Binlog Ingest + Corrupted Chunk Injection + Merkle Rejection**: Full MySQL dump with binlog capture, followed by deliberate byte mutation in storage, asserting that the Merkle root verifier halts restoration before touching disk.
- **SQLite WAL + Quick Check + Standalone Emergency Restore**: Online SQLite backup snapshotting with WAL replay, quick_check probe verification, and standalone zero-dependency emergency extraction directly from storage artifacts.

### Tier 4: Real-World Workloads
- **Live PostgreSQL (127.0.0.1:5432)**: Live connections, schema creation, data insertion, logical dump, and live restore verification.
- **Live MySQL (127.0.0.1:3306)**: Live connections, database creation, table check probes, logical export, and verification.
- **SQLite 3 Engine**: Real filesystem DBs, WAL concurrent writes, `.backup` execution, and row reconciliation.
- **Local S3 / WORM Storage Mock**: Full S3 multipart simulation, Object Lock immutability simulation, and CAS chunk deduplication.

---

## 3. Directory Layout & File Organization

The E2E test suite is located in `/home/sifu/Documents/dbvault/test/e2e/`:

```
test/e2e/
├── e2e_test.go              # Test suite entry point & runner setup
├── testenv.go               # Shared test harness, mock S3 server, live DB helpers
├── tier1_features_test.go   # Tier 1: >=5 tests per feature for Features 1-10
├── tier1_features2_test.go  # Tier 1: >=5 tests per feature for Features 11-20
├── tier1_features3_test.go  # Tier 1: >=5 tests per feature for Features 21-29
├── tier2_boundary_test.go   # Tier 2: Boundary, corner cases & edge conditions
├── tier3_combined_test.go   # Tier 3: Cross-feature end-to-end integration workflows
└── tier4_realworld_test.go  # Tier 4: Real-world workloads against live DBs and S3
```

---

## 4. Test Execution & Pass/Fail Semantics

### Execution Commands

To execute the entire E2E test suite:
```bash
go test -v -timeout 10m ./test/e2e/...
```

To execute a specific tier:
```bash
# Run Tier 1 Feature Tests
go test -v -run "TestTier1" ./test/e2e/...

# Run Tier 2 Boundary Tests
go test -v -run "TestTier2" ./test/e2e/...

# Run Tier 3 Cross-Feature Combinations
go test -v -run "TestTier3" ./test/e2e/...

# Run Tier 4 Real-World Workload Tests
go test -v -run "TestTier4" ./test/e2e/...
```

### Pass/Fail Semantics
1. **Deterministic Exit Codes**:
   - Exit code `0`: All test cases passed with zero failures and zero unhandled errors.
   - Exit code `!= 0`: One or more test assertions failed, or an unexpected runtime panic occurred.
2. **Authoritative Expected Outputs**:
   - Cryptographic hashes and signatures are verified with standard crypto libraries (`crypto/ed25519`, `crypto/aes`, `crypto/hmac`, `crypto/sha256`).
   - Consistency probes match live database engine state (`pg_amcheck`, `CHECK TABLE`, `PRAGMA quick_check`).
   - Bit-flip corruption tests must strictly fail verification and return designated error types (`ErrChunkAuthenticationFailed`, `ErrManifestInvalid`, etc.).
3. **No Flakiness / Isolation**:
   - Every test creates its own ephemeral scratch directories and cleans up after itself (`t.Cleanup()`).
   - No cross-test state leakage or port collision.
