package pgdiag

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const deniedStderr = "pg_dump: error: query failed: ERROR:  permission denied for table migrations\npg_dump: error: query was: LOCK TABLE public.migrations IN ACCESS SHARE MODE"

func TestIsPermissionDenied(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"denied", deniedStderr, true},
		{"schema-qualified", "pg_dump: error: permission denied for table public.migrations", true},
		{"connection-refused", "pg_dump: error: could not connect to server: Connection refused", false},
		{"auth-failed", "pg_dump: error: password authentication failed for user \"app\"", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPermissionDenied(tc.stderr); got != tc.want {
				t.Fatalf("IsPermissionDenied(%q) = %v, want %v", tc.stderr, got, tc.want)
			}
		})
	}
}

func TestDeniedTables(t *testing.T) {
	got := DeniedTables("permission denied for table migrations\npermission denied for table public.users\npermission denied for table migrations")
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct tables, got %v", got)
	}
}

func TestHintNonPermissionFailureReturnsEmpty(t *testing.T) {
	orig := Audit
	Audit = func(ctx context.Context, conn Connection) (string, error) {
		t.Fatal("audit must not run for non-permission failures")
		return "", nil
	}
	defer func() { Audit = orig }()

	if got := Hint(context.Background(), "connection refused", Connection{Host: "h", Database: "d"}); got != "" {
		t.Fatalf("expected empty hint, got %q", got)
	}
}

func TestHintWithAuditSuccess(t *testing.T) {
	orig := Audit
	Audit = func(ctx context.Context, conn Connection) (string, error) {
		if conn.User != "app_user" {
			t.Fatalf("audit must receive connection role, got %q", conn.User)
		}
		return "public.migrations, public.users, public.orders", nil
	}
	defer func() { Audit = orig }()

	hint := Hint(context.Background(), deniedStderr, Connection{Host: "db", Port: 5432, User: "app_user", Database: "app", Password: "pw"})
	for _, want := range []string{
		"3 table(s)",
		"app_user",
		"public.migrations",
		"public.users",
		"public.orders",
		"GRANT SELECT ON ALL TABLES IN SCHEMA public TO app_user;",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO app_user;",
		"permission denied for table migrations",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint missing %q\n---\n%s", want, hint)
		}
	}
}

func TestHintWhenAuditFailsFallsBackToDetectedTable(t *testing.T) {
	orig := Audit
	Audit = func(ctx context.Context, conn Connection) (string, error) {
		return "", errors.New("psql not found")
	}
	defer func() { Audit = orig }()

	hint := Hint(context.Background(), deniedStderr, Connection{Host: "db", User: "app_user", Database: "app"})
	if !strings.Contains(hint, "migrations") {
		t.Fatalf("expected detected table in fallback hint:\n%s", hint)
	}
	if strings.Contains(hint, "public.users") {
		t.Fatalf("audit result must not appear when audit failed:\n%s", hint)
	}
}

func TestHintWithoutRoleUsesPlaceholder(t *testing.T) {
	orig := Audit
	Audit = func(ctx context.Context, conn Connection) (string, error) {
		return "", errors.New("skip")
	}
	defer func() { Audit = orig }()

	hint := Hint(context.Background(), "permission denied for table migrations", Connection{})
	if !strings.Contains(hint, "<dbvault_role>") {
		t.Fatalf("expected placeholder role in hint:\n%s", hint)
	}
}

func TestRemediationSQL(t *testing.T) {
	sql := RemediationSQL("app_user")
	for _, want := range []string{
		"GRANT USAGE ON SCHEMA public TO app_user;",
		"GRANT SELECT ON ALL TABLES IN SCHEMA public TO app_user;",
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app_user;",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO app_user;",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO app_user;",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("remediation missing %q\n---\n%s", want, sql)
		}
	}
}
