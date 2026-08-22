package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

const (
	testAdminUser = "admin"
	testAdminPass = "correct-horse-battery-staple"
)

// authedFixture is an appliance whose administrator has been bootstrapped
// through the real setup-token flow, with live session credentials.
type authedFixture struct {
	App    *Appliance
	H      http.Handler
	Cookie *http.Cookie
}

func newAuthedFixture(t *testing.T, cfg Config) *authedFixture {
	t.Helper()
	dir := t.TempDir()
	cfg.DataDirectory = dir
	cfg.SetupTokenPath = filepath.Join(dir, "setup-token")
	app, err := New(cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	token, err := app.EnsureSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	h := app.Handler()
	payload, _ := json.Marshal(map[string]string{
		"setup_token": token,
		"username":    testAdminUser,
		"password":    testAdminPass,
	})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", bytes.NewReader(payload)))
	if res.Code != http.StatusCreated {
		t.Fatalf("bootstrap status=%d body=%s", res.Code, res.Body.String())
	}
	for _, c := range res.Result().Cookies() {
		if c.Name == "dbvault_session" && c.Value != "" {
			return &authedFixture{App: app, H: h, Cookie: c}
		}
	}
	t.Fatal("bootstrap did not issue a session cookie")
	return nil
}

func (f *authedFixture) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if body != "" {
		req = httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(f.Cookie)
	res := httptest.NewRecorder()
	f.H.ServeHTTP(res, req)
	return res
}

func (f *authedFixture) issueBearer(t *testing.T) string {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"username": testAdminUser, "password": testAdminPass})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/tokens", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	f.H.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("token issuance status=%d body=%s", res.Code, res.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("token issuance body=%s err=%v", res.Body.String(), err)
	}
	return out.Token
}

func mustOK(t *testing.T, res *httptest.ResponseRecorder, want int, contains ...string) {
	t.Helper()
	if res.Code != want {
		t.Fatalf("status=%d want=%d body=%s", res.Code, want, res.Body.String())
	}
	for _, s := range contains {
		if !bytes.Contains(res.Body.Bytes(), []byte(s)) {
			t.Fatalf("body missing %q: %s", s, res.Body.String())
		}
	}
}
