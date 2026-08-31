---
session: ses_w3_warehouse_evidence
updated: 2026-08-31T16:00:00Z
---

# W3 — Warehouse catalog evidence: durable, truthful, tested — COMPLETE

## What this session did

Continued (post-compaction) the W3 warehouse work: catalogue-backed sync
evidence driving the product-experience warehouse catalog, plus test coverage
for the watermark/incremental boundaries.

### Product code (service_warehouse.go, internal/application/productexperience)
- Extracted two pure helpers from `syncWarehouseTable`:
  - `resolveWarehouseSyncMode(opts, prev)` — full-vs-incremental decision;
    incremental only when requested AND a previously succeeded sync holds a
    cursor; unsafe watermark column fails closed (biIdentifier).
  - `buildWarehouseExtractQuery(table, mode, watermarkColumn, lastWatermark)` —
    full read vs `WHERE col > '<escaped cursor>' ORDER BY col`.
- Incremental appends now partition by sync day for date-typed watermarks:
  `tbl=<t>/dt=<YYYY-MM-DD>/part-<ns>.parquet` (UTC), mkdir of the dt dir added.
- `classifyWatermarkType` hoisted before path selection; watermark comparison
  semantics `watermarkGreater(a,b,typ)` (numeric for integer, lexical else).
- `inspectParquet` converted to a `var` so tests can stub the DuckDB reader.
- Catalog assembly (`GetWarehouseCatalog`) serves catalogue evidence:
  - inventory rows with matching evidence get stored schema/counts/watermark/
    verified flag (`evidenceTableDataset`),
  - evidence for tables the live schema no longer reports still appears,
  - evidence for databases no longer in inventory appears as evidence-only
    databases. No invented columns or counts anywhere.

### Catalogue adapters (evidence store)
- `ports.WarehouseEvidenceStore` wired via `Service.SetWarehouseEvidence`.
- `internal/adapters/catalogue/memory` — in-memory evidence store (used by
  tests and demo wiring).
- `internal/adapters/catalogue/sqlite` — restricted build persists evidence in
  the JSON state file (`warehouse_datasets` map, flushed per upsert); prod
  build uses real SQLite DAO + migration 095 (UNIQUE(database_id, dataset_name)).

### Tests (all passing, restricted AND default/prod builds)
- `internal/application/productexperience/service_warehouse_watermark_test.go`
  - mode boundaries (first sync full; no prior success full; no cursor full;
    full stays full; incremental with cursor; unsafe column errors)
  - extract query builder incl. `'` escaping
  - watermarkGreater numeric-vs-lexical table
  - classifyWatermarkType date/datetime/integer/text/missing("")
  - parseDescribeCSV round trip + empty-schema error (NOTE: nullable always
    false — DESCRIBE CSV parses name+type only)
  - evidenceTableDataset mapping incl. ratio/partition count/stale evidence
  - TestWarehouseCatalogServesEvidence: orphan-db evidence + inventory db
    evidence-only table; verified/unverified provenance asserted
- `internal/adapters/catalogue/memory/catalogue_test.go` — round trip, upsert
  replaces, scoped list, missing get.
- `internal/adapters/catalogue/sqlite/warehouse_evidence_roundtrip_test.go` —
  persistence across Close/reopen in BOTH builds (prod run exercises the real
  SQLite DAO + migration).

### Gotchas discovered
- Sandbox has psql/mysql installed AND ports 5432/3306 OPEN → do NOT write
  catalog tests that depend on DatabaseSchema("production-postgres") live
  tables; evidence-only paths + injected `svc.customDatabases["db-inv"]`
  (engine postgres, live inspect fails fast → zero tables) are deterministic.
- resolveWarehouseSyncMode returns the watermark column even in full mode (so
  dataset records keep their incremental config across full refreshes).
- classifyWatermarkType returns "" for a column absent from the schema.
- sqlite3 CLI creates a file for a missing path in CWD — avoid engine sqlite
  in tests unless path is controlled.

## Verification
- `CGO_ENABLED=0 go test -tags=restricted -count=1 ./...` → all green (exit 0)
- `go vet ./internal/...` → clean (default/prod build)
- `go test ./internal/adapters/catalogue/... ./internal/application/productexperience/` (prod build) → all green

## Next steps (not started)
- W4 (or next task in plan): see thoughts/shared/plans/2026-08-23-warehouse-production-push.md
- UI surfacing of watermark state in the warehouse catalog view (fields are
  already in the API JSON: watermark_column, last_watermark_value,
  last_sync_mode, schema_verified).
- DuckDB contract test for dt= partition pruning once duckdb CLI available.
