package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStack(t *testing.T) (*Middleware, *http.ServeMux) {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "auth-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewMiddleware(store)
	mux := http.NewServeMux()
	m.RegisterPublic(mux, nil, nil)
	m.RegisterProtected(mux)
	mux.HandleFunc("GET /api/v1/ping", func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFromContext(r.Context())
		if !ok {
			http.Error(w, "no principal", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("pong:" + p.Role))
	})
	return m, mux
}

func do(t *testing.T, h http.Handler, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var rd *strings.Reader
	if body == "" {
		rd = strings.NewReader("")
	} else {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func TestBootstrapLoginAndAccess(t *testing.T) {
	m, mux := newTestStack(t)
	h := m.Protect(mux)

	// Before bootstrap the public status probe reports the appliance is unset.
	res := do(t, h, "GET", "/api/v1/auth/bootstrap-status", "", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("bootstrap-status status=%d body=%s", res.Code, res.Body.String())
	}
	var status struct {
		Bootstrapped bool `json:"bootstrapped"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Bootstrapped {
		t.Fatal("expected bootstrapped=false before admin creation")
	}

	if res := do(t, h, "POST", "/api/v1/auth/bootstrap", `{"setup_token":"tok","username":"admin@example.test","password":"correct horse battery"}`, nil); res.Code != http.StatusCreated {
		t.Fatalf("bootstrap status=%d body=%s", res.Code, res.Body.String())
	}

	// After bootstrap the probe flips and no longer advertises first-run.
	res = do(t, h, "GET", "/api/v1/auth/bootstrap-status", "", nil)
	if err := json.Unmarshal(res.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Bootstrapped {
		t.Fatal("expected bootstrapped=true after admin creation")
	}

	// Replay must fail: admin already provisioned.
	if res := do(t, h, "POST", "/api/v1/auth/bootstrap", `{"setup_token":"tok","username":"x@y.z","password":"another long password"}`, nil); res.Code != http.StatusForbidden {
		t.Fatalf("bootstrap replay status=%d body=%s", res.Code, res.Body.String())
	}

	// Wrong password rejected.
	if res := do(t, h, "POST", "/api/v1/auth/login", `{"username":"admin@example.test","password":"wrong password here"}`, nil); res.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status=%d", res.Code)
	}

	res = do(t, h, "POST", "/api/v1/auth/login", `{"username":"admin@example.test","password":"correct horse battery"}`, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", res.Code, res.Body.String())
	}
	cookie := res.Result().Cookies()
	if len(cookie) == 0 || cookie[0].Name != sessionCookie {
		t.Fatal("no session cookie set")
	}
	c := cookie[0]
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie flags wrong: %+v", c)
	}

	authed := map[string]string{"Cookie": c.Name + "=" + c.Value}
	if res := do(t, h, "GET", "/api/v1/ping", "", authed); res.Code != 200 || !strings.Contains(res.Body.String(), "pong:admin") {
		t.Fatalf("authed ping status=%d body=%s", res.Code, res.Body.String())
	}

	// Bearer token issuance and use.
	res = do(t, h, "POST", "/api/v1/auth/tokens", `{"username":"admin@example.test","password":"correct horse battery"}`, nil)
	if res.Code != http.StatusCreated {
		t.Fatalf("token issue status=%d body=%s", res.Code, res.Body.String())
	}
	var tok struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &tok)
	bearer := map[string]string{"Authorization": "Bearer " + tok.Token}
	if res := do(t, h, "GET", "/api/v1/ping", "", bearer); res.Code != 200 || !strings.Contains(res.Body.String(), "pong") {
		t.Fatalf("bearer ping status=%d body=%s", res.Code, res.Body.String())
	}

	// Tampered token fails.
	bad := map[string]string{"Authorization": "Bearer " + tok.Token + "aa"}
	if res := do(t, h, "GET", "/api/v1/ping", "", bad); res.Code != http.StatusUnauthorized {
		t.Fatalf("tampered bearer status=%d", res.Code)
	}

	// Logout kills the session.
	if res := do(t, h, "POST", "/api/v1/auth/logout", "", authed); res.Code != 200 {
		t.Fatalf("logout status=%d", res.Code)
	}
	if res := do(t, h, "GET", "/api/v1/ping", "", authed); res.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout status=%d", res.Code)
	}
}

func TestUnauthenticatedProtectedPaths(t *testing.T) {
	m, mux := newTestStack(t)
	h := m.Protect(mux)
	for _, path := range []string{"/api/v1/overview", "/api/v1/jobs", "/api/v1/status", "/api/v1"} {
		if res := do(t, h, "GET", path, "", nil); res.Code != http.StatusUnauthorized {
			t.Fatalf("%s unauth status=%d want 401", path, res.Code)
		}
	}
	for _, path := range []string{"/health", "/ready", "/", "/assets/app.js", "/setup/status"} {
		if res := do(t, h, "GET", path, "", nil); res.Code == http.StatusUnauthorized {
			t.Fatalf("%s must be reachable pre-auth", path)
		}
	}
}

func TestSessionExpiryWithFakeClock(t *testing.T) {
	store, err := NewStore("")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-2 * time.Hour)
	store.now = func() time.Time { return now }
	_ = NewMiddleware(store)
	if err := store.BootstrapAdmin("root", "long enough password"); err != nil {
		t.Fatal(err)
	}
	id, _, err := store.Login("root", "long enough password")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.AuthenticateSession(id); !ok {
		t.Fatal("fresh session should authenticate")
	}
	now = now.Add(25 * time.Hour)
	if _, ok := store.AuthenticateSession(id); ok {
		t.Fatal("expired session must not authenticate")
	}
}

func TestRateLimitFixedWindow(t *testing.T) {
	store, _ := NewStore("")
	m := NewMiddleware(store)
	ip := "10.1.1.9|/api/v1/auth/login"
	now := time.Now().UTC()
	okCount := 0
	for i := 0; i < loginMaxTries; i++ {
		if m.allow(ip, now) {
			okCount++
		}
	}
	if okCount != loginMaxTries {
		t.Fatalf("expected %d allowed attempts, got %d", loginMaxTries, okCount)
	}
	if m.allow(ip, now.Add(time.Second)) {
		t.Fatal("attempt beyond limit must be denied within window")
	}
	if !m.allow(ip, now.Add(loginWindow)) {
		t.Fatal("new window must allow again")
	}
	if m.allow("", now) {
		t.Fatal("empty IP must be denied (fail-closed)")
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BootstrapAdmin("root", "long enough password"); err != nil {
		t.Fatal(err)
	}
	token, _, err := store.IssueToken("root", "long enough password")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Bootstrapped() {
		t.Fatal("admin credential did not persist")
	}
	if p, ok := reopened.AuthenticateToken(token); !ok || p.Role != RoleAdmin {
		t.Fatalf("token did not survive reopen: %+v %v", p, ok)
	}
	info, err := statMode(path)
	if err != nil {
		t.Fatal(err)
	}
	if info != "0600" {
		t.Fatalf("auth state mode = %s, want 0600", info)
	}
}

func TestShortPasswordRejected(t *testing.T) {
	store, _ := NewStore("")
	if err := store.BootstrapAdmin("root", "short"); err == nil {
		t.Fatal("weak password accepted")
	}
}
