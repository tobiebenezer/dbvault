package warehouse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ClickHouseConnector communicates with ClickHouse HTTP interfaces.
type ClickHouseConnector struct {
	endpoint string
	username string
	password string
	database string
	client   *http.Client
}

// NewClickHouseConnector initializes a new ClickHouse client.
func NewClickHouseConnector(endpoint, username, password, database string) *ClickHouseConnector {
	if endpoint == "" {
		endpoint = "http://127.0.0.1:8123"
	}
	if database == "" {
		database = "default"
	}
	return &ClickHouseConnector{
		endpoint: strings.TrimSuffix(endpoint, "/"),
		username: username,
		password: password,
		database: database,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Ping checks if ClickHouse is reachable and measures latency.
func (c *ClickHouseConnector) Ping(ctx context.Context) (int64, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/ping", nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("clickhouse connection refused at %s: %w", c.endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("clickhouse ping returned status %d", resp.StatusCode)
	}

	return time.Since(start).Milliseconds(), nil
}

// ExecuteQuery runs an analytical query on ClickHouse and parses the JSONEachRow output.
func (c *ClickHouseConnector) ExecuteQuery(ctx context.Context, query string) (*QueryResult, error) {
	start := time.Now()

	// Append FORMAT JSON to get structured metadata
	cleanQuery := strings.TrimSuffix(strings.TrimSpace(query), ";")
	if !strings.Contains(strings.ToUpper(cleanQuery), "FORMAT ") {
		cleanQuery += " FORMAT JSON"
	}

	reqURL := fmt.Sprintf("%s/?database=%s", c.endpoint, c.database)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewBufferString(cleanQuery))
	if err != nil {
		return nil, err
	}

	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clickhouse query execution failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("clickhouse query error (status %d): %s", resp.StatusCode, string(body))
	}

	var chResponse struct {
		Meta []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"meta"`
		Data       []map[string]any `json:"data"`
		Rows       int              `json:"rows"`
		Statistics struct {
			Elapsed   float64 `json:"elapsed"`
			RowsRead  int64   `json:"rows_read"`
			BytesRead int64   `json:"bytes_read"`
		} `json:"statistics"`
	}

	if err := json.Unmarshal(body, &chResponse); err != nil {
		return nil, fmt.Errorf("failed to parse ClickHouse response: %w", err)
	}

	cols := make([]ColumnMeta, len(chResponse.Meta))
	for i, m := range chResponse.Meta {
		cols[i] = ColumnMeta{
			Name: m.Name,
			Type: m.Type,
		}
	}

	rows := make([][]any, len(chResponse.Data))
	for rIdx, d := range chResponse.Data {
		row := make([]any, len(cols))
		for cIdx, col := range cols {
			row[cIdx] = d[col.Name]
		}
		rows[rIdx] = row
	}

	return &QueryResult{
		Columns:      cols,
		Rows:         rows,
		RowCount:     len(rows),
		ExecutionMs:  time.Since(start).Milliseconds(),
		BytesScanned: chResponse.Statistics.BytesRead,
		Engine:       "ClickHouse (Distributed OLAP)",
		ExecutedAt:   time.Now().UTC(),
	}, nil
}
