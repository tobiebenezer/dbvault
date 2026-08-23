package webhook

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/ports"
)

func testEvent() ports.Event {
	return ports.Event{ID: "job-1", Type: "backup", Severity: "info", Resource: "demo", CreatedAt: time.Now().UTC()}
}

func TestNotifySignsPayload(t *testing.T) {
	var gotSig string
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get(SignatureHeader)
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(srv.URL, "topsecret")
	if err := n.Notify(context.Background(), testEvent()); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if gotSig == "" || len(body) == 0 {
		t.Fatalf("missing signature %q or body", gotSig)
	}
	if !VerifySignature("topsecret", body, gotSig) {
		t.Fatal("signature did not verify over transmitted body")
	}
	if VerifySignature("wrongsecret", body, gotSig) {
		t.Fatal("signature verified with wrong secret")
	}
}

func TestNotifyRetriesUntilSuccess(t *testing.T) {
	var attempts int32
	var sleeps []time.Duration
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(srv.URL, "")
	n.Sleep = func(context.Context, time.Duration) error {
		sleeps = append(sleeps, 0)
		return nil
	}
	if err := n.Notify(context.Background(), testEvent()); err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts=%d want 3", attempts)
	}
	if len(sleeps) != 2 {
		t.Fatalf("sleeps=%d want 2 (no sleep before first attempt)", len(sleeps))
	}
}

func TestNotifyFailureIsNonFatal(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	n := New(srv.URL, "s")
	n.MaxAttempts = 3
	n.Sleep = func(context.Context, time.Duration) error { return nil }
	err := n.Notify(context.Background(), testEvent())
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	if attempts != 3 {
		t.Fatalf("attempts=%d want exactly MaxAttempts=3", attempts)
	}

	// A failing notifier must never take down its caller: invoking Notify on
	// an unreachable endpoint returns an error and nothing more.
	dead := New("http://127.0.0.1:1/unreachable", "")
	dead.MaxAttempts = 1
	if err := dead.Notify(context.Background(), testEvent()); err == nil {
		t.Fatal("expected error for unreachable endpoint")
	}
}

func TestNotifyEmptyURLIsNoop(t *testing.T) {
	n := New("", "s")
	if err := n.Notify(context.Background(), testEvent()); err != nil {
		t.Fatalf("empty URL should be a silent noop, got %v", err)
	}
}
