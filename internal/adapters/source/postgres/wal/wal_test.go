package wal

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
		"000000010000000000000001",
		"000000010000000000000001.partial",
		"00000002.history",
		"000000010000000000000001.00000028.backup",
		"FFFFFFFF0000000000000001",
	}
	for _, name := range validCases {
		if !ValidName(name) {
			t.Errorf("expected %q to be valid WAL filename", name)
		}
	}

	invalidCases := []string{
		"00000001",
		"000000010000000000000001.tmp",
		"00000001.backup",
		"../000000010000000000000001",
		"wal.000001",
		"",
	}
	for _, name := range invalidCases {
		if ValidName(name) {
			t.Errorf("expected %q to be invalid WAL filename", name)
		}
	}
}

func TestArchiveHelperPush(t *testing.T) {
	store := newMockObjectStore()
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	helper := ArchiveHelper{
		SourceID:     "pg_src_1",
		RepositoryID: "repo_1",
		Store:        store,
		Now:          func() time.Time { return now },
	}

	walContent := []byte("postgres-wal-segment-binary-data-payload-1234567890")
	tmpFile := filepath.Join(t.TempDir(), "000000010000000000000001")
	if err := os.WriteFile(tmpFile, walContent, 0600); err != nil {
		t.Fatal(err)
	}

	log, err := helper.Push(context.Background(), tmpFile, "000000010000000000000001")
	if err != nil {
		t.Fatal(err)
	}

	expectedHash := sha256.Sum256(walContent)
	expectedHex := hex.EncodeToString(expectedHash[:])

	if log.ID != "000000010000000000000001" {
		t.Fatalf("unexpected log ID: %s", log.ID)
	}
	if log.Kind != domain.LogPostgresWAL {
		t.Fatalf("unexpected log kind: %s", log.Kind)
	}
	if log.Checksum != expectedHex {
		t.Fatalf("expected checksum %s, got %s", expectedHex, log.Checksum)
	}
	if log.LogicalSize != int64(len(walContent)) {
		t.Fatalf("expected size %d, got %d", len(walContent), log.LogicalSize)
	}
	if log.ObjectKey != "pg_src_1/wal/000000010000000000000001" {
		t.Fatalf("unexpected object key: %s", log.ObjectKey)
	}
	if log.Status != domain.LogVerified {
		t.Fatalf("expected LogVerified, got %v", log.Status)
	}

	// Verify store write with IfNotExists
	if len(store.putRequests) != 1 {
		t.Fatalf("expected 1 put request, got %d", len(store.putRequests))
	}
	req := store.putRequests[0]
	if !req.IfNotExists || req.Key != "pg_src_1/wal/000000010000000000000001" {
		t.Fatalf("unexpected put request: %+v", req)
	}
}

func TestArchiveHelperPushInvalid(t *testing.T) {
	helper := ArchiveHelper{SourceID: "pg_src_1"}

	// Invalid name
	_, err := helper.Push(context.Background(), "/tmp/fake", "invalid-name")
	if err == nil {
		t.Fatal("expected error on invalid WAL filename")
	}

	// Missing file
	_, err = helper.Push(context.Background(), "/nonexistent/wal/000000010000000000000001", "000000010000000000000001")
	if err == nil {
		t.Fatal("expected error on missing source file")
	}
}
