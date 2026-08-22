package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPowerBICatalogAndFeedRoutes(t *testing.T) {
	f := newAuthedFixture(t, Config{Version: "test", Demo: true})
	server := httptest.NewServer(f.H)
	defer server.Close()

	doGet := func(path string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(f.Cookie)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// 1. Test /api/v1/bi/powerbi/catalog
	resp := doGet("/api/v1/bi/powerbi/catalog")
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
	feedResp := doGet("/api/v1/bi/powerbi/feed?database=duckdb&table=orders&format=csv&limit=5")
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
	f := newAuthedFixture(t, Config{Version: "test", Demo: false})
	server := httptest.NewServer(f.H)
	defer server.Close()

	payload := `{"name":"Power BI","provider":"powerbi","datasets":[{"database":"production-postgres","table":"orders"}]}`
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/bi/connections", strings.NewReader(payload))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(f.Cookie)
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

	// Admin cookie satisfies outer auth; missing scoped connection token must
	// still fail inside the handler with a 400.
	feedReq, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/bi/powerbi/feed?connection_id="+created.Connection.ID+"&database=production-postgres&table=orders", nil)
	if err != nil {
		t.Fatal(err)
	}
	feedReq.AddCookie(f.Cookie)
	feedResp, err := http.DefaultClient.Do(feedReq)
	if err != nil {
		t.Fatal(err)
	}
	feedResp.Body.Close()
	if feedResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unauthenticated feed status = %d, want 400", feedResp.StatusCode)
	}
}
