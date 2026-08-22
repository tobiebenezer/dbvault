---
date: 2026-08-22
topic: "dbvault Production Hardening"
status: validated
---

# dbvault Production Hardening Design

## Problem Statement

dbvault is a Go backup vault appliance: SQLite metadata core with chunking, AES-GCM
encryption, signed manifests, and atomic restore. The core is solid. Everything around
it — auth, scheduling, alerting, observability, deployment truthfulness — is stubbed,
simulated, or actively dangerous. An audit found:

| # | Severity | Finding | Evidence |
|---|----------|---------|----------|
| 1 | CRITICAL | Master-key reveal endpoint is unauthenticated; anyone reaching the listen port gets the AEAD master key as JSON | `internal/server/server.go` route `/api/v1/keys/master/reveal` → `keysMasterReveal` |
| 2 | CRITICAL | Web console has zero auth — 60+ routes on a bare mux; setup token skipped entirely when `DBVAULT_DATABASE_URL` set | `internal/server/server.go`, `SetupManager` skip at `main.go:1615` |
| 3 | CRITICAL | Control-plane auth fails open — empty token bypasses the check silently | `internal/platform/controllerapi/server.go:48` |
| 4 | CRITICAL | MySQL backups cannot work — driver emits `MYSQL_PWD=""` always | `internal/adapters/source/mysql/driver.go:204-206` |
| 5 | HIGH | One master key does triple duty (AEAD + HMAC + dedup derivation); short keys zero-padded to 32 bytes | `cmd/dbvault/main.go:350-370` |
| 6 | HIGH | Scheduler simulated — `internal/application/scheduler/` empty; jobs get fake progress from a 1s ticker (`GenerateDemoProgress()`); nothing calls `LeaseNext`; real SQLite queue is dead code | `server.go:~1698`, `ports/job_queue.go` |
| 7 | HIGH | Retention/GC never runs automatically — manual `/api/v1/gc/run` only | `garbagecollection/service.go`, `server.go:171` |
| 8 | HIGH | Healthcheck lies — `doctor` without `--config` prints static OK, exits 0 | `cmd/dbvault/main.go:436-443` |
| 9 | MEDIUM | No alerting, no metrics — notification and telemetry adapter dirs are empty | `adapters/notification/email/`, `adapters/telemetry/prometheus/` |
| 10 | MEDIUM | Repo hygiene — `web/node_modules/`, ~19MB binary, release tarballs, `.opencode/` state tracked in git; master key defaults under `scratch/` | `git ls-files`, `main.go:150` |

## Constraints

- Go project with dual build tags: `restricted` (dependency-free) and prod (`!restricted`). Changes must respect both builds.
- The SQLite crypto/restore pipeline (chunking, AES-GCM, signed manifests, atomic restore) is **not** redesigned — hardening wraps it.
- Single-node Linux server target; TLS terminated by reverse proxy (Caddy/nginx) in front of the appliance.
- S3-compatible object storage as offsite target (R2/Wasabi/Minio profiles already exist).
- Webhook-first alerting; email deferred.
- Existing seams must be used, not replaced: `ports.JobQueue`, `SetupManager`, config knobs (`schedule.enabled`, `cron`, `every_seconds`, `timezone`), per-phase compose stacks.

## Approach

**Phased security-first hardening of the existing architecture.**

Order matters: fix the trust boundary first (nothing else matters if :8080 leaks the
master key), then make backups actually work (MySQL creds), then wire the operational
loop (schedule → execute → retain → notify → measure), then deployment polish.

Rejected alternatives:
- *Full multi-DB rewrite first* — the SQLite pipeline works; rewriting drivers before securing the API adds risk for no gain.
- *Ship behind an authenticating reverse proxy only* — external auth cannot fix fail-open controller semantics or key-reveal endpoint behavior; auth must be native.

## Architecture

Four layers change; storage/crypto core untouched:

1. **Trust boundary layer** — session/token auth middleware wrapping all appliance routes; fail-closed controller startup; master-key reveal disabled by default behind explicit opt-in requiring admin auth.
2. **Execution layer** — real worker loop consuming `ports.JobQueue.LeaseNext`; schedule table ticks enqueue due jobs using existing config knobs; lease-based locking gives crash safety via `RecoverExpired`.
3. **Operations layer** — post-job hooks: scheduled GC sweep, failure notifications via webhook adapter, Prometheus metrics endpoint.
4. **Deployment layer** — truthful healthchecks, production compose/systemd profiles, self-backup of dbvault's own SQLite metadata, ops runbook.

## Components

### Auth middleware (`internal/server/middleware/auth`)
- Admin credential bootstrapped through the existing setup-token flow.
- Session cookies for console; Bearer tokens for API clients; constant-time comparisons.
- All `/api/v1/*` routes protected except setup flow and health endpoints.

### Controller hardening
- Empty token = refuse to start, unless `--insecure-demo` passed explicitly (loud warning).

### Key management
- HKDF-split master key into independent encryption / HMAC / dedup subkeys (domain separation).
- New repositories require genuine 32-byte keys; zero-pad path removed for new keys.
- Legacy repos detected via manifest version: warn loudly; re-key tooling is an open question.

### Worker loop (`internal/application/scheduler`)
- Ticks schedule table, enqueues due jobs, N workers lease-execute, heartbeats renew leases.
- `RecoverExpired` on startup + interval reclaims crashed leases.
- Demo progress simulator removed from server path; kept only behind explicit `--demo`.

### MySQL credential fix
- Driver resolves credentials through the same secret-resolution path as other drivers and passes them correctly to mysqldump env.

### Notification adapter (webhook)
- JSON POST on job success/failure/GC results with job summary.
- Retries with backoff; delivery failures logged, never fatal.

### Metrics adapter (`/metrics`)
- Job counts by status, bytes backed up, duration histograms, queue depth, lease-expiry events.

### Truthful healthcheck
- `doctor --config <path>` verifies: config loads, SQLite opens+migrates, storage backend reachable (HeadBucket for S3), scratch disk above free-space watermark.
- Exits non-zero on any failure; Docker HEALTHCHECK invokes it with `--config`.

### Deployment artifacts
- Production compose profile alongside demo.
- systemd timer backing up dbvault's own SQLite metadata store into the vault itself.
- Ops runbook: first-boot setup, key ceremony, restore drill, alert triage.
- `.gitignore` gains `bin/`, `dist/`, `web/node_modules/`, `.opencode/`; tracked artifacts removed from index (files stay on disk).

## Data Flow

**Backup:** schedule tick → job enqueued → worker leases → source dump with real creds → chunks encrypted (AEAD subkey) → manifest signed (HMAC subkey) → written to repository (filesystem/S3) → job completed → retention GC sweep → webhook fired, counters incremented.

**Restore:** request → manifest signature verified → chunks decrypted/reassembled → atomic swap into target → post-restore verification recorded on job.

**Trust bootstrap:** first boot prints setup token → admin sets credentials via setup flow → console sessions issued, API clients use Bearer tokens.

## Error Handling

- Fail-closed by default on every auth surface; `--insecure-demo` is the only bypass and logs a loud warning.
- Job retries with backoff at queue level; terminal failures trigger webhook + metric — never silent drops.
- Notification delivery failures are logged, never fatal — alerting must not take down backups.
- Lease recovery makes worker crashes self-healing.
- Structured logging with job IDs threaded through every pipeline stage for 3am triage.

## Testing Strategy

- **Unit:** auth middleware accept/reject paths, HKDF derivation vectors, schedule resolution, retention evaluation.
- **Integration:** extend existing per-phase compose stacks with a real MySQL container proving credential flow; Postgres round-trips.
- **End-to-end:** backup → destroy → restore → verify against filesystem and MinIO repositories.
- **Negative/security:** unauthenticated requests rejected on all `/api/v1/*`; key reveal returns 403 by default even when authenticated; empty-token controller refuses start; demo ticker absent without `--demo`; doctor exits non-zero on broken config.

## Rollout Phases

- **Phase A — Trust boundary:** auth middleware, fail-closed controller, key-reveal gating, HKDF split, repo hygiene.
- **Phase B — Real execution:** worker loop, scheduler wiring, MySQL creds fix, remove demo ticker outside `--demo`.
- **Phase C — Operations:** scheduled GC, webhook notifications, Prometheus metrics, truthful doctor.
- **Phase D — Deployment:** production compose/systemd polish, metadata self-backup timer, runbook.

## Open Questions

- Re-key tooling for legacy zero-padded repos — launch blocker or documented limitation?
- Email SMTP specifics — deferred behind webhook adapter.
