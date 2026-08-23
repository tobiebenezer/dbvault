package prometheus

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

func countOccurrences(s, sub string) int {
	return strings.Count(s, sub)
}

func TestRenderSnapshotFamilies(t *testing.T) {
	m := New()
	m.IncCounter("dbvault_jobs_total", map[string]string{"type": "backup", "status": "succeeded"})
	m.IncCounter("dbvault_jobs_total", map[string]string{"type": "backup", "status": "succeeded"})
	m.IncCounter("dbvault_jobs_total", map[string]string{"type": "backup", "status": "failed"})
	m.AddCounter("dbvault_bytes_backed_up_total", map[string]string{"source": "demo"}, 2048)
	m.SetGauge("dbvault_queue_depth", 3, nil)
	m.ObserveDuration("dbvault_job_duration_seconds", 250*time.Millisecond, map[string]string{"type": "backup"})
	m.IncCounter("dbvault_lease_expired_events_total", nil)
	m.AddCounter("dbvault_lease_expired_events_total", nil, 1)
	m.AddCounter("dbvault_notification_delivery_failures_total", nil, 1)

	out := m.Render()

	for _, family := range []string{
		"dbvault_jobs_total",
		"dbvault_bytes_backed_up_total",
		"dbvault_job_duration_seconds",
		"dbvault_queue_depth",
		"dbvault_lease_expired_events_total",
		"dbvault_notification_delivery_failures_total",
	} {
		if n := countOccurrences(out, "# TYPE "+family+" "); n != 1 {
			t.Errorf("%s: TYPE header appears %d times, want exactly 1\n%s", family, n, out)
		}
		if !contains(out, "# HELP "+family+" ") {
			t.Errorf("%s: missing HELP header", family)
		}
	}
	if !contains(out, `dbvault_jobs_total{status="failed",type="backup"} 1`) {
		t.Errorf("jobs counter by status missing:\n%s", out)
	}
	if !contains(out, `dbvault_bytes_backed_up_total{source="demo"} 2048`) {
		t.Errorf("bytes counter missing:\n%s", out)
	}
	if !contains(out, "dbvault_queue_depth 3") {
		t.Errorf("queue depth gauge missing:\n%s", out)
	}
	// Histogram: buckets cumulative + +Inf + sum + count; le sorts last.
	if !contains(out, `dbvault_job_duration_seconds_bucket{type="backup",le="0.5"} 1`) {
		t.Errorf("histogram bucket line missing:\n%s", out)
	}
	if !contains(out, `dbvault_job_duration_seconds_bucket{type="backup",le="+Inf"} 1`) {
		t.Errorf("histogram +Inf bucket missing:\n%s", out)
	}
	if !contains(out, `dbvault_job_duration_seconds_count{type="backup"} 1`) {
		t.Errorf("histogram count missing:\n%s", out)
	}
}

func TestRenderDeterministicAndEscaped(t *testing.T) {
	m := New()
	m.SetGauge("g", 1, map[string]string{"z": "a\"b"})
	m.SetGauge("g", 2, map[string]string{"a": "c\\d"})
	first := m.Render()
	second := m.Render()
	if first != second {
		t.Fatal("render is not deterministic")
	}
	if !contains(first, `a="c\\d"`) || !contains(first, `z="a\"b"`) {
		t.Errorf("label escaping wrong:\n%s", first)
	}
	// Gauge overwrite replaces rather than duplicates the series.
	m.SetGauge("g", 9, map[string]string{"z": "a\"b"})
	third := m.Render()
	if n := countOccurrences(third, `z="a\"b"`); n != 1 {
		t.Fatalf("gauge series duplicated: %d occurrences\n%s", n, third)
	}
}

func TestContentTypeHeader(t *testing.T) {
	m := New()
	h := m.Handler()
	rec := &rw{}
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/metrics", nil)
	h.ServeHTTP(rec, req)
	if ct := rec.header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content type %q", ct)
	}
}

type rw struct {
	header http.Header
	code   int
}

func (w *rw) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}
func (w *rw) Write(b []byte) (int, error) { return len(b), nil }
func (w *rw) WriteHeader(code int)        { w.code = code }
