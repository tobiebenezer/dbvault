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

func TestDeniedObjectsFromKinds(t *testing.T) {
	stderr := "pg_dump: error: permission denied for table migrations\n" +
		"pg_dump: error: permission denied for sequence public.users_id_seq\n" +
		"pg_dump: error: permission denied for schema private\n" +
		"pg_dump: error: permission denied for table migrations"
	obj := DeniedObjectsFrom(stderr)
	if len(obj.Tables) != 1 || obj.Tables[0] != "migrations" {
		t.Fatalf("tables = %v", obj.Tables)
	}
	if len(obj.Sequences) != 1 || obj.Sequences[0] != "public.users_id_seq" {
		t.Fatalf("sequences = %v", obj.Sequences)
	}
	if len(obj.Schemas) != 1 || obj.Schemas[0] != "private" {
		t.Fatalf("schemas = %v", obj.Schemas)
	}
	if obj.Count() != 3 {
		t.Fatalf("count = %d", obj.Count())
	}
}

func TestAuditObjectsQueries(t *testing.T) {
	orig := runPsql
	var queries []string
	runPsql = func(ctx context.Context, conn Connection, query string) (string, error) {
		queries = append(queries, query)
		switch {
		case strings.Contains(query, "pg_tables"):
			return "public.migrations, public.users", nil
		case strings.Contains(query, "pg_sequences"):
			return "public.users_id_seq", nil
		case strings.Contains(query, "pg_namespace"):
			return "", nil
		}
		return "", errors.New("unexpected query")
	}
	defer func() { runPsql = orig }()

	obj, err := AuditObjects(context.Background(), Connection{Host: "db", Database: "app"})
	if err != nil {
		t.Fatalf("AuditObjects: %v", err)
	}
	if len(obj.Tables) != 2 || len(obj.Sequences) != 1 || len(obj.Schemas) != 0 {
		t.Fatalf("audit = %+v", obj)
	}
	if len(queries) != 3 {
		t.Fatalf("expected 3 audit queries, ran %d", len(queries))
	}
}

func TestAuditObjectsAllQueriesFail(t *testing.T) {
	orig := runPsql
	runPsql = func(ctx context.Context, conn Connection, query string) (string, error) {
		return "", errors.New("psql not found")
	}
	defer func() { runPsql = orig }()

	if _, err := AuditObjects(context.Background(), Connection{Host: "db", Database: "app"}); err == nil {
		t.Fatal("expected error when every audit query fails")
	}
}

func TestElevateGeneratesGrantSQL(t *testing.T) {
	orig := runPsql
	var executed []string
	runPsql = func(ctx context.Context, conn Connection, query string) (string, error) {
		executed = append(executed, query)
		if conn.User != "dba" {
			t.Fatalf("elevation must run as elevation role, got %q", conn.User)
		}
		return "", nil
	}
	defer func() { runPsql = orig }()

	obj := DeniedObjects{
		Tables:    []string{"public.migrations", `weird"name`},
		Sequences: []string{"public.users_id_seq"},
		Schemas:   []string{"private"},
	}
	granted, err := Elevate(context.Background(), Connection{Host: "db", User: "dba", Database: "app", Password: "pw"}, "app_user", obj)
	if err != nil {
		t.Fatalf("Elevate: %v", err)
	}
	want := []string{
		`GRANT SELECT ON public.migrations TO "app_user";`,
		`GRANT USAGE, SELECT ON public.users_id_seq TO "app_user";`,
		`GRANT USAGE ON private TO "app_user";`,
	}
	if len(executed) != 1 {
		t.Fatalf("expected single psql call, got %d", len(executed))
	}
	for _, w := range want {
		if !strings.Contains(executed[0], w) {
			t.Fatalf("grant SQL missing %q\n---\n%s", w, executed[0])
		}
	}
	if strings.Contains(executed[0], `weird"name`) {
		t.Fatal("unsafe identifier must be skipped")
	}
	if len(granted) != 3 {
		t.Fatalf("granted = %v", granted)
	}
}

func TestRevokeMirrorsGrants(t *testing.T) {
	orig := runPsql
	var executed []string
	runPsql = func(ctx context.Context, conn Connection, query string) (string, error) {
		executed = append(executed, query)
		return "", nil
	}
	defer func() { runPsql = orig }()

	obj := DeniedObjects{Tables: []string{"public.migrations"}, Sequences: []string{"public.users_id_seq"}, Schemas: []string{"private"}}
	if err := Revoke(context.Background(), Connection{Host: "db", User: "dba", Database: "app"}, "app_user", obj); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	for _, w := range []string{
		`REVOKE SELECT ON public.migrations FROM "app_user";`,
		`REVOKE USAGE, SELECT ON public.users_id_seq FROM "app_user";`,
		`REVOKE USAGE ON private FROM "app_user";`,
	} {
		if !strings.Contains(executed[0], w) {
			t.Fatalf("revoke SQL missing %q\n---\n%s", w, executed[0])
		}
	}
}

func TestElevateRejectsSameRole(t *testing.T) {
	orig := runPsql
	runPsql = func(ctx context.Context, conn Connection, query string) (string, error) {
		t.Fatal("psql must not run when roles are identical")
		return "", nil
	}
	defer func() { runPsql = orig }()

	obj := DeniedObjects{Tables: []string{"public.migrations"}}
	if _, err := Elevate(context.Background(), Connection{User: "app_user"}, "app_user", obj); err == nil {
		t.Fatal("expected error when elevation role equals backup role")
	}
}

func TestElevateWithoutPlainObjectsFails(t *testing.T) {
	orig := runPsql
	runPsql = func(ctx context.Context, conn Connection, query string) (string, error) {
		t.Fatal("psql must not run when no grantable objects")
		return "", nil
	}
	defer func() { runPsql = orig }()

	obj := DeniedObjects{Tables: []string{`we"ird`}}
	if _, err := Elevate(context.Background(), Connection{User: "dba"}, "app_user", obj); err == nil {
		t.Fatal("expected error when no object can be safely granted")
	}
}
