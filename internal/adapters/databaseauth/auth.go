package databaseauth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dbvault/dbvault/internal/config"
)

type Material struct {
	Environment []string
	Arguments   []string
	Directory   string
	Files       []string
}

func (m Material) Cleanup() error {
	if m.Directory == "" {
		return nil
	}
	return os.RemoveAll(m.Directory)
}

func PreparePostgres(root string, source config.PostgresSourceConfig, password string) (Material, error) {
	if source.Host == "" || source.Database == "" || source.Username == "" {
		return Material{}, errors.New("postgres connection is incomplete")
	}
	dir, err := privateDirectory(root, "dbvault-pg-auth-")
	if err != nil {
		return Material{}, err
	}
	path := filepath.Join(dir, "pgpass")
	line := strings.Join([]string{escapePGPass(source.Host), fmt.Sprint(source.Port), escapePGPass(source.Database), escapePGPass(source.Username), escapePGPass(password)}, ":") + "\n"
	if err := os.WriteFile(path, []byte(line), 0600); err != nil {
		_ = os.RemoveAll(dir)
		return Material{}, err
	}
	env := []string{"PGHOST=" + source.Host, "PGPORT=" + fmt.Sprint(source.Port), "PGDATABASE=" + source.Database, "PGUSER=" + source.Username, "PGPASSFILE=" + path}
	if source.SSL.Mode != "" {
		env = append(env, "PGSSLMODE="+source.SSL.Mode)
	}
	if source.SSL.RootCert != "" {
		env = append(env, "PGSSLROOTCERT="+source.SSL.RootCert)
	}
	if source.SSL.ClientCert != "" {
		env = append(env, "PGSSLCERT="+source.SSL.ClientCert)
	}
	if source.SSL.ClientKey != "" {
		env = append(env, "PGSSLKEY="+source.SSL.ClientKey)
	}
	return Material{Environment: env, Directory: dir, Files: []string{path}}, nil
}

func PrepareMySQL(root string, source config.MySQLSourceConfig, password string) (Material, error) {
	if source.Host == "" || source.Database == "" || source.Username == "" {
		return Material{}, errors.New("mysql connection is incomplete")
	}
	dir, err := privateDirectory(root, "dbvault-mysql-auth-")
	if err != nil {
		return Material{}, err
	}
	path := filepath.Join(dir, "client.cnf")
	var b strings.Builder
	b.WriteString("[client]\n")
	fmt.Fprintf(&b, "host=%s\nport=%d\nuser=%s\npassword=%s\n", iniValue(source.Host), source.Port, iniValue(source.Username), iniValue(password))
	if source.TLS.Mode != "" {
		fmt.Fprintf(&b, "ssl-mode=%s\n", iniValue(source.TLS.Mode))
	}
	if source.TLS.CAFile != "" {
		fmt.Fprintf(&b, "ssl-ca=%s\n", iniValue(source.TLS.CAFile))
	}
	if source.TLS.CertFile != "" {
		fmt.Fprintf(&b, "ssl-cert=%s\n", iniValue(source.TLS.CertFile))
	}
	if source.TLS.KeyFile != "" {
		fmt.Fprintf(&b, "ssl-key=%s\n", iniValue(source.TLS.KeyFile))
	}
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		_ = os.RemoveAll(dir)
		return Material{}, err
	}
	return Material{Arguments: []string{"--defaults-extra-file=" + path}, Directory: dir, Files: []string{path}}, nil
}

func privateDirectory(root, pattern string) (string, error) {
	if root == "" {
		root = os.TempDir()
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, pattern)
	if err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func escapePGPass(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, ":", "\\:")
}
func iniValue(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\n", ""), "\r", "")
}
