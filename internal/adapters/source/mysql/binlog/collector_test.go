package binlog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type mockObjectStore struct {
	putRequests []ports.PutObjectRequest
	objects     map[string][]byte
}

func newMockObjectStore() *mockObjectStore {
	return &mockObjectStore{objects: make(map[string][]byte)}
}

func (m *mockObjectStore) Name() string                       { return "mock" }
func (m *mockObjectStore) Validate(ctx context.Context) error { return nil }
func (m *mockObjectStore) Capabilities(ctx context.Context) (ports.StorageCapabilities, error) {
	return ports.StorageCapabilities{}, nil
}

func (m *mockObjectStore) Put(ctx context.Context, req ports.PutObjectRequest) (ports.StoredObject, error) {
	m.putRequests = append(m.putRequests, req)
	b, err := io.ReadAll(req.Body)
	if err != nil {
		return ports.StoredObject{}, err
	}
	m.objects[req.Key] = b
	return ports.StoredObject{
		Key:  req.Key,
		Size: int64(len(b)),
		ETag: "dummy-etag",
	}, nil
}

func (m *mockObjectStore) Get(ctx context.Context, req ports.GetObjectRequest) (ports.ObjectReader, ports.StoredObject, error) {
	b, ok := m.objects[req.Key]
	if !ok {
		return nil, ports.StoredObject{}, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(b)), ports.StoredObject{Key: req.Key, Size: int64(len(b))}, nil
}

func (m *mockObjectStore) Head(ctx context.Context, key string) (ports.StoredObject, error) {
	b, ok := m.objects[key]
	if !ok {
		return ports.StoredObject{}, os.ErrNotExist
	}
	return ports.StoredObject{Key: key, Size: int64(len(b))}, nil
}

func (m *mockObjectStore) Delete(ctx context.Context, key string) error {
	delete(m.objects, key)
	return nil
}

func (m *mockObjectStore) List(ctx context.Context, req ports.ListObjectsRequest) (ports.ListObjectsResult, error) {
	var objects []ports.StoredObject
	for k, v := range m.objects {
		objects = append(objects, ports.StoredObject{Key: k, Size: int64(len(v))})
	}
	return ports.ListObjectsResult{Objects: objects}, nil
}

func TestValidName(t *testing.T) {
	validCases := []string{
		"mysql-bin.000001",
		"binlog.000123",
		"mysqld-bin.999999",
		"server-1.bin.000042",
		"mariadb-bin.000001",
	}
	for _, name := range validCases {
		if !ValidName(name) {
			t.Errorf("expected %q to be valid binlog name", name)
		}
	}

	invalidCases := []string{
		"binlog.1",
		"binlog.index",
		"../binlog.000001",
		"mysql-bin.0000001",
		"mysql-bin",
		"",
	}
	for _, name := range invalidCases {
		if ValidName(name) {
			t.Errorf("expected %q to be invalid binlog name", name)
		}
	}
}

func TestFileCollectorPushMySQL(t *testing.T) {
	store := newMockObjectStore()
	now := time.Date(2026, 8, 30, 14, 0, 0, 0, time.UTC)
	collector := FileCollector{
		SourceID:     "mysql_src_1",
		RepositoryID: "repo_1",
		Engine:       domain.EngineMySQL,
		ServerID:     "srv_10",
		Store:        store,
		Now:          func() time.Time { return now },
	}

	content := []byte("\xfe\x62\x69\x6ebinlog-payload-events-12345")
	tmpFile := filepath.Join(t.TempDir(), "mysql-bin.000001")
	if err := os.WriteFile(tmpFile, content, 0600); err != nil {
		t.Fatal(err)
	}

	log, err := collector.Push(context.Background(), tmpFile, "mysql-bin.000001")
	if err != nil {
		t.Fatal(err)
	}

	expectedHash := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(expectedHash[:])

	if log.ID != "mysql-bin.000001" {
		t.Fatalf("unexpected log ID: %s", log.ID)
	}
	if log.Kind != domain.LogMySQLBinlog {
		t.Fatalf("expected LogMySQLBinlog, got %v", log.Kind)
	}
	if log.Checksum != expectedHex {
		t.Fatalf("expected checksum %s, got %s", expectedHex, log.Checksum)
	}
	if log.LogicalSize != int64(len(content)) {
		t.Fatalf("expected size %d, got %d", len(content), log.LogicalSize)
	}
	if log.ObjectKey != "mysql_src_1/binlog/srv_10/mysql-bin.000001" {
		t.Fatalf("unexpected object key: %s", log.ObjectKey)
	}
	if log.Status != domain.LogVerified {
		t.Fatalf("expected LogVerified, got %v", log.Status)
	}

	if len(store.putRequests) != 1 {
		t.Fatalf("expected 1 put request, got %d", len(store.putRequests))
	}
	req := store.putRequests[0]
	if !req.IfNotExists || req.Key != "mysql_src_1/binlog/srv_10/mysql-bin.000001" {
		t.Fatalf("unexpected put request: %+v", req)
	}
}

func TestFileCollectorPushMariaDB(t *testing.T) {
	store := newMockObjectStore()
	collector := FileCollector{
		SourceID:     "maria_src_1",
		RepositoryID: "repo_1",
		Engine:       domain.EngineMariaDB,
		ServerID:     "srv_20",
		Store:        store,
	}

	content := []byte("\xfe\x62\x69\x6emariadb-binlog-content")
	tmpFile := filepath.Join(t.TempDir(), "mariadb-bin.000001")
	if err := os.WriteFile(tmpFile, content, 0600); err != nil {
		t.Fatal(err)
	}

	log, err := collector.Push(context.Background(), tmpFile, "mariadb-bin.000001")
	if err != nil {
		t.Fatal(err)
	}
	if log.Kind != domain.LogMariaDBBinlog {
		t.Fatalf("expected LogMariaDBBinlog, got %v", log.Kind)
	}
	if log.ObjectKey != "maria_src_1/binlog/srv_20/mariadb-bin.000001" {
		t.Fatalf("unexpected object key: %s", log.ObjectKey)
	}
}

func TestFileCollectorPushInvalid(t *testing.T) {
	collector := FileCollector{SourceID: "src_1"}

	// Invalid name
	_, err := collector.Push(context.Background(), "/tmp/fake", "invalid-name")
	if err == nil {
		t.Fatal("expected error on invalid binlog filename")
	}

	// Missing file
	_, err = collector.Push(context.Background(), "/nonexistent/mysql-bin.000001", "mysql-bin.000001")
	if err == nil {
		t.Fatal("expected error on missing source file")
	}
}
