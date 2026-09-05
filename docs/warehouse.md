# Warehouse & Analytics

The warehouse pipeline turns live database content into verified Parquet
datasets on local disk, serves them through a governed query/export surface,
and feeds BI tools through scoped connection tokens. Everything the catalog
claims is measured from real artifacts; nothing is invented.

This feature is in alpha. It runs on a single appliance node and depends on
local CLI tooling (see prerequisites).

---

## Prerequisites

| Tool | Required for | Without it |
|---|---|---|
| `duckdb` CLI | Sync (Parquet write + schema verification), warehouse query, export, BI feed | The warehouse surface fails closed with an explicit error |
| `mysql` CLI | Syncing/querying MySQL or MariaDB sources | MySQL extraction fails with an explicit error |
| `psql` CLI | Syncing/querying PostgreSQL sources | PostgreSQL extraction fails with an explicit error |

The server resolves these tools from its `PATH` at use time. The embedded
analytical engine is a DuckDB subprocess; there is no bundled engine binary.

---

## How a sync works

`POST /api/v1/warehouse/sync` (or the `warehouse_sync` schedule operation)
enqueues a durable `warehouse_sync` job. A worker then:

1. Validates the request (known database, analytical runtime available,
   connector resolvable) and fails fast on anything unknown.
2. Discovers the source tables through the live engine.
3. Extracts each table row set through the configured warehouse connector.
4. Writes one Parquet file per dataset under
   `data/lakehouse/db=<database_id>/tbl=<table>/`.
5. Re-reads the written file with DuckDB to verify the on-disk schema and row
   count before recording evidence.
6. Commits per-dataset evidence (schema, measured row count, raw bytes,
   Parquet bytes, watermark state, sync mode, status) to the appliance
   catalogue.

A sync without a verified on-disk schema is recorded as
`succeeded_unverified` rather than silently passing. Every attempt, success
or failure, lands in the audit trail (`warehouse.sync_requested`,
`warehouse.sync_completed`, `warehouse.sync_failed`).

### Full refresh vs incremental

- **Full refresh** (default): the whole table is re-extracted and the
  Parquet artifact is rewritten. The embedded engine view is re-seeded.
- **Incremental** (`"incremental": true`, `"watermark_column": "created_at"`):
  only rows with a watermark value strictly greater than the last recorded
  watermark are extracted and appended as a new partition file. If no
  previous successful sync exists in the catalogue, the job falls back to a
  full refresh and records that fact. When nothing new matches the watermark,
  the sync is an honest no-op: previous measurements are kept, only the sync
  timestamp moves.

Incremental appends are deliberately not re-materialized into the embedded
view: the view would then show only the delta, which misrepresents the
dataset. Re-run a full refresh to rebuild the view.

Crash-window note: a row committed to the source between extraction and
watermark commit can be missed by the next incremental run (watermark moved
past it). Choose a watermark column with that tolerance in mind, or schedule
periodic full refreshes to heal the tail.

### Partition layout

```text
data/lakehouse/
  db=<database_id>/
    tbl=<table>/
      data.parquet                    # full refresh artifact
      dt=2026-08-31/part-*.parquet    # incremental append partitions (date watermarks)
```

Date-typed watermarks partition by sync day; duplicate days resolve
newest-wins when queried.

---

## Connectors

Source credentials live in stored warehouse connectors, never in API
requests or catalogue records. The plaintext secret is written to the
appliance secret store and referenced by path; API responses and the catalogue
carry only the reference.

```bash
# Create a MySQL source connector
curl -X POST -H "Content-Type: application/json" \
  -d '{"name":"app-mysql","kind":"mysql","endpoint":"10.0.0.5:3306",
       "database":"app_db","username":"ro_user","password":"..."}' \
  https://appliance/api/v1/warehouse/connectors

# Verify reachability before relying on it
curl -X POST "https://appliance/api/v1/warehouse/connectors/test?id=<connector_id>"
```

Supported connector kinds: `mysql`, `postgres` (source extraction). The
connector kind must match the target database engine; a mismatch fails the
request explicitly. Updates overlay non-empty fields; a new password replaces
the stored secret. Deleting a connector does not delete sync evidence.

---

## Query, export and masking

- `POST /api/v1/warehouse/query` runs a **read-only** analytical query through
  the embedded engine (or a connector-backed live query when
  `connector_id` is set). The guard rejects multiple statements, writes
  (`DROP/INSERT/UPDATE/DELETE/CREATE/ATTACH/COPY/INSTALL/...`), file-reading
  table functions, and truncates results at 10k rows.
- `POST /api/v1/warehouse/export` returns a CSV or JSON download. Sensitive
  columns are masked **by default** before any byte leaves the appliance, and
  every export is audited with its row count and masked column list.
- Exports are hard-capped at 50,000 rows regardless of query result size.

Masking is applied per column name using the same pattern vocabulary the
privacy rules API advertises:

| Column matches | Strategy | Output |
|---|---|---|
| `email` | deterministic hash | `user_<sha256[:4]>@anonymized.internal` |
| `password`, `pass_hash`, `secret` | full redaction | `[REDACTED]` |
| `phone`, `mobile`, `tel` | fixed synthetic | `+1 (555) 010-0000` |
| `card_num`, `cc_number`, `cvv` | full redaction | `[REDACTED]` |
| `api_key`, `auth_token`, `jwt` | deterministic hash | `tk_<sha256[:4]>` |

Email and token masking are deterministic, so repeated exports stay
consistent and BI joins keep working.

---

## BI feed (Power BI)

BI tools never receive appliance credentials. A scoped connection carries a
bearer token (stored hashed) limited to explicit dataset scopes:

```bash
# Create a scoped connection for one dataset
curl -X POST -H "Content-Type: application/json" \
  -d '{"name":"powerbi-prod","provider":"powerbi",
       "datasets":[{"database":"<database_id>","table":"customers"}],
       "expires_in_days":30}' \
  https://appliance/api/v1/bi/connections
# -> {"connection":{"id":"biconn-..."},"token":"bifeed_..."}   (token shown once)

# Consume the feed
curl -H "Authorization: Bearer bifeed_..." \
  "https://appliance/api/v1/bi/powerbi/feed?database=<database_id>&table=customers&format=csv"
```

- Feeds are available **only** for synchronized datasets (the catalog must
  hold real sync evidence for the database).
- Feed output flows through the same masking pipeline as exports.
- Requests without valid connection credentials fail closed; dataset scopes
  are enforced per request. Connections can be rotated and revoked via
  `/api/v1/bi/connections/<id>/rotate` and `.../revoke`.

---

## Scheduling

Warehouse syncs can run on a schedule through the durable job queue:

```yaml
schedules:
  - id: nightly-warehouse-sync
    operation: warehouse_sync
    source: <database_id>      # or all-databases
    cron: "0 2 * * *"          # or every_seconds: 86400
    enabled: true
```

Scheduled runs use the same pipeline and evidence rules as API-triggered
syncs. The appliance persists sync evidence in the catalogue beside the job
queue, so watermarks and measured facts survive restarts; recovered jobs
reappear through `/api/v1/jobs`.

---

## API quick reference

| Route | Purpose |
|---|---|
| `GET /api/v1/warehouse/catalog` | Evidence-backed catalog: per-database tables, measured counts/bytes, compression ratio, watermark state |
| `POST /api/v1/warehouse/query` | Read-only analytical query (connector optional) |
| `POST /api/v1/warehouse/sync` | Enqueue a durable warehouse sync job |
| `GET/POST /api/v1/warehouse/connectors` | List / create connectors (secrets never echoed) |
| `PUT/DELETE /api/v1/warehouse/connectors?id=` | Update / delete a connector |
| `POST /api/v1/warehouse/connectors/test?id=` | Reachability + credential test |
| `POST /api/v1/warehouse/export` | Masked CSV/JSON export (50k row cap) |
| `GET /api/v1/bi/powerbi/catalog` | BI-facing catalog of synchronized datasets |
| `GET /api/v1/bi/powerbi/feed` | Scoped, masked BI feed |
| `GET/POST /api/v1/bi/connections` | List / create scoped BI connections |
| `GET /api/v1/audit/events` | Audit trail incl. `warehouse.*` events |

---

## Limitations (alpha)

- Single-node; the lakehouse lives on local appliance disk.
- Requires `duckdb` (plus engine-specific CLIs) on the server `PATH`.
- The embedded engine materializes full refreshes in-process; very large
  tables are bounded by memory and the 10k query / 50k export caps.
- Incremental appends keep only the watermark heuristic; the crash-tail note
  above applies.
- No concurrent multi-writer protection: schedule syncs so only one worker
  syncs a database at a time.

## Verification

- Unit/service tests: `go test ./internal/application/productexperience/
  ./internal/server/ -run TestWarehouse`
- End-to-end drill (live MySQL -> Parquet -> governed surfaces):
  `./integration/drills/warehouse-roundtrip.sh`
