package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	memory "github.com/dbvault/dbvault/internal/adapters/catalogue/memory"
	jobqueue "github.com/dbvault/dbvault/internal/adapters/jobqueue/sqlite"
	"github.com/dbvault/dbvault/internal/adapters/warehouse"
	"github.com/dbvault/dbvault/internal/domain"
)

// Case set: every warehouse and BI feed route must reject anonymous callers.
func TestWarehouseAndBIRoutesRejectAnonymousCallers(t *testing.T) {
	f := newAuthedFixture(t, Config{Version: "test"})
	routes := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/warehouse/catalog"},
		{http.MethodPost, "/api/v1/warehouse/query"},
		{http.MethodPost, "/api/v1/warehouse/sync"},
		{http.MethodGet, "/api/v1/warehouse/connectors"},
		{http.MethodPost, "/api/v1/warehouse/connectors"},
		{http.MethodDelete, "/api/v1/warehouse/connectors?id=whconn-x"},
		{http.MethodPost, "/api/v1/warehouse/connectors/test?id=whconn-x"},
		{http.MethodPost, "/api/v1/warehouse/export"},
		{http.MethodGet, "/api/v1/bi/powerbi/catalog"},
		{http.MethodGet, "/api/v1/bi/powerbi/feed?database=duckdb&table=orders"},
	}
	for _, rt := range routes {
		res := httptest.NewRecorder()
		f.H.ServeHTTP(res, httptest.NewRequest(rt.method, rt.path, nil))
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s anonymous status=%d want=401 body=%s", rt.method, rt.path, res.Code, res.Body.String())
		}
	}
}

// The query route must reject non-read-only SQL with an explicit error, never
// execute it and never return partial data.
func TestWarehouseQueryRejectsDangerousSQL(t *testing.T) {
	f := newAuthedFixture(t, Config{Version: "test"})
	queries := []string{
		"DROP TABLE users",
		"DROP TABLE users; SELECT 1",
		"SELECT 1; DROP TABLE users",
		"INSERT INTO users VALUES (1)",
		"UPDATE users SET name = 'x'",
		"DELETE FROM users",
		"CREATE TABLE t (id INT)",
		"ATTACH 'duckdb.db' AS other",
		"COPY (SELECT 1) TO '/tmp/out.parquet'",
		"SELECT * FROM read_parquet('/etc/passwd')",
		"SELECT * FROM read_csv_auto('/etc/passwd')",
		"INSTALL httpfs",
		"PRAGMA database_list",
		"",
	}
	for _, q := range queries {
		body, _ := json.Marshal(map[string]any{"query": q})
		res := f.do(t, http.MethodPost, "/api/v1/warehouse/query", string(body))
		if res.Code != http.StatusBadRequest {
			t.Fatalf("query %q status=%d want=400 body=%s", q, res.Code, res.Body.String())
		}
		if !bytes.Contains(res.Body.Bytes(), []byte("error")) {
			t.Fatalf("query %q must return an explicit error JSON, got %s", q, res.Body.String())
		}
	}
}

// Without a durable scheduler the sync endpoint fails closed with 503 instead
// of silently degrading to an in-memory goroutine.
func TestWarehouseSyncFailsClosedWithoutDurableQueue(t *testing.T) {
	f := newAuthedFixture(t, Config{Version: "test"})
	res := f.do(t, http.MethodPost, "/api/v1/warehouse/sync", `{"database_id":"wh-noqueue"}`)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("sync without queue status=%d want=503 body=%s", res.Code, res.Body.String())
	}
	if !bytes.Contains(res.Body.Bytes(), []byte("durable scheduler not configured")) {
		t.Fatalf("fail-closed error must name the missing scheduler: %s", res.Body.String())
	}
}

// With the durable queue wired, POST /warehouse/sync returns 202 with an
// honest queued view, the px job record is listed, and the queue holds a real
// domain.JobWarehouseSync entry that a backup-only lease must not pick up.
func TestWarehouseSyncEnqueuesDurableWarehouseSyncJob(t *testing.T) {
	f := newAuthedFixture(t, Config{Version: "test"})
	q, err := jobqueue.Open(filepath.Join(t.TempDir(), "drill-jobs.db"))
	if err != nil {
		t.Fatalf("queue open: %v", err)
	}
	t.Cleanup(func() { _ = q.Close() })
	f.App.px.SetJobQueue(q)

	res := f.do(t, http.MethodPost, "/api/v1/warehouse/sync", `{"database_id":"wh-route-db"}`)
	if res.Code != http.StatusAccepted {
		t.Fatalf("sync status=%d want=202 body=%s", res.Code, res.Body.String())
	}
	var view domain.JobView
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode sync response: %v body=%s", err, res.Body.String())
	}
	if view.JobType != "warehouse_sync" || view.Status != "queued" || view.ResourceID != "wh-route-db" {
		t.Fatalf("sync view type=%s status=%s resource=%s", view.JobType, view.Status, view.ResourceID)
	}

	// The px record must be immediately visible through the jobs API.
	listed := f.do(t, http.MethodGet, "/api/v1/jobs/"+view.ID, "")
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte(view.ID)) {
		t.Fatalf("jobs API status=%d body=%s", listed.Code, listed.Body.String())
	}

	// Backup-only leases must not steal the warehouse job.
	if _, ok, err := q.LeaseNext(context.Background(), "t-backup", []domain.JobType{domain.JobBackup}, time.Minute); ok || err != nil {
		t.Fatalf("backup lease picked warehouse job (ok=%v err=%v)", ok, err)
	}
	job, ok, err := q.LeaseNext(context.Background(), "t-worker", []domain.JobType{domain.JobWarehouseSync}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("durable lease: ok=%v err=%v", ok, err)
	}
	if job.Type != domain.JobWarehouseSync || job.ResourceID != "wh-route-db" {
		t.Fatalf("leased job type=%s resource=%s", job.Type, job.ResourceID)
	}
}

// Connector CRUD over HTTP: create, list (no secrets ever serialized), update,
// delete, plus validation failures.
func TestWarehouseConnectorCRUDLifecycleHidesSecrets(t *testing.T) {
	f := newAuthedFixture(t, Config{Version: "test"})
	f.App.px.SetWarehouseConnectors(memory.New())

	const secret = "plaintext-s3cret-value"
	createBody, _ := json.Marshal(map[string]string{
		"name": "drill-mysql", "kind": "mysql", "endpoint": "127.0.0.1:3306",
		"database": "app_db", "username": "svc", "password": secret,
	})
	created := f.do(t, http.MethodPost, "/api/v1/warehouse/connectors", string(createBody))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	if bytes.Contains(created.Body.Bytes(), []byte(secret)) {
		t.Fatal("create response leaked the plaintext password")
	}
	var view warehouse.WarehouseConnector
	if err := json.Unmarshal(created.Body.Bytes(), &view); err != nil || view.ID == "" {
		t.Fatalf("create view=%s err=%v", created.Body.String(), err)
	}
	if view.Type != "mysql" || view.Endpoint != "127.0.0.1:3306" || view.Status != "configured" {
		t.Fatalf("create view mismatch: %+v", view)
	}

	listed := f.do(t, http.MethodGet, "/api/v1/warehouse/connectors", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list status=%d", listed.Code)
	}
	if !bytes.Contains(listed.Body.Bytes(), []byte("drill-mysql")) || bytes.Contains(listed.Body.Bytes(), []byte(secret)) {
		t.Fatalf("connector list must show the connector and never the secret: %s", listed.Body.String())
	}

	updateBody, _ := json.Marshal(map[string]string{
		"name": "drill-mysql-renamed", "kind": "mysql", "endpoint": "127.0.0.1:3307",
	})
	updated := f.do(t, http.MethodPut, "/api/v1/warehouse/connectors?id="+view.ID, string(updateBody))
	if updated.Code != http.StatusOK || !bytes.Contains(updated.Body.Bytes(), []byte("drill-mysql-renamed")) {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}

	deleted := f.do(t, http.MethodDelete, "/api/v1/warehouse/connectors?id="+view.ID, "")
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	afterList := f.do(t, http.MethodGet, "/api/v1/warehouse/connectors", "")
	if bytes.Contains(afterList.Body.Bytes(), []byte(view.ID)) {
		t.Fatalf("connector still listed after delete: %s", afterList.Body.String())
	}

	invalid := f.do(t, http.MethodPost, "/api/v1/warehouse/connectors", `{"name":"bad","kind":"oracle","endpoint":"127.0.0.1:1"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid kind status=%d want=400", invalid.Code)
	}
	testUnknown := f.do(t, http.MethodPost, "/api/v1/warehouse/connectors/test?id=whconn-missing", "")
	if testUnknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown connector test status=%d want=400", testUnknown.Code)
	}
}

// The catalog endpoint must serve the evidence the sync pipeline recorded —
// measured counts, on-disk schema, watermark state — not invented values.
func TestWarehouseCatalogServesStoredEvidenceShape(t *testing.T) {
	f := newAuthedFixture(t, Config{Version: "test"})
	store := memory.New()
	f.App.px.SetWarehouseEvidence(store)

	syncedAt := time.Now().UTC().Truncate(time.Second)
	ev := domain.WarehouseDataset{
		ID:                 domain.WarehouseDatasetKey("wh-ev-db", "events"),
		DatabaseID:         "wh-ev-db",
		DatasetName:        "events",
		SourceEngine:       "mysql",
		WatermarkColumn:    "created_at",
		WatermarkType:      "date",
		LastWatermarkValue: "2026-08-30",
		LastSyncMode:       "incremental",
		Columns: []domain.WarehouseColumn{
			{Name: "id", Type: "BIGINT"},
			{Name: "created_at", Type: "DATE", Nullable: true},
		},
		RowCount:       42,
		BytesRaw:       1000,
		BytesParquet:   250,
		ParquetPaths:   []string{"/lake/db=wh-ev-db/tbl=events/data.parquet"},
		SchemaVerified: true,
		LastSyncAt:     syncedAt,
		LastSyncStatus: "succeeded",
	}
	if err := store.UpsertWarehouseDataset(context.Background(), ev); err != nil {
		t.Fatalf("upsert evidence: %v", err)
	}

	res := f.do(t, http.MethodGet, "/api/v1/warehouse/catalog", "")
	if res.Code != http.StatusOK {
		t.Fatalf("catalog status=%d body=%s", res.Code, res.Body.String())
	}
	var catalog warehouse.WarehouseCatalog
	if err := json.Unmarshal(res.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	var table *warehouse.TableDataset
	for i := range catalog.Databases {
		if catalog.Databases[i].ID == "wh-ev-db" {
			for j := range catalog.Databases[i].Tables {
				if catalog.Databases[i].Tables[j].Name == "events" {
					table = &catalog.Databases[i].Tables[j]
				}
			}
		}
	}
	if table == nil {
		t.Fatalf("evidence-backed table missing from catalog: %s", res.Body.String())
	}
	if table.RowCount != 42 || table.ParquetSizeBytes != 250 || table.UncompressedBytes != 1000 {
		t.Fatalf("evidence counts: %+v", table)
	}
	if table.CompressionRatio != 4 {
		t.Fatalf("ratio=%v want 4", table.CompressionRatio)
	}
	if !table.SchemaVerified || table.SyncStatus != "synced" || !table.LastSyncedAt.Equal(syncedAt) {
		t.Fatalf("provenance: %+v", table)
	}
	if table.WatermarkColumn != "created_at" || table.LastWatermarkValue != "2026-08-30" || table.LastSyncMode != "incremental" {
		t.Fatalf("watermark state: %+v", table)
	}
	if len(table.Columns) != 2 || table.Columns[0].Name != "id" || table.Columns[1].Type != "DATE" {
		t.Fatalf("schema columns: %+v", table.Columns)
	}
}

// Export masking and the hard row cap run against the real query engine, so
// the test needs a CLI; it skips with a note when none is installed.
func warehouseQueryCLIAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("duckdb"); err == nil {
		return
	}
	if _, err := exec.LookPath("sqlite3"); err == nil {
		return
	}
	t.Skip("no duckdb or sqlite3 CLI in PATH; export happy path requires a real query engine")
}

func TestWarehouseExportMasksSensitiveColumnsByDefault(t *testing.T) {
	warehouseQueryCLIAvailable(t)
	f := newAuthedFixture(t, Config{Version: "test"})

	body, _ := json.Marshal(map[string]string{
		"query":  "SELECT 'ada@example.test' AS email, 'hunter2' AS password, 'Ada Lovelace' AS name",
		"format": "csv",
	})
	res := f.do(t, http.MethodPost, "/api/v1/warehouse/export", string(body))
	if res.Code != http.StatusOK {
		t.Fatalf("export status=%d body=%s", res.Code, res.Body.String())
	}
	if ct := res.Header().Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Fatalf("content type=%q want text/csv", ct)
	}
	if cd := res.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment;") || !strings.Contains(cd, ".csv") {
		t.Fatalf("content disposition=%q", cd)
	}
	out := res.Body.String()
	if strings.Contains(out, "ada@example.test") || strings.Contains(out, "hunter2") {
		t.Fatalf("export leaked sensitive values: %s", out)
	}
	if !strings.Contains(out, "@anonymized.internal") || !strings.Contains(out, "[REDACTED]") || !strings.Contains(out, "Ada Lovelace") {
		t.Fatalf("export must mask email/password and keep plain columns: %s", out)
	}
}

func TestWarehouseExportEnforcesHardRowCap(t *testing.T) {
	warehouseQueryCLIAvailable(t)
	f := newAuthedFixture(t, Config{Version: "test"})

	body, _ := json.Marshal(map[string]string{
		"query":  "WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM seq WHERE i < 51000) SELECT i AS seq_value, 'r' AS marker FROM seq LIMIT 51000",
		"format": "csv",
	})
	res := f.do(t, http.MethodPost, "/api/v1/warehouse/export", string(body))
	if res.Code != http.StatusOK {
		t.Fatalf("export status=%d body=%s", res.Code, res.Body.String())
	}
	// 50000 capped rows + one header line; the 50001st data row is gone.
	if got := strings.Count(res.Body.String(), "\n"); got != 50001 {
		t.Fatalf("export line count=%d want 50001 (header + 50000 capped rows)", got)
	}
	header := strings.SplitN(res.Body.String(), "\n", 2)[0]
	if !strings.Contains(header, "seq_value") || !strings.Contains(header, "marker") {
		t.Fatalf("unexpected CSV header: %q", header)
	}
}
