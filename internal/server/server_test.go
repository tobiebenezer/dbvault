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
	f := newAuthedFixture(t, Config{Listen: "127.0.0.1:0"})

	mustOK(t, f.do(t, http.MethodGet, "/api/v1/overview", ""), http.StatusOK, "protected_databases")
	mustOK(t, f.do(t, http.MethodGet, "/api/v1/sources/prod/recovery-timeline", ""), http.StatusOK, "continuous")
	mustOK(t, f.do(t, http.MethodPost, "/api/v1/policies/simulate", `{"policy":{"backup_frequency":"Every 12 hours","replica_count":2}}`), http.StatusOK, "estimated_thirty_day_bytes")
	mustOK(t, f.do(t, http.MethodPost, "/api/v1/sandboxes", `{"source_id":"prod","engine":"postgres"}`), http.StatusAccepted, "sandbox-")
}

func TestJobRoutesAndSSESnapshot(t *testing.T) {
	f := newAuthedFixture(t, Config{Demo: true})

	mustOK(t, f.do(t, http.MethodGet, "/api/v1/jobs", ""), http.StatusOK, "job-backup-demo")
	mustOK(t, f.do(t, http.MethodGet, "/api/v1/events/jobs?once=1", ""), http.StatusOK, "event: job.")
	mustOK(t, f.do(t, http.MethodPost, "/api/v1/jobs/job-backup-demo/cancel", ""), http.StatusOK, "cancelled")
	mustOK(t, f.do(t, http.MethodPost, "/api/v1/jobs/job-replica-demo/retry", ""), http.StatusAccepted, "new_job_id")
}

func TestConnectedUIActionRoutes(t *testing.T) {
	f := newAuthedFixture(t, Config{Demo: true})

	mustOK(t, f.do(t, http.MethodGet, "/api/v1/inventory", ""), http.StatusOK, "contabo-replica")
	mustOK(t, f.do(t, http.MethodPost, "/api/v1/setup/current-step", `{"step":"run-doctor"}`), http.StatusOK, "run-doctor")

	res := f.do(t, http.MethodPost, "/api/v1/agents/local/discover", `{"roots":[]}`)
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
	mustOK(t, f.do(t, http.MethodPost, "/api/v1/discoveries/"+discovered.Discoveries[0].ID+"/adopt", ""), http.StatusOK, "adopted")
	mustOK(t, f.do(t, http.MethodPost, "/api/v1/alerts/alert-contabo-lag/acknowledge", ""), http.StatusOK, "acknowledged")
	mustOK(t, f.do(t, http.MethodPost, "/api/v1/sandboxes", `{"source_id":"prod","engine":"postgres"}`), http.StatusAccepted, "job_id")
}

func TestRestoreApprovalRoute(t *testing.T) {
	f := newAuthedFixture(t, Config{})
	mustOK(t, f.do(t, http.MethodPost, "/api/v1/approvals", `{"source_id":"production-postgres","reason":"bad deployment","target":"production replacement","requested_by":"console"}`), http.StatusCreated, `"status":"pending"`)
}

func TestGarbageCollectionRoutes(t *testing.T) {
	f := newAuthedFixture(t, Config{Demo: true})
	mustOK(t, f.do(t, http.MethodPost, "/api/v1/gc/plan", ""), http.StatusOK, "ReclaimableBytes")
	mustOK(t, f.do(t, http.MethodPost, "/api/v1/gc/run", ""), http.StatusOK, `"status":"completed"`)
}

func TestNotificationChannelsCreateAndValidate(t *testing.T) {
	f := newAuthedFixture(t, Config{})

	mustOK(t, f.do(t, http.MethodPost, "/api/v1/notifications/channels", `{"name":"Bad URL","type":"webhook","url":"http://127.0.0.1:59999/unreachable"}`), http.StatusBadRequest, "unreachable")
	mustOK(t, f.do(t, http.MethodPost, "/api/v1/notifications/channels", `{"name":"","type":"email"}`), http.StatusBadRequest)
}

// TestSetupRequiredRegardlessOfEnvDBURL pins the A5 fix: DBVAULT_DATABASE_URL
// must not bypass setup; bootstrap still demands a valid one-time token.
func TestSetupRequiredRegardlessOfEnvDBURL(t *testing.T) {
	t.Setenv("DBVAULT_DATABASE_URL", "postgresql://postgres:test@127.0.0.1:5432/testdb")
	dir := t.TempDir()
	app, err := New(Config{DataDirectory: dir, SetupTokenPath: filepath.Join(dir, "setup-token")}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	token, err := app.EnsureSetupToken()
	if err != nil || token == "" {
		t.Fatalf("setup token must be ensured even with external DB URL: %q err=%v", token, err)
	}
	h := app.Handler()

	wrong := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"setup_token": "wrong-token-value", "username": "admin", "password": testAdminPass})
	h.ServeHTTP(wrong, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", bytes.NewReader(body)))
	if wrong.Code != http.StatusForbidden {
		t.Fatalf("invalid token with env set: status=%d want=403 body=%s", wrong.Code, wrong.Body.String())
	}

	right := httptest.NewRecorder()
	body, _ = json.Marshal(map[string]string{"setup_token": token, "username": "admin", "password": testAdminPass})
	h.ServeHTTP(right, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", bytes.NewReader(body)))
	if right.Code != http.StatusCreated {
		t.Fatalf("valid token with env set: status=%d want=201 body=%s", right.Code, right.Body.String())
	}
}
