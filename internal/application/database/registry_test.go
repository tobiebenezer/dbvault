package database

import (
	"context"
	"testing"

	"github.com/dbvault/dbvault/internal/adapters/source/mysql"
	"github.com/dbvault/dbvault/internal/adapters/source/postgres"
	"github.com/dbvault/dbvault/internal/domain"
)

func TestRegistryListsPhase4Drivers(t *testing.T) {
	r := NewRegistry(postgres.New(nil, postgres.DefaultConfig()), mysql.New(nil, mysql.DefaultConfig("mysql")), mysql.New(nil, mysql.DefaultConfig("mariadb")))
	if _, ok := r.Get(domain.EnginePostgres); !ok {
		t.Fatal("postgres driver missing")
	}
	if _, ok := r.Get(domain.EngineMySQL); !ok {
		t.Fatal("mysql driver missing")
	}
	if _, ok := r.Get(domain.EngineMariaDB); !ok {
		t.Fatal("mariadb driver missing")
	}
	if got := len(r.List()); got != 3 {
		t.Fatalf("expected 3 drivers, got %d", got)
	}
}

func TestDriverDescriptors(t *testing.T) {
	pg := postgres.New(nil, postgres.DefaultConfig()).Descriptor()
	if pg.API != domain.DatabaseDriverAPIV1 || !pg.SupportsGlobals || !pg.SupportsParallel {
		t.Fatalf("unexpected postgres descriptor: %+v", pg)
	}
	my := mysql.New(nil, mysql.DefaultConfig("mysql")).Descriptor()
	if my.Engine != domain.EngineMySQL || !my.SupportsStreaming {
		t.Fatalf("unexpected mysql descriptor: %+v", my)
	}
	ma := mysql.New(nil, mysql.DefaultConfig("mariadb")).Descriptor()
	if ma.Engine != domain.EngineMariaDB {
		t.Fatalf("unexpected mariadb descriptor: %+v", ma)
	}
}

func TestDriversValidateMissingRunner(t *testing.T) {
	cfg := postgres.DefaultConfig()
	cfg.Host = "127.0.0.1"
	cfg.Database = "app"
	cfg.Username = "dbvault"
	if err := postgres.New(nil, cfg).ValidateSource(context.Background(), domain.Source{}); err == nil {
		t.Fatal("expected missing runner error")
	}
}
