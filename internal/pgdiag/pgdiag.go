// Package pgdiag turns raw pg_dump permission failures into actionable,
// role-aware remediation guidance for PostgreSQL backup operators. When an
// opt-in elevation credential is available it can also grant the missing read
// privileges itself and revoke them again afterwards.
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

// DeniedObjects holds the objects a role cannot read, grouped by kind.
type DeniedObjects struct {
	Tables    []string // schema-qualified when known (e.g. public.migrations)
	Sequences []string
	Schemas   []string
}

// IsEmpty reports whether no denied objects were found.
func (o DeniedObjects) IsEmpty() bool {
	return len(o.Tables) == 0 && len(o.Sequences) == 0 && len(o.Schemas) == 0
}

// Count returns the total number of denied objects.
func (o DeniedObjects) Count() int {
	return len(o.Tables) + len(o.Sequences) + len(o.Schemas)
}

// deniedObjectPattern matches pg_dump / PostgreSQL error messages for tables,
// sequences, schemas, views, and relations.
var deniedObjectPattern = regexp.MustCompile(`(?i)permission denied for (?:(table|sequence|schema|relation|view|materialized view)\s+)?([A-Za-z_][\w$]*(?:\.[A-Za-z_][\w$]*)?|"[^"]+"(?:\."[^"]+")?)`)

// plainIdent matches an unquoted safe identifier.
var plainIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*$`)

// runPsql executes psql with the given connection and query. Replaceable in tests.
var runPsql = func(ctx context.Context, conn Connection, query string) (string, error) {
	return defaultRunPsql(ctx, conn, query)
}

// Audit runs the table privilege diagnostic and returns the comma-separated
// list of tables the connected role cannot SELECT. Replaceable in tests.
var Audit = func(ctx context.Context, conn Connection) (string, error) {
	return defaultAudit(ctx, conn)
}

// IsPermissionDenied reports whether pg_dump/pg_restore stderr indicates a
// permission failure on a table, sequence, schema, view, or relation.
func IsPermissionDenied(stderr string) bool {
	return deniedObjectPattern.MatchString(stderr)
}

// DeniedTables extracts the distinct table/view/relation names mentioned in
// permission errors found in stderr.
func DeniedTables(stderr string) []string {
	obj := DeniedObjectsFrom(stderr)
	return obj.Tables
}

// DeniedObjectsFrom parses permission-denied stderr into objects grouped by
// kind (tables, sequences, schemas).
func DeniedObjectsFrom(stderr string) DeniedObjects {
	var out DeniedObjects
	seen := map[string]bool{}
	for _, m := range deniedObjectPattern.FindAllStringSubmatch(stderr, -1) {
		name := strings.TrimSpace(m[2])
		kind := strings.ToLower(strings.TrimSpace(m[1]))
		key := kind + "|" + name
		if name == "" || seen[key] {
			continue
		}
		seen[key] = true
		switch kind {
		case "sequence":
			out.Sequences = append(out.Sequences, name)
		case "schema":
			out.Schemas = append(out.Schemas, name)
		default:
			// "table", "relation", "view", "materialized view", or bare identifier
			out.Tables = append(out.Tables, name)
		}
	}
	return out
}

// RemediationSQL returns the GRANT statements that give role read access to
// every table and sequence in the public schema, plus default privileges for
// future tables.
func RemediationSQL(role string) string {
	roleTarget := roleForSQL(role)
	return "GRANT USAGE ON SCHEMA public TO " + roleTarget + ";\n" +
		"GRANT SELECT ON ALL TABLES IN SCHEMA public TO " + roleTarget + ";\n" +
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO " + roleTarget + ";\n" +
		"-- future tables: run as the role that owns/creates the tables\n" +
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO " + roleTarget + ";\n" +
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO " + roleTarget + ";"
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

const auditTablesQuery = `SELECT string_agg(quote_ident(n.nspname) || '.' || quote_ident(c.relname), ', ' ORDER BY n.nspname, c.relname) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind IN ('r', 'p', 'm') AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND NOT has_table_privilege(current_user, c.oid, 'SELECT')`

const auditSequencesQuery = `SELECT string_agg(quote_ident(n.nspname) || '.' || quote_ident(c.relname), ', ' ORDER BY n.nspname, c.relname) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind = 'S' AND n.nspname NOT IN ('pg_catalog', 'information_schema') AND NOT (has_sequence_privilege(current_user, c.oid, 'USAGE') OR has_sequence_privilege(current_user, c.oid, 'SELECT'))`

const auditSchemasQuery = `SELECT string_agg(quote_ident(nspname), ', ' ORDER BY nspname) FROM pg_catalog.pg_namespace WHERE nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast') AND NOT has_schema_privilege(current_user, quote_ident(nspname), 'USAGE')`

// AuditObjects audits tables, sequences, and schemas the connected role
// cannot read. The tables query is authoritative; a failure there fails the
// whole audit.
func AuditObjects(ctx context.Context, conn Connection) (DeniedObjects, error) {
	var out DeniedObjects
	var tablesErr error
	if rows, err := runPsql(ctx, conn, auditTablesQuery); err != nil {
		tablesErr = err
	} else {
		out.Tables = splitAuditList(rows)
	}
	if rows, err := runPsql(ctx, conn, auditSequencesQuery); err == nil {
		out.Sequences = splitAuditList(rows)
	}
	if rows, err := runPsql(ctx, conn, auditSchemasQuery); err == nil {
		out.Schemas = splitAuditList(rows)
	}
	if out.IsEmpty() && tablesErr != nil {
		return out, tablesErr
	}
	return out, nil
}

// Elevate grants the backup role (grantee) read privileges on the denied
// objects using the elevation connection. Identifiers are safely validated
// and quoted. It returns the list of object identifiers that were granted.
func Elevate(ctx context.Context, elevConn Connection, grantee string, obj DeniedObjects) ([]string, error) {
	grantee = strings.TrimSpace(grantee)
	if grantee == "" {
		return nil, fmt.Errorf("backup role for elevation is required")
	}
	if grantee == elevConn.User {
		return nil, fmt.Errorf("elevation role must differ from the backup role")
	}
	stmts, granted := elevationStatements(obj, grantee, true)
	if len(stmts) == 0 {
		return nil, fmt.Errorf("no grantable objects (only valid identifiers are elevated)")
	}
	if _, err := runPsql(ctx, elevConn, strings.Join(stmts, ";\n")+";"); err != nil {
		return nil, err
	}
	return granted, nil
}

// Revoke removes the read privileges Elevate granted. It is safe to call
// multiple times and a no-op when no grantable objects remain.
func Revoke(ctx context.Context, elevConn Connection, grantee string, obj DeniedObjects) error {
	grantee = strings.TrimSpace(grantee)
	if grantee == "" || grantee == elevConn.User {
		return fmt.Errorf("invalid revoke parameters")
	}
	stmts, _ := elevationStatements(obj, grantee, false)
	if len(stmts) == 0 {
		return nil
	}
	_, err := runPsql(ctx, elevConn, strings.Join(stmts, ";\n")+";")
	return err
}

// elevationStatements builds the grant (or revoke) statements for the denied
// objects and returns the statements together with the sanitized object
// identifiers that were granted (empty list when revoking).
func elevationStatements(obj DeniedObjects, grantee string, grant bool) ([]string, []string) {
	role := pgQuoteIdent(grantee)
	var out []stmtAndID

	// 1. On grant, schemas must be granted USAGE before tables/sequences can be accessed
	if grant {
		for _, schema := range obj.Schemas {
			clean := cleanIdentPart(schema)
			if clean == "" {
				continue
			}
			q := pgQuoteIdent(clean)
			out = append(out, stmtAndID{q, fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s", q, role)})
		}
	}

	// 2. Tables, views, and relations
	for _, t := range obj.Tables {
		q := sanitizeQualified(t)
		if q == "" {
			continue
		}
		if grant {
			out = append(out, stmtAndID{q, fmt.Sprintf("GRANT SELECT ON %s TO %s", q, role)})
		} else {
			out = append(out, stmtAndID{q, fmt.Sprintf("REVOKE SELECT ON %s FROM %s", q, role)})
		}
	}

	// 3. Sequences
	for _, seq := range obj.Sequences {
		q := sanitizeQualified(seq)
		if q == "" {
			continue
		}
		if grant {
			out = append(out, stmtAndID{q, fmt.Sprintf("GRANT USAGE, SELECT ON %s TO %s", q, role)})
		} else {
			out = append(out, stmtAndID{q, fmt.Sprintf("REVOKE USAGE, SELECT ON %s FROM %s", q, role)})
		}
	}

	// 4. On revoke, schemas are revoked last after child objects have been revoked
	if !grant {
		for _, schema := range obj.Schemas {
			clean := cleanIdentPart(schema)
			if clean == "" {
				continue
			}
			q := pgQuoteIdent(clean)
			out = append(out, stmtAndID{q, fmt.Sprintf("REVOKE USAGE ON SCHEMA %s FROM %s", q, role)})
		}
	}

	stmts := make([]string, 0, len(out))
	ids := make([]string, 0, len(out))
	for _, s := range out {
		stmts = append(stmts, s.stmt)
		ids = append(ids, s.id)
	}
	return stmts, ids
}

type stmtAndID struct {
	id   string
	stmt string
}

// sanitizeQualified accepts plain or quoted (optionally schema-qualified)
// identifiers and safely re-quotes each component for injection-safe SQL.
func sanitizeQualified(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	parts := splitQualifiedIdent(name)
	if len(parts) == 0 {
		return ""
	}
	quoted := make([]string, len(parts))
	for i, p := range parts {
		clean := cleanIdentPart(p)
		if clean == "" {
			return ""
		}
		quoted[i] = pgQuoteIdent(clean)
	}
	return strings.Join(quoted, ".")
}

// splitQualifiedIdent splits an identifier by '.' while respecting quoted parts.
func splitQualifiedIdent(name string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	runes := []rune(name)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '"' {
			inQuote = !inQuote
			cur.WriteRune(r)
		} else if r == '.' && !inQuote {
			parts = append(parts, cur.String())
			cur.Reset()
		} else {
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

// cleanIdentPart strips matching surrounding double quotes and unescapes internal
// quotes ("" -> "), validating that the identifier contains no null bytes or control characters.
func cleanIdentPart(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	for _, r := range p {
		if r == 0 || r < 32 {
			return ""
		}
	}
	if strings.HasPrefix(p, `"`) && strings.HasSuffix(p, `"`) && len(p) >= 2 {
		inner := p[1 : len(p)-1]
		return strings.ReplaceAll(inner, `""`, `"`)
	}
	if plainIdent.MatchString(p) {
		return p
	}
	return ""
}

// pgQuoteIdent double-quotes a role name for safe use in generated SQL.
func pgQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func defaultRunPsql(ctx context.Context, conn Connection, query string) (string, error) {
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
		return "", fmt.Errorf("psql failed: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func defaultAudit(ctx context.Context, conn Connection) (string, error) {
	if conn.Host == "" && conn.Database == "" {
		return "", fmt.Errorf("no connection details for privilege audit")
	}
	return runPsql(ctx, conn, auditTablesQuery)
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
	r := strings.TrimSpace(role)
	if r == "" {
		return "<dbvault_role>"
	}
	if r == "<dbvault_role>" {
		return r
	}
	return pgQuoteIdent(r)
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
