package productexperience

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBIConnectionLifecycleAndDatasetScope(t *testing.T) {
	s, err := NewWithDemo(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	view, token, err := s.CreateBIConnection(BIConnectionInput{
		Name:     "Power BI",
		Provider: "powerbi",
		Datasets: []BIDatasetScope{{Database: "production-postgres", Table: "orders"}},
	})
	if err != nil || token == "" {
		t.Fatalf("create connection: view=%#v token=%q err=%v", view, token, err)
	}
	if err := s.AuthorizeBIFeed(view.ID, token, "production-postgres", "orders"); err != nil {
		t.Fatalf("authorized feed rejected: %v", err)
	}
	if err := s.AuthorizeBIFeed(view.ID, token, "production-postgres", "users"); err == nil {
		t.Fatal("out-of-scope dataset was authorized")
	}
	rotated, replacement, err := s.RotateBIConnection(view.ID)
	if err != nil || replacement == token || rotated.Status != "active" {
		t.Fatalf("rotate connection: view=%#v token=%q err=%v", rotated, replacement, err)
	}
	if err := s.AuthorizeBIFeed(view.ID, token, "production-postgres", "orders"); err == nil {
		t.Fatal("old token remained valid after rotation")
	}
	if err := s.AuthorizeBIFeed(view.ID, replacement, "production-postgres", "orders"); err != nil {
		t.Fatalf("replacement token rejected: %v", err)
	}
	if err := s.RevokeBIConnection(view.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeBIFeed(view.ID, replacement, "production-postgres", "orders"); err == nil {
		t.Fatal("revoked token remained valid")
	}
}

func TestBIConnectionsPersistWithoutPlaintextToken(t *testing.T) {
	root := t.TempDir()
	s, err := NewWithDemo(root, false)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := s.CreateBIConnection(BIConnectionInput{Name: "Python", Provider: "python", Datasets: []BIDatasetScope{{Table: "orders"}}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "bi_connections.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), token) {
		t.Fatal("plaintext BI token was persisted")
	}
	reloaded, err := NewWithDemo(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.BIConnections()) != 1 {
		t.Fatal("BI connection did not persist")
	}
}
