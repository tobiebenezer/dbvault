package ports

import "time"

type Metrics interface {
	IncCounter(name string, labels map[string]string)
	ObserveDuration(name string, d time.Duration, labels map[string]string)
	SetGauge(name string, value float64, labels map[string]string)
}

type NoopMetrics struct{}

func (NoopMetrics) IncCounter(string, map[string]string)                     {}
func (NoopMetrics) ObserveDuration(string, time.Duration, map[string]string) {}
func (NoopMetrics) SetGauge(string, float64, map[string]string)              {}
