package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPowerBICatalogAndFeedRoutes(t *testing.T) {
	a, err := New(Config{DataDirectory: t.TempDir(), Version: "test", Demo: true}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()

	// 1. Test /api/v1/bi/powerbi/catalog
	resp, err := http.Get(server.URL + "/api/v1/bi/powerbi/catalog")
	if err != nil {
		t.Fatalf("catalog GET failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("catalog status = %d, want 200", resp.StatusCode)
	}
	var catalog map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		t.Fatalf("decode catalog JSON failed: %v", err)
	}
	resp.Body.Close()

	if _, ok := catalog["protocols_supported"]; !ok {
		t.Errorf("expected protocols_supported in catalog response")
	}

	// 2. Test /api/v1/bi/powerbi/feed
	feedResp, err := http.Get(server.URL + "/api/v1/bi/powerbi/feed?database=duckdb&table=orders&format=csv&limit=5")
	if err != nil {
		t.Fatalf("feed GET failed: %v", err)
	}
	defer feedResp.Body.Close()

	if feedResp.StatusCode != http.StatusOK {
		t.Fatalf("feed status = %d, want 200", feedResp.StatusCode)
	}
	contentType := feedResp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/csv") {
		t.Errorf("feed Content-Type = %s, want text/csv", contentType)
	}
}

func TestBIConnectionRoutesRequireScopedBearerToken(t *testing.T) {
	a, err := New(Config{DataDirectory: t.TempDir(), Version: "test", Demo: false}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()

	payload := `{"name":"Power BI","provider":"powerbi","datasets":[{"database":"production-postgres","table":"orders"}]}`
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/bi/connections", strings.NewReader(payload))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", createResp.StatusCode)
	}
	var created struct {
		Connection struct {
			ID string `json:"id"`
		} `json:"connection"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Connection.ID == "" || created.Token == "" {
		t.Fatalf("missing connection credentials: %#v", created)
	}

	feedResp, err := http.Get(server.URL + "/api/v1/bi/powerbi/feed?connection_id=" + created.Connection.ID + "&database=production-postgres&table=orders")
	if err != nil {
		t.Fatal(err)
	}
	feedResp.Body.Close()
	if feedResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unauthenticated feed status = %d, want 400", feedResp.StatusCode)
	}
}
