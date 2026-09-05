package sqlite

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

func createTestSQLiteDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	cmd := exec.Command("sqlite3", dbPath, `
		CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT UNIQUE);
		CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER REFERENCES users(id), amount REAL);
		CREATE INDEX idx_orders_user ON orders(user_id);
		CREATE VIEW active_users AS SELECT * FROM users WHERE email IS NOT NULL;
		INSERT INTO users (name, email) VALUES ('Alice', 'alice@example.com'), ('Bob', 'bob@example.com');
		INSERT INTO orders (user_id, amount) VALUES (1, 99.50), (2, 149.00);
		PRAGMA journal_mode = WAL;
	`)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to initialize test sqlite database: %v", err)
	}
	return dbPath
}

func TestDriverNameAndDescriptor(t *testing.T) {
	d := New("")
	if d.Name() != "sqlite" {
		t.Fatalf("expected sqlite, got %s", d.Name())
	}
}

func TestDriverValidate(t *testing.T) {
	d := New("")

	t.Run("valid database", func(t *testing.T) {
		dbPath := createTestSQLiteDB(t)
		if err := d.Validate(context.Background(), domain.Source{Path: dbPath}); err != nil {
			t.Fatalf("expected valid: %v", err)
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		if err := d.Validate(context.Background(), domain.Source{Path: "/nonexistent/test.db"}); err == nil {
			t.Fatal("expected error for non-existent file")
		}
	})

	t.Run("directory path", func(t *testing.T) {
		dir := t.TempDir()
		if err := d.Validate(context.Background(), domain.Source{Path: dir}); err == nil {
			t.Fatal("expected error for directory path")
		}
	})

	t.Run("invalid header file", func(t *testing.T) {
		dir := t.TempDir()
		badFile := filepath.Join(dir, "corrupt.db")
		if err := os.WriteFile(badFile, []byte("NOT_A_SQLITE_DATABASE_HEADER"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := d.Validate(context.Background(), domain.Source{Path: badFile}); err == nil {
			t.Fatal("expected error for file missing sqlite header")
		}
	})
}

func TestDriverInspect(t *testing.T) {
	d := New("")
	dbPath := createTestSQLiteDB(t)

	insp, err := d.Inspect(context.Background(), domain.Source{Path: dbPath})
	if err != nil {
		t.Fatal(err)
	}

	if insp.PageSize <= 0 {
		t.Fatalf("expected positive page size, got %d", insp.PageSize)
	}
	if insp.PageCount <= 0 {
		t.Fatalf("expected positive page count, got %d", insp.PageCount)
	}
	if insp.SchemaDigest == "" || insp.SchemaDigest == "unknown" {
		t.Fatalf("expected valid schema digest, got %q", insp.SchemaDigest)
	}
	if insp.EngineVersion == "" || insp.EngineVersion == "unknown" {
		t.Fatalf("expected valid engine version, got %q", insp.EngineVersion)
	}
}

func TestDriverCreateSnapshot(t *testing.T) {
	d := New("")
	dbPath := createTestSQLiteDB(t)
	scratch := t.TempDir()

	art, err := d.CreateSnapshot(context.Background(), ports.SnapshotRequest{
		Source:           domain.Source{ID: "src-sqlite", Path: dbPath},
		ScratchDirectory: scratch,
	})
	if err != nil {
		t.Fatal(err)
	}

	if art.Size() <= 0 {
		t.Fatalf("expected positive size, got %d", art.Size())
	}
	if art.Metadata().PageSize <= 0 {
		t.Fatalf("unexpected metadata: %+v", art.Metadata())
	}
	if art.Metadata().SchemaDigest == "" || art.Metadata().SchemaDigest == "unknown" {
		t.Fatalf("unexpected schema digest in metadata: %+v", art.Metadata())
	}

	// Verify file exists
	if _, err := os.Stat(art.Path()); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}

	// Verify cleanup
	if err := art.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(art.Path()); !os.IsNotExist(err) {
		t.Fatalf("expected snapshot file to be removed after Close()")
	}
}
