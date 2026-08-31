---
session: ses_fa7c
updated: 2026-08-31T14:42:49.941Z
---

Let me see the rest of `syncWarehouseTable` to make precise edits:
[Assistant tool call]: read({"filePath":"/home/sifu/Documents/dbvault/internal/application/productexperience/service_warehouse.go","limit":110,"offset":372})
[Tool result]: <path>/home/sifu/Documents/dbvault/internal/application/productexperience/service_warehouse.go</path>
<type>content>
372: 	if mode == "incremental" && len(result.Rows) == 0 {
373: 		// Nothing new since the recorded watermark: the honest outcome is a
374: 		// no-op that keeps every previous measurement and only records that
375: 		// the dataset is current as of now.
376: 		current := *prev
377: 		current.LastSyncAt = now
378: 		current.LastSyncStatus = "succeeded"
379: 		current.LastSyncMode = "incremental"
380: 		if s.warehouseEvidence != nil {
381: 			return s.warehouseEvidence.UpsertWarehouseDataset(ctx, current)
382: 		}
383: 		return nil
384: 	}
385: 	wmType := classifyWatermarkType(result.Columns, watermarkColumn)
385→385: 	wmType := classifyWatermarkType(result.Columns, watermarkColumn)
386: 	dir := filepath.Join(s.root, "lakehouse", "db="+databaseID, "tbl="+tableName)
387: 	if err := os.MkdirAll(dir, 0700); err != nil {
388: 		return err
389: 	}
390: 	path := filepath.Join(dir, "data.parquet")
391: 	if mode == "incremental" {
392: 		path = filepath.Join(dir, fmt.Sprintf("part-%d.parquet", now.UnixNano()))
393: 	}
394: 	rawBytes, err := s.writeWarehouseParquet(ctx, duckdbPath, path, result)
395: 	if err != nil {
396: 		return err
397: 	}
398: 	verifiedColumns, fileRows, verifyErr := inspectParquet(duckdbPath, path)
398→398: 	verifiedColumns, fileRows, verifyErr := inspectParquet(duckdbPath, path)
399: 	schemaVerified := verifyErr == nil && fileRows == int64(len(result.Rows)) && parquetColumnsMatch(verifiedColumns, result.Columns)
400: 	if len(verifiedColumns) == 0 {
401: 		verifiedColumns = columnsFromResult(result.Columns)
402: 	}
403: 	if !parquetColumnsMatch(verifiedColumns, result.Columns) {
404: 		// On-disk schema diverges from the extraction metadata: fail closed
404→403: 		// On-disk schema diverges from the extraction metadata: fail closed
405: 		// rather than record a schema the artifact does not actually carry.
406: 		return fmt.Errorf("parquet schema for %s.%s diverges from extraction metadata", databaseID, tableName)
407: 	}
408: 	...
460: }
```

Hmm wait — line 399/403 logic: `schemaVerified := verifyErr == nil && fileRows == int64(len(result.Rows)) && parquetColumnsMatch(...)`, then line 403: `if !parquetColumnsMatch(verifiedColumns, result.Columns) { return err }` — but if verifiedColumns were empty they got replaced at 400-402 with the extraction columns, so 403 compares extraction-derived vs extraction — always true. Wait no: if verifyErr != nil, verifiedColumns is empty → replaced by extraction columns → 403 passes. If inspectParquet succeeded, verifiedColumns are real on-disk columns → 403 fails closed on divergence. OK that's coherent.

I notice the file now has line numbers 385→385 markers which suggests my read had some rendering glitch, but content is clear. Now let me apply the edits:

**Edit 1**: Replace lines 342-361 with the extracted helpers + calls.
**Edit 2**: Replace lines 390-393 with dt= logic (wmType already computed at 385 — good, it's already hoisted! The concurrent agent moved it up. So I only need the path change).
**Edit 3**: Convert `inspectParquet` func to var (line 489).

Let me apply Edit 1 + 2 via a careful replacement. I'll use the edit tool on the file. Since the concurrent agent may also edit, I'll do all service_warehouse.go edits quickly in sequence.

Edit for lines 342-354:
```go
	mode := "full"
	watermarkColumn := strings.TrimSpace(opts.WatermarkColumn)
	if watermarkColumn == "" && prev != nil {
		watermarkColumn = prev.WatermarkColumn
	}
	var lastWatermark string
	if opts.Incremental && prev != nil && prev.LastSyncStatus != "" && watermarkColumn != "" && prev.LastWatermarkValue != "" {
		if !biIdentifier.MatchString(watermarkColumn) {
			return fmt.Errorf("watermark column %q is not a safe identifier", watermarkColumn)
		}
		mode = "incremental"
		lastWatermark = prev.LastWatermarkValue
	}
```
→
```go
	mode, watermarkColumn, lastWatermark, err := resolveWarehouseSyncMode(opts, prev)
	if err != nil {
		return err
	}
```

Wait — `err` gets declared here, then line 362 `result, err := s.executeLiveDBQuery(...)` — that's fine (result is new). But then `rawBytes, err := ...` at 394 — rawBytes new. OK. But wait, careful: after my edit there's `mode, watermarkColumn, lastWatermark, err :=` — all four new vars. Then 362 `result, err := ...` — err exists, result new → OK.

Edit for lines 356-361:
```go
	var query string
	if mode == "incremental" {
		query = fmt.Sprintf("SELECT * FROM %s WHERE %s > '%s' ORDER BY %s", tableName, watermarkColumn, strings.ReplaceAll(lastWatermark, "'", "''"), watermarkColumn)
	} else {
		query = "SELECT * FROM " + tableName
	}
```
→
```go
	var query string
	if mode == "incremental" {
		query = buildWarehouseExtractQuery(tableName, watermarkColumn, lastWatermark)
	} else {
		query = buildWarehouseExtractQuery(tableName, "", "")
	}
```

Edit for 390-393:
```go
	path := filepath.Join(dir, "data.parquet")
	if mode == "incremental" {
		path = filepath.Join(dir, fmt.Sprintf("part-%d.parquet", now.UnixNano()))
	}
```
→
```go
	path := filepath.Join(dir, "data.parquet")
	if mode == "incremental" {
		name := fmt.Sprintf("part-%d.parquet", now.UnixNano())
		if wmType == "date" {
			// Date-typed watermarks partition appends by sync day so each
			// incremental run lands in its own dt=<YYYY-MM-DD> directory.
			name = filepath.Join("dt="+now.UTC().Format("2006-01-02"), name)
		}
		path = filepath.Join(dir, name)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
```

Then add the helper functions near the other watermark helpers (after detectWatermarkType/columnIndex area, e.g., after findColumn or near classifyWatermarkType). And convert inspectParquet to var.

Let me do these edits now with the edit tool.
</content>
