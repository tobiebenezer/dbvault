package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dbvault/dbvault/internal/application/controlplane"
	"github.com/dbvault/dbvault/internal/platform/controllerapi"
)

// Case set 1: every sampled /api/v1/* route must reject anonymous callers.
func TestAllAPIRoutesRejectAnonymousCallers(t *testing.T) {
	f := newAuthedFixture(t, Config{Demo: true})
	routes := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/status"},
		{http.MethodGet, "/api/v1/overview"},
		{http.MethodGet, "/api/v1/jobs"},
		{http.MethodGet, "/api/v1/inventory"},
		{http.MethodGet, "/api/v1/alerts"},
		{http.MethodGet, "/api/v1/sources/prod/recovery-timeline"},
		{http.MethodPost, "/api/v1/policies/simulate"},
		{http.MethodPost, "/api/v1/sandboxes"},
		{http.MethodPost, "/api/v1/approvals"},
		{http.MethodPost, "/api/v1/gc/run"},
		{http.MethodGet, "/api/v1/bi/powerbi/catalog"},
		{http.MethodGet, "/api/v1/keys/master"},
		{http.MethodPost, "/api/v1/keys/master/reveal"},
		{http.MethodPost, "/api/v1/keys/master/set"},
		{http.MethodPost, "/api/v1/keys/master/generate"},
		{http.MethodGet, "/api/v1/auth/session"},
	}
	for _, rt := range routes {
		res := httptest.NewRecorder()
		f.H.ServeHTTP(res, httptest.NewRequest(rt.method, rt.path, nil))
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s anonymous status=%d want=401", rt.method, rt.path, res.Code)
		}
	}
}

// Case set 2: master-key reveal is 403 by default even for an authenticated
// admin, and only 200 with explicit opt-in.
func TestMasterKeyRevealGatedByConfig(t *testing.T) {
	f := newAuthedFixture(t, Config{})
	if res := f.do(t, http.MethodPost, "/api/v1/keys/master/reveal", ""); res.Code != http.StatusForbidden {
		t.Fatalf("reveal default posture status=%d want=403 body=%s", res.Code, res.Body.String())
	}

	optIn := newAuthedFixture(t, Config{AllowMasterKeyReveal: true})
	if res := optIn.do(t, http.MethodPost, "/api/v1/keys/master/reveal", ""); res.Code != http.StatusOK {
		t.Fatalf("reveal opt-in status=%d want=200 body=%s", res.Code, res.Body.String())
	}
	if cc := optIn.do(t, http.MethodPost, "/api/v1/keys/master/reveal", "").Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("reveal Cache-Control=%q want=no-store", cc)
	}

	bearer := optIn.issueBearer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/keys/master/reveal", nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	res := httptest.NewRecorder()
	optIn.H.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("reveal via bearer status=%d want=200 body=%s", res.Code, res.Body.String())
	}
}

// Case set 3: a controller built without a token fails closed with 503.
func TestControllerFailsClosedWithoutToken(t *testing.T) {
	open := controllerapi.New(controlplane.NewStore(), "").Handler()
	req := httptest.NewRequest(http.MethodPost, "/v1/organisations", bytes.NewReader([]byte(`{"id":"org"}`)))
	res := httptest.NewRecorder()
	open.ServeHTTP(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("controller without token status=%d want=503", res.Code)
	}

	demo := controllerapi.New(controlplane.NewStore(), "")
	demo.InsecureDemo = true
	h := demo.Handler()
	req = httptest.NewRequest(http.MethodPost, "/v1/organisations", bytes.NewReader([]byte(`{"id":"org-1"}`)))
	req.Header.Set("X-DBVault-Organisation", "org-1")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code == http.StatusServiceUnavailable || res.Code == http.StatusUnauthorized {
		t.Fatalf("insecure-demo controller should serve, got %d", res.Code)
	}

	tokened := controllerapi.New(controlplane.NewStore(), "secret-token").Handler()
	req = httptest.NewRequest(http.MethodGet, "/v1/sources", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	res = httptest.NewRecorder()
	tokened.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("wrong controller token status=%d want=401", res.Code)
	}
}

// Case set 4: setup cannot be completed twice.
func TestSetupCannotBeReplayed(t *testing.T) {
	dir := t.TempDir()
	app, err := New(Config{DataDirectory: dir, SetupTokenPath: filepath.Join(dir, "setup-token")}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	token, err := app.EnsureSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"setup_token": token, "username": "admin", "password": testAdminPass})
	h := app.Handler()

	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", bytes.NewReader(payload)))
	if first.Code != http.StatusCreated {
		t.Fatalf("bootstrap status=%d body=%s", first.Code, first.Body.String())
	}

	replay := httptest.NewRecorder()
	h.ServeHTTP(replay, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", bytes.NewReader(payload)))
	if replay.Code != http.StatusForbidden {
		t.Fatalf("setup replay status=%d want=403 body=%s", replay.Code, replay.Body.String())
	}
	if _, err := app.EnsureSetupToken(); err == nil {
		t.Log("EnsureSetupToken after consume returned no error; token file must stay absent")
	}
}

// Case set 5: session cookies carry HttpOnly, Secure and SameSite=Strict.
func TestSessionCookieFlagsHardened(t *testing.T) {
	f := newAuthedFixture(t, Config{})

	login := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]string{"username": testAdminUser, "password": testAdminPass})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	f.H.ServeHTTP(login, req)

	cookies := login.Result().Cookies()
	var session *http.Cookie
	for _, c := range cookies {
		if c.Name == "dbvault_session" {
			session = c
		}
	}
	if session == nil {
		t.Fatalf("no session cookie set: %d %s", login.Code, login.Body.String())
	}
	if !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie flags HttpOnly=%v Secure=%v SameSite=%v", session.HttpOnly, session.Secure, session.SameSite)
	}
}
