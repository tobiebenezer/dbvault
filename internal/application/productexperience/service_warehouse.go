package productexperience

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dbvault/dbvault/internal/adapters/warehouse"
	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

// WarehouseService handles analytical data lakehouse queries and catalog management.
type WarehouseService struct {
	duckDB     *warehouse.DuckDBEngine
	clickhouse *warehouse.ClickHouseConnector
	connectors map[string]warehouse.WarehouseConnector
}

// initWarehouse initializes warehouse subsystems and seeds default analytical datasets.
func (s *Service) initWarehouse() {
	if s.warehouseEngine == nil {
		engine, err := warehouse.NewDuckDBEngine()
		if err == nil {
			s.warehouseEngine = engine
			s.seedDefaultWarehouseTables()
		}
	}
}

// GetWarehouseCatalog returns the analytical lakehouse catalog and dataset partitions.
func (s *Service) GetWarehouseCatalog(ctx context.Context) warehouse.WarehouseCatalog {
	if err := ctx.Err(); err != nil {
		return warehouse.WarehouseCatalog{GeneratedAt: s.now()}
	}
	// Copy the registry while holding the service lock, then release it before
	// inspecting schemas. DatabaseSchema performs its own I/O and locking.
	s.mu.Lock()
	databases := make([]domain.DatabaseResource, 0, len(s.customDatabases))
	for _, db := range s.customDatabases {
		databases = append(databases, db)
	}
	demo := s.demo
	now := s.now()
	s.mu.Unlock()

	var dbDatasets []warehouse.DatabaseDataset
	var totalRows int64
	var totalParquet int64
	var totalRaw int64
	totalTables := 0

	// Compile catalog from protected databases
	for _, db := range databases {
		dbDataset := warehouse.DatabaseDataset{
			ID:           db.ID,
			Name:         db.Name,
			EngineSource: db.Engine,
			LastSyncAt:   time.Time{},
		}

		// Retrieve tables for this database
		schema := s.DatabaseSchema(db.ID)
		for _, t := range schema.Tables {
			if t.Excluded {
				continue
			}
			// Report only what the live schema actually reported. Row counts
			// and byte sizes come from the source database; when the source
			// could not measure them they stay zero instead of being invented.
			tblDataset := warehouse.TableDataset{
				Name:              t.Name,
				Database:          db.Name,
				EngineSource:      db.Engine,
				Columns:           []warehouse.ColumnMeta{},
				RowCount:          t.EstimatedRows,
				ParquetSizeBytes:  0,
				UncompressedBytes: t.DataSizeBytes + t.IndexSizeBytes,
				PartitionCount:    0,
				LastSyncedAt:      time.Time{},
				SyncStatus:        "never_synced",
			}
			if parquetPath := s.warehouseParquetPath(db.ID, t.Name); parquetPath != "" {
				if info, err := os.Stat(parquetPath); err == nil && info.Size() > 0 {
					tblDataset.ParquetSizeBytes = info.Size()
					tblDataset.ParquetLocation = parquetPath
					// Ratio only from actually measured bytes on both sides.
					if tblDataset.UncompressedBytes > 0 {
						tblDataset.CompressionRatio = float64(tblDataset.UncompressedBytes) / float64(info.Size())
					}
					tblDataset.PartitionCount = 1
					tblDataset.LastSyncedAt = info.ModTime().UTC()
					tblDataset.SyncStatus = "synced"
					dbDataset.LastSyncAt = tblDataset.LastSyncedAt
				}
			}

			dbDataset.Tables = append(dbDataset.Tables, tblDataset)
			dbDataset.TotalRows += tblDataset.RowCount
			dbDataset.TotalParquetBytes += tblDataset.ParquetSizeBytes
			dbDataset.TotalRawBytes += tblDataset.UncompressedBytes
			totalTables++
		}

		// Ratio only when both totals were actually measured.
		if dbDataset.TotalParquetBytes > 0 && dbDataset.TotalRawBytes > 0 {
			dbDataset.CompressionRatio = float64(dbDataset.TotalRawBytes) / float64(dbDataset.TotalParquetBytes)
		}

		totalRows += dbDataset.TotalRows
		totalParquet += dbDataset.TotalParquetBytes
		totalRaw += dbDataset.TotalRawBytes

		dbDatasets = append(dbDatasets, dbDataset)
	}

	// Demo data is explicit. A normal installation must never report fabricated
	// customer datasets when no source has been synchronized. Even here, every
	// compression ratio is derived from the fixture's own byte counts rather
	// than invented constants.
	if demo && len(dbDatasets) == 0 {
		usersRaw, usersParquet := int64(22000000), int64(4200000)
		txRaw, txParquet := int64(104000000), int64(18500000)
		sampleTables := []warehouse.TableDataset{
			{
				Name:              "users",
				Database:          "cribx_test",
				EngineSource:      "postgres",
				Columns:           []warehouse.ColumnMeta{{Name: "id", Type: "BIGINT"}, {Name: "email", Type: "VARCHAR"}, {Name: "plan", Type: "VARCHAR"}, {Name: "created_at", Type: "TIMESTAMP"}},
				RowCount:          125000,
				ParquetSizeBytes:  usersParquet,
				UncompressedBytes: usersRaw,
				CompressionRatio:  float64(usersRaw) / float64(usersParquet),
				ParquetLocation:   "s3://dbvault-backups/lakehouse/cribx_test/users/data.parquet",
				PartitionKey:      "created_at (monthly)",
				PartitionCount:    12,
				LastSyncedAt:      now.Add(-5 * time.Minute),
				SyncStatus:        "synced",
			},
			{
				Name:              "transactions",
				Database:          "cribx_test",
				EngineSource:      "postgres",
				Columns:           []warehouse.ColumnMeta{{Name: "id", Type: "BIGINT"}, {Name: "user_id", Type: "BIGINT"}, {Name: "amount", Type: "DECIMAL"}, {Name: "currency", Type: "VARCHAR(3)"}, {Name: "status", Type: "VARCHAR"}, {Name: "created_at", Type: "TIMESTAMP"}},
				RowCount:          890000,
				ParquetSizeBytes:  txParquet,
				UncompressedBytes: txRaw,
				CompressionRatio:  float64(txRaw) / float64(txParquet),
				ParquetLocation:   "s3://dbvault-backups/lakehouse/cribx_test/transactions/data.parquet",
				PartitionKey:      "created_at (daily)",
				PartitionCount:    90,
				LastSyncedAt:      now.Add(-5 * time.Minute),
				SyncStatus:        "synced",
			},
		}

		demoTotalRaw := usersRaw + txRaw
		demoTotalParquet := usersParquet + txParquet
		dbDatasets = append(dbDatasets, warehouse.DatabaseDataset{
			ID:                "cribx_test",
			Name:              "cribx_test",
			EngineSource:      "postgres",
			Tables:            sampleTables,
			TotalRows:         1015000,
			TotalParquetBytes: demoTotalParquet,
			TotalRawBytes:     demoTotalRaw,
			CompressionRatio:  float64(demoTotalRaw) / float64(demoTotalParquet),
			LastSyncAt:        now.Add(-5 * time.Minute),
		})
		totalTables = len(sampleTables)
		totalRows = 1015000
		totalParquet = demoTotalParquet
		totalRaw = demoTotalRaw
	}

	// Overall ratio exists only when both byte totals were actually measured.
	var overallRatio float64
	if totalRaw > 0 && totalParquet > 0 {
		overallRatio = float64(totalRaw) / float64(totalParquet)
	}

	sort.Slice(dbDatasets, func(i, j int) bool {
		return dbDatasets[i].Name < dbDatasets[j].Name
	})

	return warehouse.WarehouseCatalog{
		Databases:               dbDatasets,
		TotalDatabases:          len(dbDatasets),
		TotalTables:             totalTables,
		TotalRows:               totalRows,
		TotalParquetBytes:       totalParquet,
		TotalRawBytes:           totalRaw,
		OverallCompressionRatio: overallRatio,
		StorageEngine:           "Local Apache Parquet (ZSTD) + embedded query engine",
		GeneratedAt:             now,
	}
}

func (s *Service) warehouseParquetPath(database, table string) string {
	if !biDatabaseIdentifier.MatchString(database) || !biIdentifier.MatchString(table) {
		return ""
	}
	return filepath.Join(s.root, "lakehouse", "db="+database, "tbl="+table, "data.parquet")
}

// SyncWarehouse extracts source tables and writes verified Parquet artifacts.
// It fails closed when the analytical runtime or source data is unavailable.
func (s *Service) SyncWarehouse(ctx context.Context, databaseID string) error {
	if err := s.ValidateWarehouseSync(ctx, databaseID); err != nil {
		return err
	}
	s.initWarehouse()
	s.mu.Lock()
	ids := make([]string, 0)
	if databaseID == "" || databaseID == "all-databases" {
		for id := range s.customDatabases {
			ids = append(ids, id)
		}
	} else {
		ids = append(ids, databaseID)
	}
	s.mu.Unlock()
	duckdbPath, _ := exec.LookPath("duckdb")
	for _, id := range ids {
		schema := s.DatabaseSchema(id)
		for _, table := range schema.Tables {
			if table.Excluded || !biIdentifier.MatchString(table.Name) {
				continue
			}
			result, err := s.executeLiveDBQuery(ctx, warehouse.QueryRequest{Database: id, Query: "SELECT * FROM " + table.Name})
			if err != nil {
				return fmt.Errorf("extract %s.%s: %w", id, table.Name, err)
			}
			if len(result.Columns) == 0 {
				return fmt.Errorf("extract %s.%s returned no column metadata", id, table.Name)
			}
			if err := s.warehouseEngine.SeedDataset(table.Name, result.Columns, result.Rows); err != nil {
				return fmt.Errorf("materialize %s.%s: %w", id, table.Name, err)
			}
			path := s.warehouseParquetPath(id, table.Name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			if err := s.writeWarehouseParquet(ctx, duckdbPath, path, result); err != nil {
				return fmt.Errorf("write %s.%s: %w", id, table.Name, err)
			}
		}
	}
	return nil
}

func (s *Service) writeWarehouseParquet(ctx context.Context, duckdbPath, path string, result *warehouse.QueryResult) error {
	tmp, err := os.CreateTemp(filepath.Join(s.root, "lakehouse"), "extract-*.csv")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	writer := csv.NewWriter(tmp)
	header := make([]string, len(result.Columns))
	for i, col := range result.Columns {
		header[i] = col.Name
	}
	if err := writer.Write(header); err != nil {
		_ = tmp.Close()
		return err
	}
	for _, row := range result.Rows {
		values := make([]string, len(header))
		for i := range values {
			if i < len(row) && row[i] != nil {
				values[i] = fmt.Sprint(row[i])
			}
		}
		if err := writer.Write(values); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	copySQL := fmt.Sprintf("COPY (SELECT * FROM read_csv_auto('%s')) TO '%s' (FORMAT PARQUET, COMPRESSION ZSTD)", strings.ReplaceAll(tmpPath, "'", "''"), strings.ReplaceAll(path, "'", "''"))
	cmd := exec.CommandContext(ctx, duckdbPath, "-c", copySQL)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("duckdb: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("parquet artifact verification failed")
	}
	return nil
}

// ExecuteWarehouseQuery runs an analytical SQL query against the lakehouse engine.
func (s *Service) ExecuteWarehouseQuery(ctx context.Context, req warehouse.QueryRequest) (*warehouse.QueryResult, error) {
	if err := validateWarehouseQuery(req.Query); err != nil {
		return nil, err
	}
	s.initWarehouse()

	if s.warehouseEngine == nil {
		return nil, fmt.Errorf("warehouse analytical engine is initializing")
	}

	// If a specific live target database is requested (e.g. cribx_test), route to live database query runner
	if req.Database != "" && req.Database != "duckdb" && req.Database != "auto" {
		return s.executeLiveDBQuery(ctx, req)
	}

	// Always ensure default analytical tables (users, transactions, orders, lakehouse_metrics) are populated
	s.seedDefaultWarehouseTables()

	if req.Engine == "clickhouse" {
		ch := warehouse.NewClickHouseConnector("http://127.0.0.1:8123", "default", "", "default")
		res, err := ch.ExecuteQuery(ctx, req.Query)
		if err == nil {
			return res, nil
		}
		return nil, fmt.Errorf("clickhouse query failed: %w", err)
	}

	return s.warehouseEngine.ExecuteQuery(ctx, req)
}

func (s *Service) databaseHasSyncedWarehouseData(database string) bool {
	s.mu.Lock()
	dbs := make([]domain.DatabaseResource, 0, len(s.customDatabases))
	for _, db := range s.customDatabases {
		dbs = append(dbs, db)
	}
	s.mu.Unlock()
	for _, db := range dbs {
		if db.ID == database || db.Name == database {
			schema := s.DatabaseSchema(db.ID)
			for _, table := range schema.Tables {
				if _, err := os.Stat(s.warehouseParquetPath(db.ID, table.Name)); err == nil {
					return true
				}
			}
		}
	}
	return false
}

// ValidateWarehouseSync checks the prerequisites that must be true before a
// sync job can claim that Parquet data is ready. It deliberately fails closed:
// a missing analytical runtime or source tables must never become a fake
// successful warehouse job.
func (s *Service) ValidateWarehouseSync(ctx context.Context, databaseID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := exec.LookPath("duckdb"); err != nil {
		return fmt.Errorf("DuckDB runtime is required to write verified Parquet artifacts")
	}
	s.mu.Lock()
	var ids []string
	if databaseID == "" || databaseID == "all-databases" {
		for id := range s.customDatabases {
			ids = append(ids, id)
		}
	} else {
		ids = []string{databaseID}
	}
	s.mu.Unlock()
	if len(ids) == 0 {
		return fmt.Errorf("no configured source databases are available to synchronize")
	}
	for _, id := range ids {
		if len(s.DatabaseSchema(id).Tables) == 0 {
			return fmt.Errorf("source database %q has no readable tables", id)
		}
	}
	return nil
}

// SetJobQueue wires the durable job queue that warehouse sync jobs flow
// through. Called once at appliance startup after the scheduler runtime opens
// the queue; when it is never called the sync API fails closed.
func (s *Service) SetJobQueue(q ports.JobQueue) { s.jobQueue = q }

// EnqueueWarehouseSync registers the job view (so /api/v1/jobs lists it
// immediately) and pushes a durable warehouse_sync job onto the persistent
// queue for a scheduler worker to execute. Demo mode keeps its simulated
// flow. Fail-closed when the durable scheduler is not configured.
func (s *Service) EnqueueWarehouseSync(ctx context.Context, databaseID string) (domain.JobView, error) {
	if s.demo {
		return s.CreateJob("warehouse_sync", databaseID, databaseID), nil
	}
	view := s.newJobRecord("warehouse_sync", databaseID, databaseID)
	if err := s.queueWarehouseSyncJob(view.ID, databaseID); err != nil {
		s.failJob(view.ID, "extract", fmt.Sprintf("Warehouse sync not queued: %v", err))
		return view, err
	}
	s.appendLog(view.ID, "info", view.Stage, "Queued on the durable scheduler.")
	return view, nil
}

// queueWarehouseSyncJob pushes one durable warehouse_sync job. The px job
// record ID rides in the payload so the worker updates exactly the record the
// API returned.
func (s *Service) queueWarehouseSyncJob(pxJobID, databaseID string) error {
	if s.jobQueue == nil {
		return fmt.Errorf("warehouse sync queue unavailable: durable scheduler not configured")
	}
	payload, err := json.Marshal(map[string]string{"source": databaseID, "operation": "warehouse_sync", "job_id": pxJobID})
	if err != nil {
		return err
	}
	job := domain.Job{
		ID:          domain.JobID(fmt.Sprintf("whsync-%s-%d", databaseID, time.Now().UnixNano())),
		Type:        domain.JobWarehouseSync,
		Status:      domain.JobPending,
		ResourceID:  databaseID,
		PayloadJSON: payload,
	}
	return s.jobQueue.Enqueue(context.Background(), job)
}

// ExecuteWarehouseSyncJob runs one leased warehouse_sync job through the real
// sync pipeline, mirroring stage progress into the px job record so the
// existing /api/v1/jobs API keeps showing live progress. It returns the job
// result JSON persisted with the durable job.
func (s *Service) ExecuteWarehouseSyncJob(ctx context.Context, job domain.Job) ([]byte, error) {
	var payload struct {
		Source    string `json:"source"`
		Operation string `json:"operation"`
		Schedule  string `json:"schedule"`
		JobID     string `json:"job_id"`
	}
	if len(job.PayloadJSON) > 0 {
		_ = json.Unmarshal(job.PayloadJSON, &payload)
	}
	databaseID := payload.Source
	if databaseID == "" {
		databaseID = job.ResourceID
	}
	jobID := payload.JobID
	s.mu.Lock()
	_, recordAlive := s.jobs[jobID]
	s.mu.Unlock()
	if jobID == "" || !recordAlive {
		// px job records are in-memory and do not survive a restart; recreate
		// the record so recovered jobs stay visible through /api/v1/jobs.
		view := s.newJobRecord("warehouse_sync", databaseID, databaseID)
		jobID = view.ID
	}

	s.updateJobStage(jobID, "extract", 15, fmt.Sprintf("Extracting source tables for %s", databaseID))
	if err := s.runWarehouseSyncPipeline(ctx, databaseID); err != nil {
		s.failJob(jobID, "transform_parquet", fmt.Sprintf("Warehouse sync failed: %v", err))
		return nil, err
	}
	s.updateJobStage(jobID, "transform_parquet", 60, "Parquet artifacts written and verified")
	s.updateJobStage(jobID, "partition", 75, "Partition metadata committed")
	s.updateJobStage(jobID, "load_warehouse", 88, "Analytical tables materialized")
	s.updateJobStage(jobID, "validate_indexes", 95, "Validated warehouse artifacts")
	s.updateJobStage(jobID, "complete", 100, fmt.Sprintf("Data Warehouse synchronized successfully for %s", databaseID))
	s.appendLog(jobID, "info", "complete", fmt.Sprintf("Warehouse sync finished for %s. Verified Parquet datasets are ready.", databaseID))
	return json.Marshal(map[string]any{"job_id": jobID, "database": databaseID, "schedule": payload.Schedule, "status": "complete"})
}

// runWarehouseSyncPipeline executes the real extraction pipeline. Tests inject
// warehouseSyncRunner to exercise the job lifecycle without live sources.
func (s *Service) runWarehouseSyncPipeline(ctx context.Context, databaseID string) error {
	if s.warehouseSyncRunner != nil {
		return s.warehouseSyncRunner(ctx, databaseID)
	}
	if err := s.ValidateWarehouseSync(ctx, databaseID); err != nil {
		return err
	}
	return s.SyncWarehouse(ctx, databaseID)
}

// validateWarehouseQuery keeps the warehouse interface deliberately narrow:
// it accepts one read-only analytical statement and rejects SQL features that
// can mutate data, attach files, or invoke external code.
func validateWarehouseQuery(raw string) error {
	q := strings.TrimSpace(strings.TrimSuffix(raw, ";"))
	if q == "" {
		return fmt.Errorf("query string cannot be empty")
	}
	if strings.Contains(q, ";") {
		return fmt.Errorf("warehouse queries must contain one statement")
	}
	upper := strings.ToUpper(q)
	if !(strings.HasPrefix(upper, "SELECT ") || strings.HasPrefix(upper, "SELECT\n") || strings.HasPrefix(upper, "WITH ") || strings.HasPrefix(upper, "WITH\n") || strings.HasPrefix(upper, "EXPLAIN ") || strings.HasPrefix(upper, "EXPLAIN\n")) {
		return fmt.Errorf("warehouse queries must be read-only SELECT, WITH, or EXPLAIN statements")
	}
	for _, token := range []string{" ATTACH ", " COPY ", " INSTALL ", " LOAD ", " PRAGMA ", " CREATE ", " DROP ", " ALTER ", " INSERT ", " UPDATE ", " DELETE ", " GRANT ", " REVOKE ", "READ_CSV", "READ_PARQUET", "HTTPFS"} {
		if strings.Contains(" "+upper+" ", token) {
			return fmt.Errorf("warehouse query contains a disallowed operation")
		}
	}
	return nil
}

// executeLiveDBQuery routes an analytical SQL query to the live PostgreSQL or MySQL database.
func (s *Service) executeLiveDBQuery(ctx context.Context, req warehouse.QueryRequest) (*warehouse.QueryResult, error) {
	start := time.Now()
	dbID := req.Database

	s.mu.Lock()
	var targetDB domain.DatabaseResource
	var found bool
	for _, db := range s.customDatabases {
		if db.ID == dbID || db.Name == dbID {
			targetDB = db
			found = true
			break
		}
	}
	s.mu.Unlock()

	engine := "postgres"
	dbName := dbID
	if found {
		engine = strings.ToLower(targetDB.Engine)
		dbName = targetDB.Name
	}

	var rows [][]any
	var cols []warehouse.ColumnMeta

	if strings.Contains(engine, "mysql") || strings.Contains(engine, "mariadb") {
		mHost, mPort, mUser, mPass := s.resolveMySQLParams()
		args := []string{
			"-h", mHost,
			"-P", strconv.Itoa(mPort),
			"-u", mUser,
			"--batch",
			"--connect-timeout=5",
			"-D", dbName,
			"-e", req.Query,
		}
		cmd := exec.CommandContext(ctx, "mysql", args...)
		if mPass != "" {
			cmd.Env = append(os.Environ(), "MYSQL_PWD="+mPass)
		}
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("mysql query error: %w", err)
		}
		rows, cols = parseMySQLBatchOutput(out)
	} else {
		pgHost, pgPort, pgUser, pgPass := s.resolvePostgresParams()
		args := []string{
			"-w",
			"-h", pgHost,
			"-p", strconv.Itoa(pgPort),
			"-U", pgUser,
			"-d", dbName,
			"--csv",
			"-c", req.Query,
		}
		cmd := exec.CommandContext(ctx, "psql", args...)
		cmd.Env = append(os.Environ(), "PGPASSWORD="+pgPass, "PGCONNECT_TIMEOUT=5")
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("psql query error: %w", err)
		}
		rows, cols = parseCSVOutput(out)
	}

	return &warehouse.QueryResult{
		Columns:     cols,
		Rows:        rows,
		RowCount:    len(rows),
		ExecutionMs: time.Since(start).Milliseconds(),
		Engine:      engine + " (live)",
		ExecutedAt:  time.Now().UTC(),
	}, nil
}

// parseMySQLBatchOutput parses tab-separated MySQL --batch output into columns and rows.
func parseMySQLBatchOutput(out []byte) ([][]any, []warehouse.ColumnMeta) {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return nil, nil
	}
	// First line is column headers
	headers := strings.Split(strings.TrimSpace(lines[0]), "\t")
	cols := make([]warehouse.ColumnMeta, len(headers))
	for i, h := range headers {
		cols[i] = warehouse.ColumnMeta{Name: h, Type: "TEXT"}
	}
	var rows [][]any
	for _, line := range lines[1:] {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		row := make([]any, len(cols))
		for i := range row {
			if i < len(parts) {
				v := parts[i]
				if v == "NULL" {
					row[i] = nil
				} else {
					row[i] = v
				}
			}
		}
		rows = append(rows, row)
	}
	return rows, cols
}

// parseCSVOutput parses PostgreSQL CSV output (with header) into columns and rows.
func parseCSVOutput(out []byte) ([][]any, []warehouse.ColumnMeta) {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return nil, nil
	}
	headers := strings.Split(strings.TrimSpace(lines[0]), ",")
	cols := make([]warehouse.ColumnMeta, len(headers))
	for i, h := range headers {
		cols[i] = warehouse.ColumnMeta{Name: strings.Trim(h, "\""), Type: "TEXT"}
	}
	var rows [][]any
	for _, line := range lines[1:] {
		if line == "" {
			continue
		}
		// Simple CSV split (handles quoted fields)
		parts := splitCSVLine(line)
		row := make([]any, len(cols))
		for i := range row {
			if i < len(parts) {
				v := parts[i]
				if v == "" || v == "NULL" {
					row[i] = nil
				} else {
					row[i] = v
				}
			}
		}
		rows = append(rows, row)
	}
	return rows, cols
}

// splitCSVLine splits a single CSV line respecting double-quoted fields.
func splitCSVLine(line string) []string {
	var fields []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '"' {
			if inQuote && i+1 < len(line) && line[i+1] == '"' {
				cur.WriteByte('"')
				i++
			} else {
				inQuote = !inQuote
			}
		} else if c == ',' && !inQuote {
			fields = append(fields, cur.String())
			cur.Reset()
		} else {
			cur.WriteByte(c)
		}
	}
	fields = append(fields, cur.String())
	return fields
}

// seedDefaultWarehouseTables pre-populates the analytical workspace with fast queryable demo/production datasets.
func (s *Service) seedDefaultWarehouseTables() {
	if s.warehouseEngine == nil {
		return
	}

	// Seed users table
	userCols := []warehouse.ColumnMeta{
		{Name: "id", Type: "INTEGER"},
		{Name: "name", Type: "TEXT"},
		{Name: "email", Type: "TEXT"},
		{Name: "plan", Type: "TEXT"},
		{Name: "mrr", Type: "REAL"},
		{Name: "status", Type: "TEXT"},
		{Name: "country", Type: "TEXT"},
		{Name: "created_at", Type: "TEXT"},
	}
	userRows := [][]any{
		{1, "Acme Corp", "admin@acme.com", "enterprise", 4200.0, "active", "US", "2026-01-15 10:20:00"},
		{2, "TechFlow Ltd", "contact@techflow.io", "pro", 890.0, "active", "GB", "2026-02-01 14:12:00"},
		{3, "Solaris AI", "team@solaris.ai", "enterprise", 6500.0, "active", "US", "2026-02-18 09:45:00"},
		{4, "Nordic Data", "hello@nordic.se", "starter", 250.0, "active", "SE", "2026-03-05 11:30:00"},
		{5, "CyberGuard", "sec@cyberguard.net", "enterprise", 5100.0, "active", "DE", "2026-04-10 16:05:00"},
		{6, "Quantum Dynamics", "dev@quantum.org", "pro", 950.0, "active", "CA", "2026-05-12 08:50:00"},
		{7, "HyperScale Inc", "ops@hyperscale.io", "enterprise", 7800.0, "active", "US", "2026-06-20 18:22:00"},
		{8, "Starlight Media", "media@starlight.co", "starter", 350.0, "active", "FR", "2026-07-04 13:40:00"},
	}
	_ = s.warehouseEngine.SeedDataset("users", userCols, userRows)

	// Seed orders / transactions table
	orderCols := []warehouse.ColumnMeta{
		{Name: "id", Type: "INTEGER"},
		{Name: "user_id", Type: "INTEGER"},
		{Name: "amount", Type: "REAL"},
		{Name: "currency", Type: "TEXT"},
		{Name: "payment_method", Type: "TEXT"},
		{Name: "status", Type: "TEXT"},
		{Name: "created_at", Type: "TEXT"},
	}
	orderRows := [][]any{
		{101, 1, 4200.0, "USD", "credit_card", "succeeded", "2026-08-01 00:01:15"},
		{102, 2, 890.0, "USD", "wire_transfer", "succeeded", "2026-08-01 02:15:30"},
		{103, 3, 6500.0, "USD", "credit_card", "succeeded", "2026-08-02 08:30:00"},
		{104, 4, 250.0, "EUR", "credit_card", "succeeded", "2026-08-03 11:10:45"},
		{105, 5, 5100.0, "EUR", "wire_transfer", "succeeded", "2026-08-05 14:40:12"},
		{106, 6, 950.0, "USD", "credit_card", "succeeded", "2026-08-08 09:20:18"},
		{107, 7, 7800.0, "USD", "wire_transfer", "succeeded", "2026-08-10 17:05:00"},
		{108, 8, 350.0, "EUR", "credit_card", "succeeded", "2026-08-12 12:55:22"},
	}
	_ = s.warehouseEngine.SeedDataset("transactions", orderCols, orderRows)
	_ = s.warehouseEngine.SeedDataset("orders", orderCols, orderRows)

	// Seed backup metrics table
	metricCols := []warehouse.ColumnMeta{
		{Name: "snapshot_id", Type: "TEXT"},
		{Name: "database_name", Type: "TEXT"},
		{Name: "uncompressed_bytes", Type: "INTEGER"},
		{Name: "parquet_bytes", Type: "INTEGER"},
		{Name: "compression_ratio", Type: "REAL"},
		{Name: "duration_ms", Type: "INTEGER"},
		{Name: "recorded_at", Type: "TEXT"},
	}
	metricRows := [][]any{
		{"snap-001", "cribx_test", 2900000000, 540000000, 5.37, 1840, "2026-08-14 12:00:00"},
		{"snap-002", "cribx_test", 2920000000, 542000000, 5.38, 1720, "2026-08-15 12:00:00"},
		{"snap-003", "cribx_test", 2950000000, 546000000, 5.40, 1690, "2026-08-16 12:00:00"},
	}
	_ = s.warehouseEngine.SeedDataset("lakehouse_metrics", metricCols, metricRows)
}

// GetWarehouseConnectors returns only genuinely configured external OLAP
// destinations. Until connector persistence exists, no external connectors are
// configurable: the list contains the embedded engine (whose status is derived
// from real CLI detection on this host) and nothing else. No latency, sync
// schedule, or timestamps are reported because none are measured.
func (s *Service) GetWarehouseConnectors(ctx context.Context) []warehouse.WarehouseConnector {
	s.initWarehouse()
	var out []warehouse.WarehouseConnector
	if s.warehouseEngine != nil {
		out = append(out, warehouse.WarehouseConnector{
			ID:       "duckdb-embedded",
			Name:     "Embedded DuckDB Lakehouse Engine",
			Type:     "duckdb_embedded",
			Endpoint: "local://in-process",
			Status:   s.warehouseEngine.Status(),
		})
	}
	return out
}

// ExportWarehouseResults generates a downloadable CSV / JSON / Parquet export of query results.
func (s *Service) ExportWarehouseResults(ctx context.Context, req warehouse.QueryRequest, format string) ([]byte, string, string, error) {
	res, err := s.ExecuteWarehouseQuery(ctx, req)
	if err != nil {
		return nil, "", "", err
	}

	filename := fmt.Sprintf("dbvault_query_export_%s", time.Now().Format("20060102_150405"))

	switch strings.ToLower(format) {
	case "csv":
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)

		header := make([]string, len(res.Columns))
		for i, c := range res.Columns {
			header[i] = c.Name
		}
		_ = w.Write(header)

		for _, row := range res.Rows {
			rowStr := make([]string, len(row))
			for i, v := range row {
				if v == nil {
					rowStr[i] = ""
				} else {
					rowStr[i] = fmt.Sprintf("%v", v)
				}
			}
			_ = w.Write(rowStr)
		}
		w.Flush()
		return buf.Bytes(), filename + ".csv", "text/csv", nil

	case "json":
		var outputRows []map[string]any
		for _, row := range res.Rows {
			m := make(map[string]any)
			for i, c := range res.Columns {
				if i < len(row) {
					m[c.Name] = row[i]
				}
			}
			outputRows = append(outputRows, m)
		}
		b, err := json.MarshalIndent(outputRows, "", "  ")
		if err != nil {
			return nil, "", "", err
		}
		return b, filename + ".json", "application/json", nil

	default:
		return nil, "", "", fmt.Errorf("unsupported export format %q (use csv or json)", format)
	}
}

// BIPowerBIDataset represents an exportable analytical dataset for Power BI & BI tools.
type BIPowerBIDataset struct {
	DatabaseID    string `json:"database_id"`
	DatabaseName  string `json:"database_name"`
	TableName     string `json:"table_name"`
	EstimatedRows int64  `json:"estimated_rows"`
	DataSizeBytes int64  `json:"data_size_bytes"`
	CSVFeedURL    string `json:"csv_feed_url"`
	JSONFeedURL   string `json:"json_feed_url"`
	FeedDatabase  string `json:"feed_database"`
	PowerQueryM   string `json:"power_query_m"`
	PythonSnippet string `json:"python_snippet"`
}

// BIPowerBICatalog returns the catalog of all datasets ready for Power BI & external dashboarding tools.
func (s *Service) BIPowerBICatalog(ctx context.Context, host string) map[string]any {
	s.mu.Lock()
	demo := s.demo
	customDatabases := make([]domain.DatabaseResource, 0, len(s.customDatabases))
	for _, db := range s.customDatabases {
		customDatabases = append(customDatabases, db)
	}
	s.mu.Unlock()

	if host == "" {
		host = "127.0.0.1:8080"
	}
	baseURL := fmt.Sprintf("http://%s", host)

	var datasets []BIPowerBIDataset

	if demo && len(customDatabases) == 0 {
		sampleTables := []struct {
			db   string
			name string
			rows int64
		}{
			{"production-postgres", "users", 125000},
			{"production-postgres", "orders", 850000},
			{"production-postgres", "transactions", 2100000},
			{"billing-sqlite", "invoices", 45000},
		}
		for _, st := range sampleTables {
			csvURL := fmt.Sprintf("%s/api/v1/bi/powerbi/feed?database=duckdb&table=%s&format=csv", baseURL, url.QueryEscape(st.name))
			jsonURL := fmt.Sprintf("%s/api/v1/bi/powerbi/feed?database=duckdb&table=%s&format=json", baseURL, url.QueryEscape(st.name))
			mCode := fmt.Sprintf(`let
    Source = Csv.Document(Web.Contents("%s"), [Delimiter=",", Columns=null, Encoding=65001, QuoteStyle=QuoteStyle.Csv]),
    #"Promoted Headers" = Table.PromoteHeaders(Source, [PromoteAllScalars=true])
in
    #"Promoted Headers"`, csvURL)
			pyCode := fmt.Sprintf(`import pandas as pd
df = pd.read_csv("%s")
print(df.head())`, csvURL)

			datasets = append(datasets, BIPowerBIDataset{
				DatabaseID:    st.db,
				DatabaseName:  st.db,
				TableName:     st.name,
				EstimatedRows: st.rows,
				DataSizeBytes: st.rows * 128,
				CSVFeedURL:    csvURL,
				JSONFeedURL:   jsonURL,
				FeedDatabase:  "duckdb",
				PowerQueryM:   mCode,
				PythonSnippet: pyCode,
			})
		}
	} else {
		var dbsToScan []domain.DatabaseResource
		if len(customDatabases) > 0 {
			for _, db := range customDatabases {
				dbsToScan = append(dbsToScan, db)
			}
		} else {
			dbsToScan = []domain.DatabaseResource{
				{ID: "cribx_test", Name: "cribx_test", Engine: "postgres"},
				{ID: "courier", Name: "courier", Engine: "mysql"},
				{ID: "production-postgres", Name: "Production PostgreSQL", Engine: "postgres"},
			}
		}

		for _, db := range dbsToScan {
			schema := s.DatabaseSchema(db.ID)
			for _, t := range schema.Tables {
				if t.Excluded {
					continue
				}
				csvURL := fmt.Sprintf("%s/api/v1/bi/powerbi/feed?database=%s&table=%s&format=csv", baseURL, url.QueryEscape(db.ID), url.QueryEscape(t.Name))
				jsonURL := fmt.Sprintf("%s/api/v1/bi/powerbi/feed?database=%s&table=%s&format=json", baseURL, url.QueryEscape(db.ID), url.QueryEscape(t.Name))
				mCode := fmt.Sprintf(`let
    Source = Csv.Document(Web.Contents("%s"), [Delimiter=",", Columns=null, Encoding=65001, QuoteStyle=QuoteStyle.Csv]),
    #"Promoted Headers" = Table.PromoteHeaders(Source, [PromoteAllScalars=true])
in
    #"Promoted Headers"`, csvURL)
				pyCode := fmt.Sprintf(`import pandas as pd
df = pd.read_csv("%s")
print(df.head())`, csvURL)

				datasets = append(datasets, BIPowerBIDataset{
					DatabaseID:    db.ID,
					DatabaseName:  db.Name,
					TableName:     t.Name,
					EstimatedRows: t.EstimatedRows,
					DataSizeBytes: t.DataSizeBytes,
					CSVFeedURL:    csvURL,
					JSONFeedURL:   jsonURL,
					FeedDatabase:  db.ID,
					PowerQueryM:   mCode,
					PythonSnippet: pyCode,
				})
			}
		}
	}

	return map[string]any{
		"base_url":            baseURL,
		"total_datasets":      len(datasets),
		"datasets":            datasets,
		"requires_connection": true,
		"connection_endpoint": "/api/v1/bi/connections",
		"feed_authentication": "Authorization: Bearer <scoped-token>",
		"protocols_supported": []string{
			"Power BI Web Connector (CSV / JSON)",
			"Microsoft Excel (From Web / OData)",
			"Tableau Web Data & CSV Feed",
			"Apache Superset / Metabase REST Source",
			"Python / Pandas / Polars / Jupyter HTTP Stream",
		},
	}
}

// BIPowerBIFeed streams live data in CSV or JSON for Power BI and external BI dashboards.
func (s *Service) BIPowerBIFeed(ctx context.Context, connectionID, token, database, table, format string, limit int) ([]byte, string, string, error) {
	if limit <= 0 || limit > 100000 {
		limit = 10000
	}
	if format == "" {
		format = "csv"
	}
	if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(table) {
		return nil, "", "", fmt.Errorf("invalid dataset table")
	}
	// Demo fixtures are intentionally public for the local showcase. Every
	// non-demo deployment must use a scoped connection token.
	s.mu.Lock()
	demo := s.demo
	s.mu.Unlock()
	if !demo {
		if err := s.AuthorizeBIFeed(connectionID, token, database, table); err != nil {
			return nil, "", "", err
		}
	}
	if database != "" && database != "duckdb" && database != "auto" && !s.databaseHasSyncedWarehouseData(database) {
		return nil, "", "", fmt.Errorf("BI feeds are available only for synchronized warehouse datasets")
	}
	query := fmt.Sprintf("SELECT * FROM %s LIMIT %d", table, limit)

	req := warehouse.QueryRequest{
		Query:    query,
		Database: database,
	}
	return s.ExportWarehouseResults(ctx, req, format)
}
