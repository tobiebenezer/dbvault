// Package pgdiag turns raw pg_dump permission failures into actionable,
// role-aware remediation guidance for PostgreSQL backup operators.
package pgdiag

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Connection carries the coordinates needed to run diagnostic queries.
type Connection struct {
	Host     string
	Port     int
	User     string
	Database string
	Password string
}

// deniedTablePattern matches pg_dump errors such as
// "permission denied for table migrations" or
// "permission denied for table public.migrations".
var deniedTablePattern = regexp.MustCompile(`(?i)permission denied for (?:table )?([A-Za-z_][\w$]*(?:\.[A-Za-z_][\w$]*)?|"[^"]+"(?:\."[^"]+")?)`)

// Audit runs a read-only privilege diagnostic and returns the comma-separated
// list of tables the connected role cannot SELECT. Replaceable in tests.
var Audit = func(ctx context.Context, conn Connection) (string, error) {
	return defaultAudit(ctx, conn)
}

// IsPermissionDenied reports whether pg_dump/pg_restore stderr indicates a
// table-level permission failure.
func IsPermissionDenied(stderr string) bool {
	return deniedTablePattern.MatchString(stderr)
}

// DeniedTables extracts the distinct table names mentioned in permission
// errors found in stderr.
func DeniedTables(stderr string) []string {
	matches := deniedTablePattern.FindAllStringSubmatch(stderr, -1)
	seen := map[string]bool{}
	out := []string{}
	for _, m := range matches {
		t := strings.TrimSpace(m[1])
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// RemediationSQL returns the GRANT statements that give role read access to
// every table and sequence in the public schema, plus default privileges for
// future tables.
func RemediationSQL(role string) string {
	role = roleForSQL(role)
	return "GRANT USAGE ON SCHEMA public TO " + role + ";\n" +
		"GRANT SELECT ON ALL TABLES IN SCHEMA public TO " + role + ";\n" +
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO " + role + ";\n" +
		"-- future tables: run as the role that owns/creates the tables\n" +
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO " + role + ";\n" +
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO " + role + ";"
}

// Hint converts pg_dump stderr into a diagnostic message with the complete
// list of unreadable tables and copy-paste remediation SQL. It returns ""
// when the failure is not a table permission problem.
func Hint(ctx context.Context, stderr string, conn Connection) string {
	if !IsPermissionDenied(stderr) {
		return ""
	}

	detected := DeniedTables(stderr)
	if len(detected) == 0 {
		return ""
	}

	denied := detected
	if rows, err := Audit(ctx, conn); err == nil && strings.TrimSpace(rows) != "" {
		if list := splitAuditList(rows); len(list) > 0 {
			denied = list
		}
	}
	sort.Strings(denied)

	var b strings.Builder
	fmt.Fprintf(&b, "pg_dump cannot read %d table(s) with role %s: %s\n\n", len(denied), roleForLabel(conn.User), strings.Join(denied, ", "))
	b.WriteString("Run as a database superuser or the table owner to fix:\n\n")
	b.WriteString(RemediationSQL(conn.User))
	b.WriteString("\n\nThen retry the backup. If grants are not possible (managed hosting), exclude the affected tables using the database's table-exclusion setting in DBVault.")
	b.WriteString("\n\nOriginal pg_dump error: " + firstLine(stderr))
	return b.String()
}

func defaultAudit(ctx context.Context, conn Connection) (string, error) {
	if conn.Host == "" && conn.Database == "" {
		return "", fmt.Errorf("no connection details for privilege audit")
	}
	query := `SELECT string_agg(quote_ident(schemaname) || '.' || quote_ident(tablename), ', ' ORDER BY schemaname, tablename) FROM pg_catalog.pg_tables WHERE schemaname NOT IN ('pg_catalog', 'information_schema') AND NOT has_table_privilege(current_user, quote_ident(schemaname) || '.' || quote_ident(tablename), 'SELECT')`

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	args := []string{"-w", "-A", "-t", "-q"}
	if conn.Host != "" {
		args = append(args, "-h", conn.Host)
	}
	if conn.Port != 0 {
		args = append(args, "-p", strconv.Itoa(conn.Port))
	}
	if conn.User != "" {
		args = append(args, "-U", conn.User)
	}
	if conn.Database != "" {
		args = append(args, "-d", conn.Database)
	}
	args = append(args, "-c", query)

	cmd := exec.CommandContext(ctx, "psql", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "PGCONNECT_TIMEOUT=10")
	if conn.Password != "" {
		cmd.Env = append(cmd.Env, "PGPASSWORD="+conn.Password)
	}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("privilege audit failed: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func splitAuditList(rows string) []string {
	raw := strings.Split(rows, ",")
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if t := strings.TrimSpace(r); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func roleForSQL(role string) string {
	if strings.TrimSpace(role) == "" {
		return "<dbvault_role>"
	}
	return role
}

func roleForLabel(role string) string {
	if strings.TrimSpace(role) == "" {
		return "(unknown role)"
	}
	return strconv.Quote(role)
}

func firstLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 300 {
			line = line[:300] + "..."
		}
		return line
	}
	return "(no error detail)"
}
