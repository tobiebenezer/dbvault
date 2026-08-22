---
date: 2026-08-22
topic: "dbvault Production Hardening"
status: draft
---

# Implementation Plan: dbvault Production Hardening

Design source: `thoughts/shared/designs/2026-08-22-dbvault-production-hardening-design.md`

## Ground truth (verified seams, 2026-08-22)

All seams below were verified against the working tree. Line numbers are current as of this plan.

| Seam | Location | Verified state |
|---|---|---|
| Appliance routes (60+, bare mux) | `internal/server/server.go:100-160` | No auth wrapper anywhere; `/api/v1/keys/master/reveal` registered at :143 |
| `keysMasterReveal` handler | `internal/server/server.go:1633-1644` | POST-only; calls `px.RevealMasterKey()` with zero auth |
| `SetupManager` | `internal/server/server.go:1590-1690` | `Ensure()` writes 0600 token file; `Complete()` constant-time compare + delete |
| Setup skip bug | `server.go:1614-1623` (`Required()` returns false when `DBVAULT_DATABASE_URL`/`DATABASE_URL` set) and `cmd/dbvault/main.go:839-845` (`_ = os.Remove(.../setup-token)` in the same branch) | Both must go |
| Controller fail-open | `internal/platform/controllerapi/server.go:46-48` | `if s.Token != ""` — empty token skips the check entirely |
| Controller binary | `cmd/dbvault-controller/main.go:27-28` | Builds server from `*token` flag/env with no empty check |
| MySQL env bug | `internal/adapters/source/mysql/driver.go:204-206` | `env()` always returns `{Name:"MYSQL_PWD", Value:"", Sensitive:true}` |
| Key triple-duty | `cmd/dbvault/main.go:348-370` (`legacyBackup`) | Same `key` slice passed to `aead.New`, `ms.NewHMACSigner`, and `DedupKey`; `<32` bytes zero-padded at :352-356 |
| Key default location | `cmd/dbvault/main.go:~150` (`keyCmd`) | Falls back to `./scratch/data/master.key`; also honors `DBVAULT_KEY_FILE` / `DBVAULT_MASTER_KEY` |
| Demo ticker | `internal/server/server.go:1698-1710` (`Run`) | Unconditional 1s ticker calling `appliance.px.GenerateDemoProgress()`; simulator lives at `internal/application/productexperience/service_jobs.go:1217` |
| Job queue port | `internal/ports/job_queue.go` | `Enqueue / LeaseNext(ctx, workerID, types, leaseDuration) / Renew / Complete / Fail / Cancel / RecoverExpired` |
| Queue impl | `internal/adapters/jobqueue/sqlite/queue.go` | **Deviation found:** despite package name `sqlite`, storage is an in-memory `map[domain.JobID]domain.Job{}`. Phase B includes a durability task. |
| Job types | `internal/domain/job.go:10-18` | `backup, verify, restore_drill, restore, replication, retention, garbage_collection, repository_scan, repository_repair` |
| Schedule knobs | `internal/config/types.go:355-360` (`Schedule{Enabled, EverySeconds, Cron, Timezone}`), richer `Schedules []ScheduleConfig` at types.go:291 (id/source/operation/cron/timezone/enabled); parsing wired in `internal/config/config.go:132-135` (restricted) — **must mirror in `config_prod.go`** | Both build-tag variants exist for every config/bootstrap/catalogue file |
| Webhook + metrics config already typed | `internal/config/types.go:317-330` | `NotificationConfig.Webhook{Enabled, URL SecretReference, SigningSecret SecretReference}`, `MetricsConfig{Enabled, Path,...}` — reuse, do not invent new shapes |
| GC service | `internal/application/garbagecollection/service.go:119` | `func (s Service) Run(ctx, plan Plan) error` |
| Doctor vacuity | `cmd/dbvault/main.go:436-451` | No `--config` → prints static OK, exit 0 |
| Healthcheck lies | `deploy/docker/Dockerfile` (HEALTHCHECK runs bare `dbvault doctor`) and `deploy/compose/demo.yml` healthcheck (same) | Fix alongside doctor rewrite |
| Empty adapter dirs | `internal/application/scheduler/`, `internal/adapters/notification/email/`, `internal/adapters/telemetry/prometheus/` | Confirmed empty |
| Compose stacks | `deploy/compose/{demo,dev-postgres,integration,phase3..7-integration}.yml` | `integration.yml` = MinIO only (bucket `dbvault-integration`, creds `integration-access`/`integration-secret`); no MySQL stack exists yet |
| systemd | `deploy/systemd/dbvault.service` (hardened), `deploy/systemd/dbvaultd.service` | Service unit exists; no timer units |
| Git junk | `.gitignore` covers only `.env*`, `lakehouse/`, `*.db*` | `git ls-files` shows **2860** tracked junk paths under `bin/`, `dist/`, `web/node_modules/`, `.opencode/` |

**Global constraint for every task:** the repo is dual-build. Every Go change must compile and test green under both:

```bash
CGO_ENABLED=0 go vet -tags=restricted ./... && CGO_ENABLED=0 go test -tags=restricted ./...
go vet ./... && go test ./...
```

Per-phase verification sections repeat this as "dual-build gate".

---

## Phase A — Trust Boundary

> Nothing else matters if `:8080` leaks the master key. Ship A before any other phase.
> A1, A2, A3, A5, A6 are mutually independent — batch them in parallel. A4 depends on A2 (needs admin identity to gate against). A7 lands last.

### A1 — Repo hygiene (parallelizable)
- **Files:** `.gitignore` (modify); git index only.
- **Changes:** Append ignore entries: `bin/`, `dist/`, `web/node_modules/`, `.opencode/`, `scratch/`. Then untrack without deleting from disk:
  ```bash
  git rm -r --cached bin dist web/node_modules .opencode scratch 2>/dev/null; true
  ```
- **Verification:** `git ls-files | grep -cE '^(bin/|dist/|web/node_modules|\.opencode/|scratch/)'` → `0`; `test -f bin/dbvault && echo kept-on-disk` → files still present; dual-build gate passes (no code touched).
- **Closes:** finding #10.

### A2 — Auth middleware for the appliance API (parallelizable)
- **Files (create):**
  - `internal/server/middleware/auth/auth.go` — session store + token model
  - `internal/server/middleware/auth/middleware.go` — the wrapping handler
  - `internal/server/middleware/auth/handlers.go` — login/logout/bootstrap endpoints
  - `internal/server/middleware/auth/*_test.go`
- **Design-level changes:**
  - Admin credential bootstrapped through the existing setup-token flow (`SetupManager.Ensure/Complete`, server.go:1590-1690): while a valid one-time setup token is presented, allow creating the admin credential; after `Complete()` deletes the token file, no further bootstrap.
  - Two credential flavors: session cookie (HttpOnly, Secure, SameSite=Strict, random 256-bit ID, sliding expiry) for the console; Bearer tokens (constant-length random, stored hashed) for API clients.
  - All comparisons via `crypto/subtle.ConstantTimeCompare` on equal-length inputs (copy the pattern from `controllerapi/server.go` but fail-closed).
  - Wrap **all** `/api/v1/*` routes except: `/health`, `/ready`, `/api/v1/setup*` (setup flow itself), and later `/metrics` (Phase C decision). Implementation point: `Appliance.HTTPServer()` in `internal/server/server.go` — wrap the mux once before returning, so all ~70 route registrations are covered by construction rather than per-route edits.
  - Login rate-limit: simple fixed-window counter per remote IP in memory (fail-closed, no new deps).
- **Verification:**
  - Unit tests: valid cookie accepted, missing/bad token → 401, expired session → 401, setup endpoints reachable pre-auth, everything else 401 pre-auth.
  - Manual: build restricted, run `bin/dbvault server --demo --listen 127.0.0.1:8081 --data-dir /tmp/dvA2`, then:
    ```bash
    curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8081/api/v1/status   # expect 401
    curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8081/health          # expect 200
    ```

### A3 — Fail-closed controller auth (parallelizable)
- **Files:** `internal/platform/controllerapi/server.go` (modify `auth()`, `New()`, `Server` struct); `cmd/dbvault-controller/main.go` (modify startup).
- **Changes:**
  - Add `InsecureDemo bool` field. In `auth()`: if `s.Token == "" && !s.InsecureDemo` → return 503 "controller token not configured" (fail closed even if somehow reached). Keep constant-time compare otherwise.
  - In `cmd/dbvault-controller/main.go`: add `--insecure-demo` flag; if token empty and flag unset → print loud error to stderr and `os.Exit(2)` before binding the listener; if flag set → log `WARNING: controller running WITHOUT authentication (--insecure-demo)` in JSON and proceed.
- **Verification:** `go test ./internal/platform/controllerapi/` with new cases: empty token + no demo → every `/v1/*` request 503; empty token + demo → 200; non-empty wrong token → 401. Binary check: `DBVAULT_CONTROLLER_TOKEN= ./bin/dbvault-controller --listen 127.0.0.1:0 ; echo $?` → `2`.

### A4 — Master-key reveal gating (depends on A2)
- **Files:** `internal/server/server.go` (`keysMasterReveal` :1633-1644, plus `keysMasterSet` :1646 and `keysMasterGenerate` :1666 get admin-auth requirement); `internal/config/types.go` + both parsers (`config.go`, `config_prod.go`) for a new knob.
- **Changes:**
  - New config knob `security.allow_master_key_reveal` (bool, default **false**) added to config types and both build-tag parsers.
  - `keysMasterReveal`: require (a) authenticated **admin** principal from A2 middleware AND (b) `allow_master_key_reveal=true`. Any miss → `403` with a body that does not echo key material. Default posture: 403 even for admins.
  - `keysMasterSet` / `keysMasterGenerate` keep their behavior but move behind admin auth; their responses contain secrets, so mark responses `Cache-Control: no-store`.
  - `GetMasterKeyInfo` (:1625) stays readable-by-admin only (metadata, no secret).
- **Verification:** httptest matrix in `internal/server/` tests: unauth reveal → 401; authed admin, opt-out default → **403**; authed admin + opt-in → 200; non-admin + opt-in → 403.

### A5 — Stop skipping setup when DBVAULT_DATABASE_URL is set (parallelizable)
- **Files:** `internal/server/server.go:1614-1623` (`Required()`), `cmd/dbvault/main.go:839-845` (serverCmd branch that removes the setup token file).
- **Changes:**
  - Delete the env-var short-circuit in `Required()`; setup is required whenever the token file is absent/unreadable, regardless of external DB URL.
  - In `serverCmd`: replace the `_ = os.Remove(filepath.Join(*dataDir, "setup-token"))` branch with unconditional `EnsureSetupToken()` printing the token once to stdout (operator records it). External-database deployments get the same trust bootstrap as SQLite ones.
- **Verification:** `DBVAULT_DATABASE_URL=postgres://u:p@localhost:1/x bin/dbvault server --listen 127.0.0.1:8082 --data-dir /tmp/dvA5` prints `setup token:` line and creates `setup-token` with mode 0600 (`stat -c '%a' /tmp/dvA5/setup-token` → `600`). Negative test: second start reuses existing token file (no rotation).

### A6 — HKDF subkey derivation, retire triple-duty + zero-pad (parallelizable)
- **Files (create):** `internal/platform/keyderive/hkdf.go` (+ `hkdf_test.go`).
- **Files (modify):** `cmd/dbvault/main.go:339-370` (`legacyBackup`), key-handling call sites in `keyCmd` (~:140-260), and the production wiring path where `bootstrap.Build` consumers assemble `backup.Service`.
- **Changes:**
  - Implement RFC 5869 HKDF-SHA256 by hand (`crypto/hmac` + `crypto/sha256` only — keeps the **restricted dependency-free build green**; do NOT import `golang.org/x/crypto`). API: `Derive(master []byte, domain string) ([32]byte, error)` with domain labels `"dbvault/aead-v1"`, `"dbvault/hmac-v1"`, `"dbvault/dedup-v1"`; salt = key fingerprint hex; refuse master keys shorter than 32 bytes with a descriptive error.
  - Replace single-key usage: AEAD gets the encryption subkey, HMACSigner the HMAC subkey, `DedupKey` the dedup subkey.
  - New keys: `key generate/set` must produce/accept exactly 32 bytes of entropy; remove the zero-pad path (:352-356) for anything newly written. Legacy detection: manifest/repository version field < v-harden marker → loud stderr warning ("repository uses legacy single-duty/zero-padded key material") while continuing to operate read/write with legacy derivation; full re-key tooling stays an open question per design §Open Questions — record it in the runbook (D3).
  - Update `scratch/data/master.key` handling so the on-disk file remains the raw master; derivation happens in memory only; add `mlock`-style best-effort note (documented, not required).
- **Verification:**
  - RFC 5869 Test Case 1 & 2 vectors reproduced exactly in `hkdf_test.go`.
  - Round-trip: backup then restore with derived subkeys succeeds; flipping any single label bit breaks HMAC verification (negative test proving domain separation).
  - `printf 'short' | ...` style unit test: master key of 16 bytes → `Derive` errors, no silent pad.
- **Closes:** finding #5.

### A7 — Phase A negative security test suite (last in phase)
- **Files (create):** `internal/server/security_negative_test.go`, additions to `internal/platform/controllerapi/server_test.go`.
- **Cases (from design §Testing Strategy):**
  1. Every sampled `/api/v1/*` route returns 401 unauthenticated (table-driven over the real mux from `HTTPServer()`).
  2. `POST /api/v1/keys/master/reveal` returns **403 by default even when fully authenticated** (admin session + bearer both tried).
  3. Controller with empty token refuses to start (exit 2) and, if constructed directly, serves 503 on `/v1/*`.
  4. Setup flow cannot be completed twice (token file deleted post-`Complete`; replay → 403).
  5. Session cookie flags asserted: HttpOnly, Secure, SameSite=Strict present on Set-Cookie.
- **Verification:** `go test ./internal/server/... ./internal/platform/controllerapi/...` under both tags.

---

## Phase B — Real Execution

> Depends on Phase A being merged (auth wraps the surface workers use).
> B1+B2 are one workstream (scheduler+queue); B3 depends on B1; B4, B5 independent — parallelize across workstreams.

### B1 — Worker loop + scheduler core
- **Files (create):** `internal/application/scheduler/scheduler.go`, `internal/application/scheduler/worker.go`, `internal/application/scheduler/cron.go`, `+ _test.go` each.
- **Changes:**
  - `Scheduler`: ticks on a bounded interval (min 1s), resolves due schedules from `cfg.Schedules[]` (preferred) falling back to legacy `cfg.Schedule` knobs (`schedule.enabled/every_seconds/cron/timezone`, types.go:355). `cron.go` implements the subset of 5-field cron needed (or converts cron→next-fire using stdlib-free arithmetic; timezone via `time.LoadLocation`). Dedupe: a schedule with a live (leased/queued) job is not re-enqueued — track in-flight map keyed by schedule ID.
  - `WorkerPool`: N workers (default 1; new optional knob `schedule.workers` added to config types + both parsers). Loop: `LeaseNext(ctx, workerID, []domain.JobType{...}, leaseDuration)` → heartbeat goroutine `Renew` at leaseDuration/3 → execute → `Complete(resultJSON)` or `Fail(domain.JobFailure{})` with retry/backoff counters already modeled by the queue's failure path.
  - Startup + every tick: `RecoverExpired(ctx, now)` to reclaim crashed leases; recovered jobs re-enter the queue.
  - Execution dispatch: `JobBackup` → `backup.Service.Create` (via injected executor interface so tests don't need real storage); `JobGarbageCollection` reserved for C1; unknown types fail with clear reason.
- **Verification:** unit tests with fake clock: enqueue-on-tick, lease exclusivity between two workers, lease-expiry recovery, heartbeat renewal prevents expiry. Dual-build gate.

### B2 — Make the job queue actually durable
- **Files:** `internal/adapters/jobqueue/sqlite/queue.go` (modify), schema init alongside catalogue's store pattern (`internal/adapters/catalogue/sqlite/` restricted/prod pair shows the seam), tests updated.
- **Changes:** The verified implementation is map-backed (deviation from its package name). Persist jobs into a `jobs` table using the same embedded-SQLite seam the catalogue uses per build tag (restricted: dependency-free persistence path already used there; prod: real driver). Keep `ports.JobQueue` signatures byte-identical. Lease columns: `leased_by`, `lease_until`, `attempts`, `status`.
- **Verification:** restart-survival test: enqueue, close store, reopen, `LeaseNext` returns the job; concurrent `LeaseNext` from two handles never hands out one job twice (race test with `-race`).

### B3 — Wire scheduler into server bootstrap
- **Files:** `internal/bootstrap/application.go` + `application_prod.go` (add `Queue ports.JobQueue` + `Executor` fields to `Application`), `cmd/dbvault/main.go` `serverCmd` (~:789-866), `internal/server/server.go` `New`/`Run` (accept worker handle; start/stop in `Run` next to ctx lifecycle).
- **Changes:** When `--config` provided: load cfg, build app via `bootstrap.Build`, construct Scheduler+WorkerPool, run alongside HTTP server; graceful stop drains leases (Cancel in-flight or wait ≤ leaseDuration). Without `--config` (demo/no-config path): scheduler disabled — demo data flow unchanged. Log job lifecycle events with job IDs (design §Error Handling) through `slog`.
- **Verification:** integration smoke: config with `schedule.enabled=true`, `every_seconds=2`, sqlite-file source → observe enqueued+completed job rows within ~5s in the jobs table and job events surfacing on `/api/v1/events/jobs` (authenticated).

### B4 — MySQL credential fix (independent)
- **Files:** `internal/adapters/source/mysql/driver.go:204-206` (`env()`), `Config` plumbing in same package; `driver_test.go`.
- **Changes:**
  - `env()` returns `{Name:"MYSQL_PWD", Value: d.Config.Password, Sensitive:true}` resolved through the same secret-resolution path used by the Postgres driver (verify parity during implementation: password arrives via config secret reference resolution, never read from process env directly).
  - Ensure username reaches mysqldump via existing arg builder (`--user`/`-u` — confirm present in `dumpArgs()` above :180; add if absent). MariaDB path identical (`mariadb-dump` honors MYSQL_PWD).
  - Validation: `ValidateSource` gains "password required unless socket auth configured" rule consistent with postgres driver semantics.
- **Verification:**
  - Unit: `env()` exposes resolved password; empty password + TCP host fails validation loudly before spawning mysqldump.
  - Integration IT1 (below) proves end-to-end against a real container.

### B5 — Remove demo progress ticker outside `--demo` (independent)
- **Files:** `internal/server/server.go:1698-1710` (`Run`).
- **Changes:** Gate the 1s ticker goroutine on the appliance's demo flag (`dbvserver.Config.Demo`, already plumbed from `--demo` at main.go:838). Without `--demo`, job progression comes solely from the real scheduler (B1/B3). Keep `GenerateDemoProgress` untouched for genuine demo mode.
- **Verification:** negative test: start server without `--demo`, assert no job transitions occur without an executed job (poll `/api/v1/jobs` for N seconds — stable); with `--demo`, progression continues. Add to `security_negative_test.go` case list.

### B6 — Integration stack IT-M1: real MySQL credential proof (after B4)
- **Files (create):** `deploy/compose/mysql-integration.yml`; `scripts/integration/mysql-roundtrip.sh`; `internal/adapters/source/mysql/integration_test.go` (build tag `integration`).
- **Changes:**
  - Stack: `mysql:8` (healthcheck `mysqladmin ping`), service seeds a small dataset via init SQL; dbvault built from local tree (prod tags where deps available; restricted fallback exercises driver arg/env assembly only).
  - Script: waits healthy → registers source pointing at container with correct creds → `backup-create` → asserts dump artifact > 0 bytes and contains seeded marker row. Negative leg: wrong-password registration → source test / backup fails with credential error, non-zero exit.
- **Verification:** `docker compose -f deploy/compose/mysql-integration.yml up --build --exit-code-from dbvault-test` → exit 0; negative variant exits non-zero.

---

## Phase C — Operations

> Depends on Phase B (jobs exist to hook). C1+C2 couple loosely (GC results feed notifications); C3, C4 independent.

### C1 — Scheduled GC trigger
- **Files:** `internal/application/scheduler/hooks.go` (create — post-job hook registry); `internal/application/scheduler/scheduler.go` (invoke hooks on Complete/Fail); bootstrap wiring from B3.
- **Changes:** On every successful `JobBackup` completion (and on a configurable interval fallback), enqueue a `JobGarbageCollection` job whose executor calls `garbagecollection.Service.Run(ctx, plan)` building `Plan` from `cfg.Repository.Retention` (`keep_last/daily/weekly/monthly/tombstone_grace`, config.go:117-121). GC results recorded as job result JSON (reclaimed object count/bytes) for C2/C3 consumption.
- **Verification:** unit test with fake clock: retention policy deletes only tombstone-eligible snapshots beyond grace; interval fallback fires without backups. Integration leg inside IT-E2 (below).

### C2 — Webhook notification adapter
- **Files (create):** `internal/ports/notifier.go` (small interface: `Notify(ctx, Event) error`); `internal/adapters/notification/webhook/notifier.go` (+ `_test.go`). Note: `internal/adapters/notification/email/` stays empty — email deferred by design.
- **Changes:**
  - Reuse existing config shape: `notification.webhook.{enabled,url,signing_secret}` (types.go:317-325) — resolve both `SecretReference`s through the same secret provider path as destination credentials.
  - Events: job success/failure (with job summary: id/type/source/status/duration/error), GC result. Payload = signed JSON: `X-DBVault-Signature: sha256=<hmac>` using signing secret; body includes event type + timestamp (anti-replay hint documented).
  - Retry: exponential backoff (e.g. 3 attempts, 1s/4s/16s cap 30s), jittered; final delivery failure → structured log warn + metric increment (C3) — **never fatal to the pipeline**.
- **Verification:** httptest server asserts signature header verifies, retries observed on first-500, pipeline continues when endpoint unreachable (job still completes). Dual-build gate.

### C3 — Prometheus metrics endpoint
- **Files (create):** `internal/ports/metrics.go` (`MetricsSink` interface: counters/gauges/histograms by name+labels); `internal/adapters/telemetry/prometheus/metrics.go` (+ `_test.go`).
- **Changes:**
  - **Dual-build constraint:** implement Prometheus text exposition format by hand (it is a stable plain-text format) so the restricted build stays dependency-free; prod build may swap internals behind the same interface later. Do not import client_golang into shared code.
  - Metrics per design: `dbvault_jobs_total{type,status}`, `dbvault_bytes_backed_up_total{source}`, `dbvault_job_duration_seconds` histogram, `dbvault_queue_depth` gauge, `dbvault_lease_expired_events_total`, plus `dbvault_notification_delivery_failures_total` from C2.
  - Mount `GET /metrics` in appliance mux outside the auth group (like `/health`), path honoring `metrics.path` config; document in D3 runbook that reverse proxy should ACL `/metrics` to scrape networks (TLS termination layer owns that control).
  - Instrument: scheduler (enqueue/lease/complete/fail/recover), backup executor (bytes/duration), webhook notifier (failures), queue depth gauge polled per tick.
- **Verification:** curl `/metrics` → parseable output containing all five families after a scripted job run; textformat linter check via unit assertion (no duplicate HELP/TYPE lines). Dual-build gate.

### C4 — Truthful doctor
- **Files:** `cmd/dbvault/main.go:436-451` (`doctor`), possibly extract checks to `internal/application/doctor/` (new small package) for testability; `deploy/docker/Dockerfile` + `deploy/compose/demo.yml` healthchecks.
- **Changes:**
  - `--config` becomes required (error + usage + exit 2 when absent — the current static-OK branch dies).
  - Checks, each reported with pass/fail + detail, aggregate exit code non-zero on any failure: (1) config loads+validates; (2) metadata SQLite opens and migrations apply (read-write probe on a temp table); (3) storage backend reachable — filesystem `Validate()` / S3 `HeadBucket` per destination binding; (4) scratch directory writable and free space above watermark (new knob `doctor.min_free_bytes`, default e.g. 1GiB, added to config types + both parsers).
  - Dockerfile HEALTHCHECK → `["/usr/local/bin/dbvault", "doctor", "--config", "/etc/dbvault/dbvault.yaml"]`; demo.yml composes a minimal generated config mounted at that path (updated again properly in D1).
- **Verification:** positive: healthy temp config → exit 0, all four checks printed. Negative (required by design): broken config (bad YAML / unreachable bucket path / undersized disk watermark) → exit non-zero with named failing check. `docker compose -f deploy/compose/demo.yml up` reaches healthy status.

### C5 — Phase C negative + ops test suite
- **Files:** extend `internal/server/security_negative_test.go`; new `scripts/integration/doctor-negative.sh`.
- **Cases:** doctor exits non-zero on each broken-config variant (this is the explicit audit test); `/metrics` does not leak key material (grep response for hex-32 patterns); webhook failure leaves job terminal state intact (already in C2 tests, surfaced here as CI gate).
- **Verification:** shell script runs all variants, asserts exit codes; wired into make target `make verify-hardening` (created in D-phase tooling task).

---

## Phase D — Deployment

### D1 — Production compose profile
- **Files (create):** `deploy/compose/production.yml`, `deploy/compose/production.env.example`.
- **Changes:** Mirrors demo.yml minus `--demo`: mounts real `/etc/dbvault/dbvault.yaml` + keys dir (read-only), `restart: unless-stopped`, healthcheck per C4, optional `minio` service behind a compose profile flag for on-prem object storage, env_file pattern for `DBVAULT_MASTER_KEY` / controller token (documented as file-per-secret alternative). No defaults that silently weaken A-phase posture (image refuses boot without config — enforced by C4 doctor in healthcheck failing fast).
- **Verification:** `docker compose -f deploy/compose/production.yml --env-file production.env.example config` renders; boot against scratch dirs → container healthy, `/api/v1/status` 401 without creds, 200 with.

### D2 — Metadata self-backup systemd timer
- **Files (create):** `deploy/systemd/dbvault-metadata-backup.service`, `deploy/systemd/dbvault-metadata-backup.timer`; runbook section in D3.
- **Changes:** Timer (daily, `Persistent=true`, `RandomizedDelaySec=15m`) triggers oneshot service invoking `dbvault backup-create --config /etc/dbvault/dbvault.yaml` scoped to dbvault's own metadata store into its own vault (filesystem repo path documented; uses the same hardened pipeline — encrypted, signed). Service hardening matches `dbvault.service` sandbox flags. Include restore instructions (restore-run + replace metadata file while service stopped) in the runbook rather than automating destructive steps.
- **Verification:** `systemd-analyze verify deploy/systemd/*.service deploy/systemd/*.timer` clean; dry-run script equivalent executes backup-create successfully against a temp install layout produced by `dbvault install --root /tmp/...`.

### D3 — Ops runbook
- **Files (create):** `docs/ops/RUNBOOK.md`.
- **Sections:** first-boot setup (token capture → admin creation → disable setup replay); key ceremony (generate 32-byte master, storage of DR sheet, HKDF label inventory from A6, legacy-repo warning meaning + re-key open question); restore drill cadence (weekly drill job, IT-E2 procedure adapted to prod); alert triage (webhook payload field reference, metric families from C3, lease-expiry playbook); metadata self-backup restore (D2); reverse-proxy guidance (TLS termination, `/metrics` ACL, HSTS).
- **Verification:** doc review vs. actual flags/endpoints — every command in runbook executed once in a scratch environment during D5 checklist.

### D4 — End-to-end integration stacks (explicit deliverable)
- **Files (create):** `deploy/compose/e2e-fs.yml` + `scripts/integration/e2e-roundtrip.sh` (filesystem leg); extend `deploy/compose/integration.yml` usage via `scripts/integration/e2e-minio.sh` (MinIO leg); `deploy/compose/webhook-mock.yml` (tiny HTTP listener capturing posts) for C2 evidence.
- **Scenarios:**
  - IT-E1 (filesystem round-trip): create source (sqlite fixture) → backup-create → corrupt/delete original → restore-plan + restore-run --replace → byte-compare restored artifact vs original → verify manifest signature validation rejects tampered chunk (flip byte → restore fails loudly).
  - IT-E2 (MinIO round-trip + GC + notify): same round-trip against MinIO destination from existing `integration.yml`; then force retention expiry (tombstone grace=0, keep_last=1) → scheduled GC (C1) reclaims old chunks (assert object count drop via `mc ls`); webhook mock receives success + gc events with valid signatures.
  - IT-M1 from B6 counts toward this deliverable (MySQL credential proof).
- **Verification:** each script `set -euo pipefail`, exit 0 on green path; deliberate-break variants exit non-zero. All runnable via `make verify-hardening`.

### D5 — Full verification sweep (closes the effort)
- **Commands:** dual-build gate; `go test -race ./...` (prod tags, network-enabled env); `make web-test && make web-build`; `make alpha-smoke` (update expectations if auth now gates smoke routes — adjust smoke harness to authenticate via setup flow); run IT-M1, IT-E1, IT-E2; walk audit checklist below item by item.
- **Verification:** all green + checklist table fully mapped.

---

## Verification Checklist — Audit Findings Closure Map

| # | Finding (severity) | Closed by | Proof at close |
|---|---|---|---|
| 1 | Master-key reveal unauthenticated (CRITICAL) | A2, A4, A7-case-2 | Unauth → 401; authed default → **403**; authed + explicit opt-in → 200; no-store headers |
| 2 | Console zero-auth; setup skipped with DBVAULT_DATABASE_URL (CRITICAL) | A2, A5, A7-case-1/4 | Sampled `/api/v1/*` all 401 unauth; setup token always ensured regardless of env; replay of completed setup → 403 |
| 3 | Control-plane auth fails open on empty token (CRITICAL) | A3, A7-case-3 | Binary exits 2 without token; direct construction serves 503; `--insecure-demo` logs loud warning |
| 4 | MySQL backups emit `MYSQL_PWD=""` (CRITICAL) | B4, IT-M1 | Real-container dump contains seeded data; wrong creds fail loudly non-zero |
| 5 | One key triple-duty + zero-pad (HIGH) | A6 | RFC 5869 vector tests; domain-separation break test; short-key rejection; legacy repos warned via manifest version |
| 6 | Simulated scheduler; nothing calls LeaseNext (HIGH) | B1, B2, B3, B5 | Jobs persist across restart; two-worker lease exclusivity; RecoverExpired heals crash; no job motion without `--demo` or real schedule |
| 7 | Retention/GC never automatic (HIGH) | C1, IT-E2 | Post-backup GC enqueued; MinIO object count drops after retention expiry |
| 8 | Doctor prints static OK, exit 0 (HIGH) | C4, C5 | `--config` mandatory; four real checks; non-zero exit on each broken variant; Docker HEALTHCHECK passes config |
| 9 | No alerting, no metrics (MEDIUM) | C2, C3, IT-E2 | Signed webhook deliveries with retries; five metric families exposed on `/metrics`; notification failures logged-not-fatal |
| 10 | Repo tracks node_modules/bin/dist/.opencode; key under scratch/ (MEDIUM) | A1 | `git ls-files` junk count = 0; artifacts remain on disk; `scratch/` ignored |

**Sequencing reminder:** A → B → C → D strictly for merge order; within-phase parallelism noted per task. Open questions carried forward (record in D3): legacy re-key tooling decision; SMTP email deferred behind webhook adapter.
