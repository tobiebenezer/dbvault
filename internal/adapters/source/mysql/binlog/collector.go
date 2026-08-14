package binlog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

var binlogNameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.[0-9]{6}$`)

func ValidName(name string) bool { return binlogNameRE.MatchString(name) }

type FileCollector struct { SourceID domain.SourceID; RepositoryID domain.RepositoryID; Engine domain.DatabaseEngine; ServerID string; Store ports.ObjectStore; Now func() time.Time }

func (c FileCollector) Push(ctx context.Context, filePath, name string) (domain.TransactionLog, error) {
	if !ValidName(name) { return domain.TransactionLog{}, domain.NewError(domain.ErrBinlogInvalid, "invalid binlog filename", nil) }
	f, err := os.Open(filePath); if err != nil { return domain.TransactionLog{}, err }; defer f.Close()
	h:=sha256.New(); n, err := io.Copy(h, f); if err != nil { return domain.TransactionLog{}, err }
	_, _ = f.Seek(0,0); kind:=domain.LogMySQLBinlog; if c.Engine==domain.EngineMariaDB { kind=domain.LogMariaDBBinlog }
	key := "logs/mysql/"+string(c.SourceID)+"/"+c.ServerID+"/"+name
	if c.Store != nil { _, err = c.Store.Put(ctx, ports.PutObjectRequest{Key:key, Body:f, Size:n, ContentType:"application/octet-stream", IfNotExists:true}); if err != nil { return domain.TransactionLog{}, err } }
	now:=time.Now(); if c.Now!=nil { now=c.Now() }
	return domain.TransactionLog{ID:domain.TransactionLogID(name), SourceID:c.SourceID, RepositoryID:c.RepositoryID, Kind:kind, NativeName:name, LogicalSize:n, StoredSize:n, Checksum:hex.EncodeToString(h.Sum(nil)), ObjectKey:key, Status:domain.LogVerified, CollectedAt:now, VerifiedAt:&now}, nil
}

type Checkpoint struct { SourceID domain.SourceID; ServerID string; File string; Position uint64; GTIDSet string; UpdatedAt time.Time }
