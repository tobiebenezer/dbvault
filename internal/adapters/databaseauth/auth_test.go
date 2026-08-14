package databaseauth

import (
	"os"
	"strings"
	"testing"

	"github.com/dbvault/dbvault/internal/config"
)

func TestPostgresCredentialsArePrivateAndCleaned(t *testing.T) {
	material, err := PreparePostgres(t.TempDir(), config.PostgresSourceConfig{Host: "db", Port: 5432, Database: "app", Username: "dbvault"}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(material.Files[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	for _, env := range material.Environment {
		if strings.HasPrefix(env, "PGPASSWORD=") {
			t.Fatal("password leaked into environment")
		}
	}
	dir := material.Directory
	if err := material.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("credential directory still exists")
	}
}

func TestMySQLPasswordNotInArguments(t *testing.T) {
	material, err := PrepareMySQL(t.TempDir(), config.MySQLSourceConfig{Host: "db", Port: 3306, Database: "app", Username: "dbvault"}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range material.Arguments {
		if strings.Contains(arg, "secret") {
			t.Fatal("password leaked into arguments")
		}
	}
	if err := material.Cleanup(); err != nil {
		t.Fatal(err)
	}
}
