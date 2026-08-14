package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedUIServedAndSPAFallback(t *testing.T) {
	a, err := New(Config{DataDirectory: t.TempDir(), Version: "test"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := a.Handler()
	for _, path := range []string{"/", "/recovery/timeline"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s got %d", path, res.Code)
		}
		if !strings.Contains(res.Body.String(), "DBVault Console") {
			t.Fatalf("missing UI shell for %s", path)
		}
	}
}

func TestSetupTokenIsOneTime(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Config{DataDirectory: dir, SetupTokenPath: filepath.Join(dir, "setup-token")}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	token, err := a.EnsureSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"token": token, "email": "admin@example.test"})
	res := httptest.NewRecorder()
	a.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/setup/complete", bytes.NewReader(body)))
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
	res = httptest.NewRecorder()
	a.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/setup/complete", bytes.NewReader(body)))
	if res.Code == http.StatusOK {
		t.Fatal("setup token was reusable")
	}
}

func TestAgentInstallScriptRoute(t *testing.T) {
	a, err := New(Config{DataDirectory: t.TempDir(), PublicURL: "https://backup.example.test"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	a.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/install/agent.sh", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "https://backup.example.test") {
		t.Fatal("script does not contain controller URL")
	}
}

func TestProductExperienceRoutes(t *testing.T) {
	dir := t.TempDir()
	app, err := New(Config{Listen: "127.0.0.1:0", DataDirectory: dir}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := app.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "protected_databases") {
		t.Fatalf("overview status=%d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/sources/prod/recovery-timeline", nil)
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "continuous") {
		t.Fatalf("timeline status=%d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/policies/simulate", strings.NewReader(`{"policy":{"backup_frequency":"Every 12 hours","replica_count":2}}`))
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "estimated_thirty_day_bytes") {
		t.Fatalf("simulate status=%d body=%s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/sandboxes", strings.NewReader(`{"source_id":"prod","engine":"postgres"}`))
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted || !strings.Contains(res.Body.String(), "sandbox-") {
		t.Fatalf("sandbox status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestJobRoutesAndSSESnapshot(t *testing.T) {
	app, err := New(Config{DataDirectory: t.TempDir()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := app.Handler()

	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "job-backup-demo") {
		t.Fatalf("jobs status=%d body=%s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/events/jobs?once=1", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "event: job.") {
		t.Fatalf("sse status=%d body=%s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-backup-demo/cancel", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "cancelled") {
		t.Fatalf("cancel status=%d body=%s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/jobs/job-replica-demo/retry", nil))
	if res.Code != http.StatusAccepted || !strings.Contains(res.Body.String(), "new_job_id") {
		t.Fatalf("retry status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestConnectedUIActionRoutes(t *testing.T) {
	app, err := New(Config{DataDirectory: t.TempDir()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := app.Handler()

	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v1/inventory", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "contabo-replica") {
		t.Fatalf("inventory status=%d body=%s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/setup/current-step", strings.NewReader(`{"step":"run-doctor"}`)))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "run-doctor") {
		t.Fatalf("setup step status=%d body=%s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/agents/local/discover", strings.NewReader(`{"roots":[]}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("discover status=%d body=%s", res.Code, res.Body.String())
	}
	var discovered struct {
		Discoveries []struct {
			ID string `json:"id"`
		} `json:"discoveries"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &discovered); err != nil || len(discovered.Discoveries) == 0 {
		t.Fatalf("discover body=%s err=%v", res.Body.String(), err)
	}
	res = httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/discoveries/"+discovered.Discoveries[0].ID+"/adopt", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "adopted") {
		t.Fatalf("adopt status=%d body=%s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/alerts/alert-contabo-lag/acknowledge", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "acknowledged") {
		t.Fatalf("acknowledge status=%d body=%s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/sandboxes", strings.NewReader(`{"source_id":"prod","engine":"postgres"}`)))
	if res.Code != http.StatusAccepted || !strings.Contains(res.Body.String(), "job_id") {
		t.Fatalf("sandbox job status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestRestoreApprovalRoute(t *testing.T) {
	app, err := New(Config{DataDirectory: t.TempDir()}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	app.Handler().ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/approvals", strings.NewReader(`{"source_id":"production-postgres","reason":"bad deployment","target":"production replacement","requested_by":"console"}`)))
	if res.Code != http.StatusCreated || !strings.Contains(res.Body.String(), `"status":"pending"`) {
		t.Fatalf("approval status=%d body=%s", res.Code, res.Body.String())
	}
}
