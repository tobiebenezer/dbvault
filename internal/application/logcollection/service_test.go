package logcollection

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestLogCollectionIngestAndTimeline(t *testing.T) {
	svc := New(nil, nil)
	ctx := context.Background()

	data1 := bytes.NewReader([]byte("dummy wal chunk segment 000000010000000000000001"))
	meta1, err := svc.Ingest(ctx, "pg-prod", "000000010000000000000001", data1)
	if err != nil {
		t.Fatalf("expected ingest to succeed: %v", err)
	}

	if meta1.SHA256Hash == "" || meta1.ByteSize <= 0 {
		t.Fatalf("expected valid metadata, got %+v", meta1)
	}

	data2 := bytes.NewReader([]byte("dummy wal chunk segment 000000010000000000000002"))
	_, err = svc.Ingest(ctx, "pg-prod", "000000010000000000000002", data2)
	if err != nil {
		t.Fatalf("expected second ingest to succeed: %v", err)
	}

	chunks := svc.ListChunks("pg-prod")
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}

	continuous, totalBytes, count := svc.ContinuousTimelineStatus("pg-prod")
	if !continuous || count != 2 || totalBytes <= 0 {
		t.Fatalf("expected continuous timeline, got continuous=%v, count=%d, bytes=%d", continuous, count, totalBytes)
	}
}

func TestLogCollectionEmptyChunk(t *testing.T) {
	svc := New(nil, nil)
	ctx := context.Background()

	empty := strings.NewReader("")
	_, err := svc.Ingest(ctx, "pg-prod", "000000010000000000000001", empty)
	if err != ErrEmptyChunkStream {
		t.Fatalf("expected ErrEmptyChunkStream, got %v", err)
	}
}
