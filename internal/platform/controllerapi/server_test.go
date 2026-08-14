package controllerapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTenantHeaderRequired(t *testing.T) {
	srv := New(nil, "token")
	req := httptest.NewRequest(http.MethodGet, "/v1/sources", nil)
	req.Header.Set("Authorization", "Bearer token")
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.Code)
	}
}
