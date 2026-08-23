// Package prometheus renders dbvault metrics in the Prometheus text
// exposition format (version 0.0.4) using only the standard library, so the
// restricted build stays dependency-free.
package prometheus

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const contentType = "text/plain; version=0.0.4; charset=utf-8"

// Default histogram buckets cover sub-second page ops through long backups.
var defaultBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300}

// help texts for the metric families dbvault guarantees on /metrics.
var familyHelp = map[string]string{
	"dbvault_jobs_total":                           "Jobs settled by type and terminal status.",
	"dbvault_bytes_backed_up_total":                "Source bytes captured by successful backups.",
	"dbvault_job_duration_seconds":                 "Job execution duration in seconds.",
	"dbvault_queue_depth":                          "Live (non-terminal) jobs currently in the queue.",
	"dbvault_lease_expired_events_total":           "Job leases recovered after worker loss.",
	"dbvault_notification_delivery_failures_total": "Webhook notifications that exhausted retries.",
}

type labelSet map[string]string

func (l labelSet) key() string {
	if len(l) == 0 {
		return ""
	}
	parts := make([]string, 0, len(l))
	for k, v := range l {
		parts = append(parts, fmt.Sprintf("%s=%q", k, v))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (l labelSet) render() string {
	if len(l) == 0 {
		return ""
	}
	parts := make([]string, 0, len(l))
	for k, v := range l {
		parts = append(parts, fmt.Sprintf("%s=%q", k, v))
	}
	sort.Strings(parts)
	return "{" + strings.Join(parts, ",") + "}"
}

type series struct {
	labels string
	value  float64
}

type histogramSeries struct {
	labels  string
	buckets []float64 // cumulative counts, parallel to defaultBuckets
	sum     float64
	count   uint64
}

// Metrics implements ports.Metrics with hand-rolled text exposition.
// All methods are safe for concurrent use.
type Metrics struct {
	mu         sync.Mutex
	counters   map[string]map[string]float64 // family -> labelkey -> value
	gauges     map[string]map[string]float64
	labelTexts map[string]map[string]string // family -> labelkey -> rendered labels
	histograms map[string]map[string]*histogramSeries
}

func New() *Metrics {
	return &Metrics{
		counters:   map[string]map[string]float64{},
		gauges:     map[string]map[string]float64{},
		labelTexts: map[string]map[string]string{},
		histograms: map[string]map[string]*histogramSeries{},
	}
}

// IncCounter adds one to the counter identified by name+labels.
func (m *Metrics) IncCounter(name string, labels map[string]string) { m.AddCounter(name, labels, 1) }

// AddCounter adds delta to the counter identified by name+labels.
func (m *Metrics) AddCounter(name string, labels map[string]string, delta float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fam, ok := m.counters[name]
	if !ok {
		fam = map[string]float64{}
		m.counters[name] = fam
	}
	k := labelSet(labels).key()
	m.rememberLabels(name, k, labels)
	fam[k] += delta
}

// SetGauge records the latest value of a gauge series.
func (m *Metrics) SetGauge(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fam, ok := m.gauges[name]
	if !ok {
		fam = map[string]float64{}
		m.gauges[name] = fam
	}
	k := labelSet(labels).key()
	m.rememberLabels(name, k, labels)
	fam[k] = value
}

// ObserveDuration records one duration observation in a cumulative histogram.
func (m *Metrics) ObserveDuration(name string, d time.Duration, labels map[string]string) {
	v := d.Seconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	fam, ok := m.histograms[name]
	if !ok {
		fam = map[string]*histogramSeries{}
		m.histograms[name] = fam
	}
	k := labelSet(labels).key()
	s, ok := fam[k]
	if !ok {
		s = &histogramSeries{buckets: make([]float64, len(defaultBuckets))}
		fam[k] = s
	}
	m.rememberLabels(name, k, labels)
	for i, bound := range defaultBuckets {
		if v <= bound {
			s.buckets[i]++
		}
	}
	s.sum += v
	s.count++
}

func (m *Metrics) rememberLabels(family, key string, labels map[string]string) {
	lf, ok := m.labelTexts[family]
	if !ok {
		lf = map[string]string{}
		m.labelTexts[family] = lf
	}
	lf[key] = labelSet(labels).render()
}

// Render produces the full exposition document. Families are emitted in
// sorted order; HELP/TYPE appear exactly once per family; series are sorted
// by label text so output is deterministic for snapshots.
func (m *Metrics) Render() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	type entry struct {
		name  string
		lines []string
	}
	entries := map[string][]string{}
	order := []string{}

	emit := func(family string, lines []string) {
		if _, ok := entries[family]; !ok {
			order = append(order, family)
		}
		entries[family] = append(entries[family], lines...)
	}

	for _, name := range sortedKeys(m.counters) {
		fam := m.counters[name]
		lines := []string{head(name, "counter")}
		for _, k := range sortedKeys(fam) {
			lines = append(lines, fmt.Sprintf("%s%s %v", name, m.labelTexts[name][k], formatValue(fam[k])))
		}
		emit(name, lines)
	}

	for _, name := range sortedKeys(m.gauges) {
		fam := m.gauges[name]
		lines := []string{head(name, "gauge")}
		for _, k := range sortedKeys(fam) {
			lines = append(lines, fmt.Sprintf("%s%s %v", name, m.labelTexts[name][k], formatValue(fam[k])))
		}
		emit(name, lines)
	}

	for _, name := range sortedKeys(m.histograms) {
		fam := m.histograms[name]
		lines := []string{head(name, "histogram")}
		keys := make([]string, 0, len(fam))
		for k := range fam {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			s := fam[k]
			base := m.labelTexts[name][k]
			for i, bound := range defaultBuckets {
				lbl := joinLabel(base, fmt.Sprintf("le=%q", formatValue(bound)))
				lines = append(lines, fmt.Sprintf("%s_bucket%s %v", name, lbl, formatValue(s.buckets[i])))
			}
			lines = append(lines, fmt.Sprintf("%s_bucket%s %v", name, joinLabel(base, `le="+Inf"`), formatValue(float64(s.count))))
			lines = append(lines, fmt.Sprintf("%s_sum%s %v", name, base, formatValue(s.sum)))
			lines = append(lines, fmt.Sprintf("%s_count%s %v", name, base, formatValue(float64(s.count))))
		}
		emit(name, lines)
	}

	sort.Strings(order)
	var b strings.Builder
	for _, family := range order {
		b.WriteString(strings.Join(entries[family], "\n"))
		b.WriteString("\n")
	}
	return b.String()
}

// Handler serves the exposition document over HTTP.
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write([]byte(m.Render()))
	})
}

func head(name, kind string) string {
	var h string
	if help, ok := familyHelp[name]; ok {
		h = fmt.Sprintf("# HELP %s %s\n", name, help)
	}
	return h + fmt.Sprintf("# TYPE %s %s", name, kind)
}

func joinLabel(base, extra string) string {
	if base == "" {
		return "{" + extra + "}"
	}
	return base[:len(base)-1] + "," + extra + "}"
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatValue(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
}
