package pve

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// PromCollector is an unchecked Prometheus collector: it emits a dynamic set of
// metric names read from the latest snapshot, so Describe intentionally sends
// nothing and metric Descs are built on the fly in Collect.
type PromCollector struct {
	store *SnapshotStore
}

// NewPromCollector returns a collector backed by store.
func NewPromCollector(store *SnapshotStore) *PromCollector {
	return &PromCollector{store: store}
}

// Describe is a no-op (unchecked collector) so dynamic names are allowed.
func (c *PromCollector) Describe(_ chan<- *prometheus.Desc) {}

// Collect emits every sample in the current snapshot.
func (c *PromCollector) Collect(ch chan<- prometheus.Metric) {
	snap := c.store.Load()
	for _, name := range snap.MetricNames() {
		samples := snap.SamplesFor(name)
		seen := make(map[string]struct{}, len(samples))
		for _, s := range samples {
			keys, values := splitLabels(s.Labels)
			sig := strings.Join(values, "\x00")
			if _, dup := seen[sig]; dup {
				continue // guard against duplicate series within one cycle
			}
			seen[sig] = struct{}{}
			valueType := prometheus.GaugeValue
			if isCounter(s.Name) {
				valueType = prometheus.CounterValue
			}
			desc := prometheus.NewDesc(s.Name, helpFor(s.Name), keys, nil)
			m, err := prometheus.NewConstMetric(desc, valueType, s.Value, values...)
			if err != nil {
				continue
			}
			ch <- m
		}
	}
}

// splitLabels separates an ordered label slice into parallel key/value slices.
func splitLabels(labels []Label) (keys, values []string) {
	keys = make([]string, len(labels))
	values = make([]string, len(labels))
	for i, l := range labels {
		keys[i] = l.Name
		values[i] = l.Value
	}
	return keys, values
}
