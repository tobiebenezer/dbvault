---
date: 2026-08-23
topic: Warehouse & Analytics production push (honesty, durable syncs, real schema/incremental, connectors/governance, tests/drill/docs)
status: draft
---

# Warehouse Production Push — Implementation Plan

> **Execution:** Work through chunks W1→W5 sequentially. Each chunk is independently committable; do not start a chunk until the previous one is verified and committed.

**Goal:** Take the "Warehouse & Analytics" feature from demo-polished/partially-real to production grade: no fabricated data, syncs that survive restart, real schema + incremental watermarks, real connector management with governance, and drill/test/doc coverage.

**Architecture:** Keep the single-light-binary constraint (CGO_ENABLED=0; restricted build tag stays dependency-free). Reuse the proven durable job infrastructure (`ports.JobQueue` + `internal/adapters/jobqueue/sqlite` with prod/restricted split) instead of the in-memory job store. Sync evidence (schema, row counts, byte sizes, watermarks) moves into catalogue-backed tables via new migrations. Connector creds reuse the `config.SecretReference` resolution pattern.

**Tech Stack:** Go stdlib only (net/http, database/sql+sqlite driver already vendored per existing adapters), DuckDB/psql/mysql CLIs as external prerequisites, existing Prom metrics + webhook notifier, vanilla-JS frontend built by `web/scripts/build-web.mjs`.

**Estimated Chunks:** 5 (~W1 half-day, W2 half-day, W3 full day, W4 full day, W5 half-day)

---

## Environment Constraints (apply to every chunk)

- System `go` is BROKEN. All Go commands MUST use `/tmp/opencode/go/bin/go`.
- Every verification runs **twice**: default build tag AND `-tags restricted`.
- Standard verification block (run after each chunk):

```bash
cd /home/sifu/Documents/dbvault
export PATH=/tmp/opencode/go/bin:$PATH
CGO_ENABLED=0 go vet ./... && CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go vet -tags restricted ./... && CGO_ENABLED=0 go build -tags restricted ./... && CGO_ENABLED=0 go test -tags restricted ./...
```

Expected: exit 0 on both, no output from vet.

---

## W1 — HONESTY: remove all fabricated results

**Goal:** The warehouse never reports data it did not compute. Silent-wrong-results path dies; fake compression ratios, invented connectors, demo seeds, fake latency/status fields, and orphaned workers are gone.

### Files

- Modify: `internal/adapters/warehouse/duckdb.go` (`executeInMemory` ~line 250)
- Delete: `internal/adapters/warehouse/parquet_transformer.go` (74 lines)
- Modify: `internal/adapters/warehouse/duckdb_test.go` (remove `TestParquetTransformer` at lines 47–48)
- Modify: `internal/application/productexperience/service_warehouse.go`
- Modify: `internal/application/productexperience/service_jobs.go` (demo seed call inside `executeLiveWarehouseSyncAsync` ~line 170)
- Modify: `web/src/pages/warehouse.jsx` (line ~61)
- Modify: `web/scripts/build-web.mjs` (lines 44–48)
- Delete: `web/src/workers/log-search.worker.js`, `web/src/workers/timeline.worker.js`, `web/src/workers/schema-layout.worker.js`
- Delete tracked copies: any `web/dist/*.worker.js` and `internal/server/web/dist/*.worker.js`

### Design changes

1. **Kill the silent-fallback query path** (`duckdb.go`):
   - `ExecuteQuery` (line 67) currently tries `executeViaDuckDBCLI` → `executeViaSQLiteCLI` → falls back to `executeInMemory(query)` which returns the FULL table ignoring WHERE/JOIN/GROUP BY/LIMIT while reporting success.
   - Preferred fix: delete `executeInMemory` entirely (lines ~250–273) plus its helpers if now unused (`SeedDataset` at 276 is used by demo seeds — see item 4; remove it in the same chunk). After deleting the sqlite3 CLI fallback too? No — keep `executeViaSQLiteCLI` (it is honest SQL execution), only the in-memory full-table return goes.
   - New behavior when neither CLI is available:

   ```go
   return nil, fmt.Errorf("warehouse query unavailable: no duckdb CLI and no sqlite3 CLI found in PATH; install one of them (see docs/warehouse.md)")
   ```

   - Update `duckdb_test.go`: replace any test asserting fallback success with a test asserting the explicit error when CLIs are absent (skip-if-CLI-present pattern like existing drills).
2. **Delete `parquet_transformer.go`** — it computes `bytes/5.4`, writes nothing, invents a ratio. Its only reference is its own test (`duckdb_test.go:47`). Real Parquet writing already lives in `service_warehouse.go` (~268–314, actual COPY ... ZSTD). Remove `TestParquetTransformer`.
3. **Real compression ratio, or none:**
   - In `SyncWarehouse` (the real Parquet COPY path), capture measured values: `bytes_raw` (sum of extracted source bytes) and `bytes_parquet` (stat() of written Parquet files). Persist them wherever partition/sync metadata is already recorded today.
   - `GetWarehouseCatalog` (~85–91): remove hardcoded placeholder columns and the invented ratio. Compute `overall_compression_ratio = bytes_parquet / bytes_raw` only when both are > 0 from recorded evidence; otherwise omit the field from the JSON response entirely (no default value).
   - Remove display-only fake fields from catalog/query responses: any `latency_ms` that wasn't measured (measure it for real around ExecuteQuery — trivial and honest), any `status: "healthy"` style constants not derived from an actual check. Grep the service for literal `"healthy"`, `"5.2"`, `"5.4"` to catch stragglers.
4. **Connectors list tells the truth** (`GetWarehouseConnectors` ~666–704): return only connectors that are actually configured in config/catalogue. Until W4 adds persistence, this returns `{"connectors": []}` plus any connector derived from real config values (e.g., ClickHouse endpoint if set in config). Delete the invented rows. Empty list is correct output.
5. **Remove demo seeds** (`seedDefaultWarehouseTables` usage at `service_jobs.go:170` and its definition in `service_warehouse.go` ~596–663): non-demo builds must not fabricate tables. If a demo mode dataset is genuinely wanted later it can be re-added behind an explicit `--demo` flag decision — not in scope here. Delete `SeedDataset` from duckdb.go along with it.
6. **Frontend stops fabricating** (`web/src/pages/warehouse.jsx:61`): the `.catch(() => ({... overall_compression_ratio: 5.2 ...}))` defaults become `.catch(err => { setError(...); return null })`. When catalog API fails: render an error banner + retry button, not plausible-looking zeros/fives. Same treatment for the other two swallowed failures (`connectors`, `inventory`) — surface errors, don't mask them.
7. **Orphaned workers:** nothing imports `web/src/workers/*`; `build-web.mjs:44–48` blindly copies them so they ship twice (web/dist AND internal/server/web/dist embedded). Delete the three files and the copy loop. Verify no references first:

   ```bash
   grep -rn "\.worker\.js\|log-search\|timeline-worker\|schema-layout" web/src --include="*.js" --include="*.jsx" | grep -v "src/workers"
   ```

   Expected: zero hits.

### Verification

```bash
# honesty greps — all must return nothing:
grep -rn "overall_compression_ratio.*5\.\|bytes / 5\|/ 5\.4" internal web/src
grep -rn "seedDefaultWarehouseTables\|NewParquetTransformer\|executeInMemory" internal
ls web/src/workers 2>&1          # expected: No such file or directory
ls web/dist/*.worker.js 2>&1     # expected: no matches
ls internal/server/web/dist/*.worker.js 2>&1

# rebuild frontend so embedded dist matches:
node web/scripts/build-web.mjs   # or the repo's documented npm script for it

# standard verification block (both tags) — see Environment Constraints
```

Then run the app without duckdb/mysql CLIs on PATH: `PATH=/usr/bin:/bin /tmp/opencode/go/bin/go run ./cmd/dbvault` → warehouse query must return HTTP 503-style explicit error naming missing CLIs; catalog must show empty/zero state with no ratio field.

**Commit:** `feat(warehouse): remove fabricated results, silent query fallback, demo seeds, orphaned workers`

---

## W2 — DURABLE SYNC JOBS: restart-safe scheduling through the real queue

**Goal:** `warehouse_sync` becomes a real `domain.JobType` flowing through `ports.JobQueue`; API enqueues instead of running goroutines; recurring schedule knob drives it; progress visible via existing jobs API; survives restart.

### Files

- Modify: `internal/domain/job.go` (enum at lines 10–18)
- Modify: `internal/application/productexperience/service_jobs.go` (stage map line 35, dispatch at ~157–166, `executeLiveWarehouseSyncAsync` ~166–193)
- Modify: `internal/application/productexperience/service_warehouse.go` (extract sync core into callable executor method)
- Modify: `internal/server/server.go` (`warehouseSync` handler ~1798–1815)
- Modify: `internal/application/scheduler/scheduler.go` (`JobForFire` ~211–220)
- Modify: `cmd/dbvault/main.go` (worker pool Types at ~986, executor wiring ~940s, queue handoff to px)

### Design changes

1. **Enum entry** (`domain/job.go` after line 18):

   ```go
   JobWarehouseSync JobType = "warehouse_sync"
   ```

   String value must match the literal already used by the stage map (`service_jobs.go:35`) and queue filters.

2. **Durable enqueue from the API handler.** `Appliance.px` is the seam (`server.go:69`). Add to `productexperience.Service`:

   ```go
   func (s *Service) SetJobQueue(q ports.JobQueue) { s.jobQueue = q }

   func (s *Service) EnqueueWarehouseSync(ctx context.Context, databaseID string) (string, error) {
       if s.jobQueue == nil {
           return "", fmt.Errorf("warehouse sync queue unavailable: durable scheduler not configured")
       }
       job := domain.Job{
           ID:          domain.JobID(fmt.Sprintf("whsync-%s-%d", databaseID, time.Now().Unix())),
           Type:        domain.JobWarehouseSync,
           Status:      domain.JobPending,
           ResourceID:  databaseID,
           PayloadJSON: []byte(fmt.Sprintf(`{"source":%q,"operation":"warehouse_sync"}`, databaseID)),
       }
       if err := s.jobQueue.Enqueue(ctx, job); err != nil { return "", err }
       // also create the lightweight px job record so existing jobs UI/API lists it immediately
       ...
   }
   ```

   Wire `px.SetJobQueue(q)` in `main.go` right after `sched := scheduler.New(...)` (queue `q` already exists there). Handler `warehouseSync` switches from `a.px.CreateJob("warehouse_sync", dbID, dbID)` + async goroutine to `a.px.EnqueueWarehouseSync(r.Context(), dbID)`. Fail closed (503) when queue is nil rather than silently degrading.

3. **Executor.** `scheduler.WorkerOptions.Executor` (interface at `worker.go:16`) is currently a backup-only `ExecutorFunc` in main.go. Extend it:

   ```go
   executor := scheduler.ExecutorFunc(func(ctx context.Context, job domain.Job) ([]byte, error) {
       switch job.Type {
       case domain.JobBackup:   // existing body unchanged
       case domain.JobWarehouseSync:
           return px.ExecuteWarehouseSyncJob(ctx, job) // wraps ValidateWarehouseSync + SyncWarehouse, returns result JSON
       default:
           return nil, fmt.Errorf("unsupported job type %q", job.Type)
       }
   })
   ```

   Move the body of `executeLiveWarehouseSyncAsync`'s non-demo branch into `ExecuteWarehouseSyncJob` (minus goroutine plumbing and demo seed branch deleted in W1). Stage updates keep using the px stage/log helpers so `/api/v1/jobs*` keeps showing progress; on completion also mark the px job record complete (existing `updateJobStage(jobID, "complete", 100, ...)` calls suffice).

4. **Worker pool subscribes** (`main.go:986`):

   ```go
   Types: []domain.JobType{domain.JobBackup, domain.JobGarbageCollection, domain.JobWarehouseSync},
   ```

   (The `worker.go:32,59` defaults stay as-is; main.go's explicit Types is what runs.)

5. **Scheduler emits the right type** (`scheduler.go` `JobForFire`): replace hard-coded `Type: domain.JobBackup` with a mapping:

   ```go
   jt := domain.JobBackup
   if spec.Operation == "warehouse_sync" { jt = domain.JobWarehouseSync }
   ```

   Deterministic ID `sched-<specID>-<unix>` already gives restart-safe dedupe via idempotent Enqueue — free survival across restarts.

6. **Schedule knob — reuse existing parsing, do NOT add a new config shape.** `ScheduleConfig` (`config/types.go:305`: ID/Source/Operation/Cron/Timezone/Enabled, `EverySeconds` fallback) already flows through `SpecsFromConfig` (`scheduler.go:28`) which passes `Operation` straight into specs. Users configure:

   ```yaml
   schedules:
     - id: wh-nightly
       operation: warehouse_sync
       source: <database-id>        # empty => sync all known datasets
       cron: "0 2 * * *"
       timezone: UTC
   ```

   Add validation in `Normalize()` (or wherever schedules are validated): reject unknown operations except `logical_backup` and `warehouse_sync`.

7. **Restart proof:** jobs persisted in SQLite queue (prod) / atomic JSON file (restricted) already handle lease recovery via `RecoverExpired`. Manual test below proves it.

### Verification

```bash
# standard verification block (both tags)
/tmp/opencode/go/bin/go test ./internal/adapters/jobqueue/... ./internal/application/scheduler/... -run .
```

Manual drill:
1. Start appliance with a MySQL source configured; POST `/api/v1/warehouse/sync`; observe job appears in `/api/v1/jobs` and progresses to complete.
2. POST again, then SIGKILL the process mid-sync; restart → `RecoverExpired` re-leases the job; sync completes after restart (check job log + parquet mtime).
3. Add `schedules:` entry with `operation: warehouse_sync`, `every_seconds: 120`; wait two ticks → exactly one new sync job per window (dedupe holds); kill/restart → no duplicate replays.

**Commit:** `feat(warehouse): route warehouse_sync through durable job queue with cron/every_seconds scheduling`

---

## W3 — REAL SCHEMA + INCREMENTAL SYNC: evidence-backed catalog and watermark mode

**Goal:** Catalog serves schema/row-counts/sizes read back from real Parquet artifacts after each sync; optional per-dataset watermark column enables incremental extraction; date-typed watermarks produce `dt=<date>` partitions.

### Files

- Create: `migrations/095_warehouse_datasets.sql`
- Modify: `internal/adapters/catalogue/sqlite/catalogue.go` + `catalogue_prod.go` (new DAO methods following existing table-DAO pattern)
- Modify: `internal/ports/` (add catalogue port method(s) per existing interface layout)
- Modify: `internal/application/productexperience/service_warehouse.go` (post-sync introspection, incremental extraction, catalog reads)
- Test: `internal/application/productexperience/service_warehouse_test.go` (watermark unit tests)

### Migration sketch (`migrations/095_warehouse_datasets.sql`)

```sql
CREATE TABLE IF NOT EXISTS warehouse_datasets (
    id                   TEXT PRIMARY KEY,
    database_id          TEXT NOT NULL,
    dataset_name         TEXT NOT NULL,
    source_engine        TEXT NOT NULL,             -- mysql | postgres | clickhouse
    watermark_column     TEXT NOT NULL DEFAULT '',  -- '' = full-refresh only
    last_watermark_value TEXT NOT NULL DEFAULT '',  -- formatted value, lexically comparable per type
    columns_json         TEXT NOT NULL DEFAULT '[]',
    row_count            INTEGER NOT NULL DEFAULT 0,
    bytes_raw            INTEGER NOT NULL DEFAULT 0,
    bytes_parquet        INTEGER NOT NULL DEFAULT 0,
    parquet_paths_json   TEXT NOT NULL DEFAULT '[]',
    last_sync_at         TEXT NOT NULL DEFAULT '',
    last_sync_status     TEXT NOT NULL DEFAULT '',  -- succeeded | failed
    UNIQUE(database_id, dataset_name)
);
CREATE INDEX IF NOT EXISTS idx_wh_datasets_db ON warehouse_datasets(database_id);
```

Next free number confirmed: highest existing migration is `094_realtime_cursors.sql`.

### Design changes

1. **Post-sync introspection.** At the end of the successful Parquet COPY path, read the REAL schema back — never trust the source's word alone:

   ```bash
   duckdb -c "DESCRIBE SELECT * FROM read_parquet('<written.parquet>');"
   duckdb -c "SELECT COUNT(*) FROM read_parquet('<glob>');"
   ```

   plus `stat` for bytes. Persist into `warehouse_datasets` (`columns_json` = name/type/nullity list). This is the same fail-closed spirit as `ValidateWarehouseSync` (~377): if duckdb CLI is absent, sync still succeeds but catalog marks `last_sync_status='succeeded_unverified'` with empty columns — explicitly labeled, not placeholder-filled.

2. **Catalog API serves evidence.** `GetWarehouseCatalog` joins inventory against `warehouse_datasets`: real column names/types, row_count, bytes_parquet, computed ratio (from W1 rule). Remove the last placeholder paths. Rows with no sync record appear as `sync_status: "never_synced"`.

3. **Incremental mode.**
   - Sync request gains optional `watermark_column` + `mode: "full"|"incremental"`; accepted values persisted per dataset.
   - Incremental extraction appends to the source query: `WHERE <watermark_col> > '<last_watermark_value>'` (parameterized/escaped per engine adapter — psql/mysql CLI invocation code at `service_warehouse.go` ~425–500 is where the query string is built; escape identifiers the same way the rest of the file does).
   - On success: persist new MAX(watermark) as `last_watermark_value`. Crash between write and watermark update ⇒ next run re-extracts the tail (idempotent because Parquet writes are new files; dedupe happens at query time via newest-partition-wins or exact-replace of that partition — document choice in `docs/warehouse.md`).
   - `mode: "full"` ignores watermark, replaces everything, resets watermark to the new MAX.
   - Watermark formatting rules (unit-tested): date/datetime → `2006-01-02` / RFC3339; integers → decimal string. Comparison must be monotonic strings within one type — enforce by storing type alongside in `columns_json` and validating new watermark > old before persisting.

4. **Partition layout.** When watermark column is date-typed, written files land as `<base>/<db>/<dataset>/dt=<YYYY-MM-DD>/data_*.parquet` (matches the existing partition-dir conventions used by the COPY path). Non-date watermarks keep flat timestamped files. Catalog `parquet_paths_json` records actual paths written.

5. **Restricted-tag check:** migration + DAO use the same sqlite driver abstraction as migrations 001–094 — confirm `go test -tags restricted ./internal/adapters/catalogue/...` stays green (no cgo).

### Verification

```bash
# standard verification block (both tags)
/tmp/opencode/go/bin/go test ./internal/application/productexperience/ -run TestWatermark -v
```

Functional: sync a MySQL table twice with `watermark_column=updated_at`, insert a new row between syncs → second sync extracts exactly 1 row (verify via job log "extracted N rows"), catalog shows cumulative row_count from Parquet COUNT(*), `dt=` dirs exist for both sync dates. Drop watermark → full refresh replaces all.

**Commit:** `feat(warehouse): evidence-backed catalog schema and incremental watermark syncs`

---

## W4 — CONNECTORS + GOVERNANCE: real CRUD, honored endpoints, masking, audit trail

**Goal:** Connectors are user-managed resources with secret refs; sync/query actually use them (ClickHouse stops being hardcoded localhost/default); exports pass through PII masking; queries/exports/syncs leave audit events.

### Files

- Create: `migrations/096_warehouse_connectors.sql`
- Modify: `internal/adapters/catalogue/sqlite/catalogue.go` + `catalogue_prod.go` (connector DAO)
- Modify: `internal/config/types.go` (if connector-level secret refs resolve through config loader helpers — follow existing `SecretReference` usage at line 81/118–120)
- Modify: `internal/application/productexperience/service_warehouse.go` (CRUD + resolution + export masking + audit emission)
- Modify: `internal/adapters/warehouse/clickhouse.go` (endpoint/creds from resolved connector)
- Modify: `internal/server/server.go` (routes ~190–195, handlers ~1816–1823)
- Test: extend W5 suite incrementally here

### Design changes

1. **Connector storage.**

   ```sql
   CREATE TABLE IF NOT EXISTS warehouse_connectors (
       id         TEXT PRIMARY KEY,
       name       TEXT NOT NULL UNIQUE,
       kind       TEXT NOT NULL,               -- clickhouse | postgres | mysql
       endpoint   TEXT NOT NULL,               -- host:port or URL
       database   TEXT NOT NULL DEFAULT '',
       username   TEXT NOT NULL DEFAULT '',
       secret_ref TEXT NOT NULL DEFAULT '',    -- SecretReference JSON: env/file ref, never plaintext password
       created_at TEXT NOT NULL,
       updated_at TEXT NOT NULL
   );
   ```

   Password materializes only at use-time via the existing `SecretReference` resolution (`types.go:81`, `.Empty()` helper). Config-file alternative rejected: CRUD would require restarts and config writes from a server process.

2. **CRUD routes** (mux pattern-consistent with neighbors):

   ```
   GET    /api/v1/warehouse/connectors          -> list (no secrets ever serialized)
   POST   /api/v1/warehouse/connectors          -> create/update (body carries secret_ref, not secret)
   DELETE /api/v1/warehouse/connectors?id=...   -> delete
   POST   /api/v1/warehouse/connectors/test     -> dial + trivial query, report latency measured for real
   ```

   `GetWarehouseConnectors` now reads this table (W1 left it returning configured-only; W4 makes "configured" mean these rows).

3. **Honor connector_id end-to-end.** Sync request and query request gain `connector_id`. Resolution chain: explicit `connector_id` → error if unknown; ClickHouse engine (`clickhouse.go`) takes addr/user/password from resolved connector instead of hardcoded localhost/default. Postgres/MySQL extraction CLIs take `-h/-P/-u` + `MYSQL_PWD`/`PGPASSWORD` from the connector too (replacing ambient-env reliance at `service_warehouse.go` ~425–500).

4. **Export masking.** The `MaskingRule` catalog (`service_business.go:54–110`) defines patterns + strategies but is display-only today; the only enforcement is the sandbox `mask_pii` SQL stage (`service_jobs.go:768`). For warehouse exports, implement an in-process applier in `service_warehouse.go`:

   ```go
   func applyMaskingRules(cols []ColumnMeta, rows [][]any, rules []MaskingRule) ([][]any, int)
   ```

   - Match `ColumnPattern` globs (`*email*` etc.) case-insensitively against result column names.
   - Strategy mapping: Redact → `[REDACTED]`; Nullify → nil; hash strategies → deterministic SHA-256 prefix (stable across exports, still anonymizing); Synthetic → fixed-format synthetic value seeded per-cell-hash (documented, not random).
   - Applied by default in `ExportWarehouseResults` (CSV/JSON paths); `?mask=false` requires an explicit opt-out that itself emits an audit event. Count of masked cells returned in response header/metadata.

5. **Audit events.** Reuse `RecordAuditEvent(eventType, actorType, actorID, resourceType, resourceID, outcome, metadata)` (`service_business.go:475`) — it's the established pattern and prod persists via catalogue `AppendAudit` (`catalogue_prod.go:233`). Emit on:
   - `warehouse.query` (actor from auth ctx, metadata: engine, database, masked=yes/no)
   - `warehouse.export` (+ format, row count, masked cell count)
   - `warehouse.sync_requested` / `warehouse.sync_completed|failed` (dataset, mode, rows extracted)

### Verification

```bash
# standard verification block (both tags)
grep -rn "localhost\|9000" internal/adapters/warehouse/clickhouse.go   # expected: only as documented default when NO connector resolves
```

Functional: create ClickHouse connector via POST with `secret_ref` pointing at an env var → `test` returns real latency → sync/query with `connector_id` hit that host (verify via listener logs) → export CSV containing a column named `email` shows `[REDACTED]`-strategy output → `/api/v1/audit/events` contains all four event types. DELETE connector → subsequent sync fails with explicit unknown-connector error.

**Commit:** `feat(warehouse): connector CRUD with secret refs, honored endpoints, PII-masked exports, audit events`

---

## W5 — TESTS + DRILL + DOCS: prove it, rehearse it, write it down

**Goal:** Route-level httptest coverage including auth-negative cases, unit tests for watermark/schema propagation, an executable roundtrip drill, and user-facing documentation.

### Files

- Create: `internal/server/warehouse_test.go`
- Create: `internal/application/productexperience/service_warehouse_test.go` (watermark logic, masking applier, catalog-from-evidence)
- Create: `integration/drills/warehouse-roundtrip.sh`
- Create: `docs/warehouse.md`
- Modify: `README.md` (product-state section)

### Design changes

1. **HTTP tests** — follow `internal/server/security_negative_test.go` + `bi_test.go` harness patterns. Cover all five routes (`catalog`, `query`, `sync`, `connectors`, `export`):
   - Anonymous request → 401 on every route (auth boundary already wraps `/api/v1/warehouse/*`; assert it stays true).
   - Authenticated happy paths against stubbed px service (pattern from `server_test.go`).
   - Query with no CLIs installed → explicit error JSON, never partial data (locks in W1).
   - Sync enqueues durable job (assert queue received `domain.JobWarehouseSync`, locks in W2).
   - Export masks email-like column by default (locks in W4).
2. **Unit tests:**
   - Watermark: formatting per type, monotonic comparison, reject-regression guard, incremental WHERE clause construction (incl. identifier escaping), `dt=` path derivation for date-typed columns.
   - Schema propagation: DESCRIBE output parser → `columns_json` round-trip; catalog renders evidence rows and `never_synced` for gaps.
   - Masking applier: each strategy × pattern-match matrix.
3. **Drill** `integration/drills/warehouse-roundtrip.sh` — clone structure from `mysql-roundtrip.sh` and reuse `lib/common.sh` helpers (`keygen` etc.). Flow: spin MySQL container → load tiny fixture (users table incl. `email` col) → configure connector + trigger sync via API → assert Parquet files exist with `dt=` layout → query via duckdb CLI → assert row counts and masked-export behavior → teardown. Header convention: if `duckdb` CLI missing, `skip-with-note` exactly like existing drills skip on missing docker/images.
4. **Docs** `docs/warehouse.md`: what it does; CLI prerequisites (duckdb, mysql, psql — how to install, what degrades without each); sync modes (full vs incremental, watermark semantics + crash-tail caveat); connector setup incl. SecretReference examples; BI feed auth pointer (`AuthorizeBIFeed` scoped tokens, `service_bi.go` ~203); schedule config example; limitations section (single-node, CLI-dependent, no concurrent multi-writer guarantee).
5. **README product-state:** move Warehouse & Analytics from "partial/demo" wording to production-state paragraph matching reality post-W1–W4 (honest about CLI prerequisites and single-appliance scope).

### Verification

```bash
# standard verification block (both tags) — includes new tests
/tmp/opencode/go/bin/go test ./internal/server/ -run Warehouse -v
/tmp/opencode/go/bin/go test ./internal/application/productexperience/ -v
bash integration/drills/warehouse-roundtrip.sh          # green with CLIs present
bash integration/drills/warehouse-roundtrip.sh          # with duckdb removed from PATH: SKIP + note, exit 0
```

**Commit:** `test(warehouse): httptest coverage, watermark/masking units, roundtrip drill, docs`

---

## Acceptance Checklist — research gap → chunk mapping

| # | Verified gap | Closed in |
|---|---|---|
| 1 | `executeInMemory` ignores WHERE/JOIN/GROUP BY/LIMIT, returns full table as success when no CLI present (`duckdb.go:250–273`) | W1 |
| 2 | `parquet_transformer.go` computes bytes/5.4, writes nothing, invents ratio | W1 |
| 3 | Fabricated `overall_compression_ratio: 5.2` in frontend failure-catch (`warehouse.jsx:61`) | W1 |
| 4 | Invented connector rows (`GetWarehouseConnectors` ~666–704) + fake latency/status fields | W1 (truthful empty) → W4 (real CRUD) |
| 5 | Demo seeds fabricate warehouse tables (~596–663, `service_jobs.go:170`) | W1 |
| 6 | Orphaned workers shipped twice (`web/src/workers/*`, `build-web.mjs:44–46`) | W1 |
| 7 | `warehouse_sync` runs on in-memory job store; vanishes on restart (`service_jobs.go:35,157–192`) | W2 |
| 8 | Placeholder catalog columns/counts/sizes, partition metadata display-only (`GetWarehouseCatalog` ~85–91) | W3 |
| 9 | No incremental sync / watermark support | W3 |
| 10 | Exports bypass PII masking; no audit events on query/export/sync | W4 |

Cross-cutting acceptance (all must hold at final commit):

- [ ] Both build tags green: default AND `-tags restricted` (vet/build/test) via `/tmp/opencode/go/bin/go`
- [ ] Zero hits on fabrication greps from W1 verification
- [ ] Sync survives SIGKILL + restart (W2 manual drill)
- [ ] Catalog numbers traceable to Parquet files on disk (W3)
- [ ] ClickHouse works against non-default endpoint via connector (W4)
- [ ] Audit log contains query/export/sync events with actor attribution (W4)
- [ ] Drill passes (or documents clean skip) on this machine (W5)
- [ ] `docs/warehouse.md` accurate against actual behavior, README product-state updated (W5)
