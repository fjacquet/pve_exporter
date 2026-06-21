package pve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fjacquet/pve_exporter/internal/models"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// fakePVE returns canned responses for the endpoints the collector calls.
func fakePVE(t *testing.T) *httptest.Server {
	t.Helper()
	routes := map[string]string{
		"/api2/json/cluster/resources": `{"data":[
			{"id":"node/proxmox","type":"node","node":"proxmox","status":"online","cpu":0.98,"maxcpu":4,"mem":53907812352,"maxmem":67399618560,"disk":17571426304,"maxdisk":31044079616,"uptime":315069},
			{"id":"qemu/100","type":"qemu","node":"proxmox","name":"samplevm1","status":"running","cpu":0.105,"maxcpu":1,"mem":16573280275,"maxmem":17179869184,"disk":0,"maxdisk":68719476736,"uptime":315039,"netin":1529756162,"netout":7750708280,"diskread":7473739264,"diskwrite":150048127488,"template":0,"tags":"tag1;tag2","hastate":"started","lock":"backup","vmid":100},
			{"id":"storage/proxmox/local","type":"storage","node":"proxmox","storage":"local","status":"available","disk":17571426304,"maxdisk":31044079616,"shared":0,"plugintype":"dir","content":"iso,vztmpl"}
		]}`,
		"/api2/json/cluster/status": `{"data":[
			{"id":"cluster","type":"cluster","name":"pvec","nodes":1,"quorate":1,"version":2},
			{"id":"node/proxmox","type":"node","name":"proxmox","nodeid":1,"level":"c","online":1}
		]}`,
		"/api2/json/version":                              `{"data":{"release":"8.1","repoid":"abcdef","version":"8.1-4"}}`,
		"/api2/json/cluster/config/qdevice":               `{"data":{"model":"net","network":{"host":"10.0.0.1","algorithm":"ffsplit"}}}`,
		"/api2/json/cluster/backup-info/not-backed-up":    `{"data":[{"vmid":100,"type":"qemu","name":"samplevm1"}]}`,
		"/api2/json/nodes/proxmox/replication":            `{"data":[{"id":"1-0","type":"local","source":"proxmox","target":"proxmox2","guest":100}]}`,
		"/api2/json/nodes/proxmox/replication/1-0/status": `{"data":{"duration":7.73,"last_sync":1713382503,"last_try":1713382503,"next_sync":1713468900,"fail_count":0}}`,
		"/api2/json/nodes/proxmox/subscription":           `{"data":{"status":"active","level":"c","nextduedate":"2024-04-17"}}`,
		"/api2/json/nodes/proxmox/qemu":                   `{"data":[{"vmid":100}]}`,
		"/api2/json/nodes/proxmox/lxc":                    `{"data":[]}`,
		"/api2/json/nodes/proxmox/qemu/100/config":        `{"data":{"onboot":1}}`,
		"/api2/json/cluster/ha/status/current": `{"data":[
			{"id":"node/proxmox","type":"node","node":"proxmox","status":"online"},
			{"id":"service:vm:100","type":"service","node":"proxmox","state":"started","sid":"vm:100"}
		]}`,
	}
	mux := http.NewServeMux()
	for path, body := range routes {
		body := body
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})
	}
	return httptest.NewTLSServer(mux)
}

func testTarget(t *testing.T, srv *httptest.Server) Target {
	t.Helper()
	host := strings.TrimPrefix(srv.URL, "https://")
	cfg := models.ClusterConfig{
		Name:               "pve1",
		Host:               host,
		TokenID:            "exporter@pam!t",
		TokenSecret:        "secret",
		InsecureSkipVerify: true,
	}
	return Target{Cfg: cfg, Client: NewClient(cfg, false)}
}

func collectFixture(t *testing.T) (*SnapshotStore, *Snapshot) {
	t.Helper()
	srv := fakePVE(t)
	t.Cleanup(srv.Close)
	store := NewSnapshotStore()
	c := NewCollector([]Target{testTarget(t, srv)}, store, time.Minute, 10*time.Second, models.CollectorToggles{}, 0)
	snap := c.CollectOnce(context.Background())
	return store, snap
}

func TestCollectorSnapshot(t *testing.T) {
	_, snap := collectFixture(t)

	tgt := snap.Targets()["pve1"]
	if tgt == nil || !tgt.Up {
		t.Fatalf("target pve1 should be up: %+v", tgt)
	}

	// Spot-check a representative metric from each collector path.
	want := map[string]bool{
		metricUp: true, metricCPURatio: true, metricMemUsage: true,
		metricGuestInfo: true, metricStorageInfo: true, metricNodeInfo: true,
		metricClusterInfo: true, metricVersionInfo: true, metricNetRxTotal: true,
		metricHAState: true, metricLockState: true, metricQDeviceInfo: true,
		metricNotBackedUpTotal: true, metricReplInfo: true, metricSubStatus: true,
		metricOnboot: true, metricCollectionDuration: true, metricRequestErrors: true,
	}
	for name := range want {
		if len(snap.SamplesFor(name)) == 0 {
			t.Errorf("expected samples for %s, got none", name)
		}
	}
}

func TestAbsentNotZero(t *testing.T) {
	_, snap := collectFixture(t)
	// qemu/100 has disk=0 but is valid; the node has no network counters, so
	// pve_network_receive_bytes_total must exist ONLY for the guest, never the node.
	for _, s := range snap.SamplesFor(metricNetRxTotal) {
		for _, l := range s.Labels {
			if l.Name == "id" && strings.HasPrefix(l.Value, "node/") {
				t.Errorf("node should not emit %s (absent-not-zero)", metricNetRxTotal)
			}
		}
	}
}

func TestStateEnumStableLabels(t *testing.T) {
	_, snap := collectFixture(t)
	all := snap.SamplesFor(metricHAState)

	// Count only guest HA samples (id prefix qemu/ or lxc/).
	var guestSamples []Sample
	for _, s := range all {
		for _, l := range s.Labels {
			if l.Name == "id" && (strings.HasPrefix(l.Value, "qemu/") || strings.HasPrefix(l.Value, "lxc/")) {
				guestSamples = append(guestSamples, s)
			}
		}
	}
	var active int
	for _, s := range guestSamples {
		if s.Value == 1 {
			active++
		}
	}
	if len(guestSamples) != len(haGuestStates) {
		t.Errorf("guest ha_state should emit all %d states, got %d", len(haGuestStates), len(guestSamples))
	}
	if active != 1 {
		t.Errorf("exactly one guest ha_state should be active, got %d", active)
	}
}

func TestPrometheusExport(t *testing.T) {
	store, _ := collectFixture(t)
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	byName := make(map[string]*dto.MetricFamily, len(families))
	for _, f := range families {
		byName[f.GetName()] = f
	}

	if f, ok := byName[metricUp]; !ok || len(f.Metric) == 0 {
		t.Fatalf("%s missing from Prometheus output", metricUp)
	}
	// Counters must be typed as counters.
	if f, ok := byName[metricNetRxTotal]; ok {
		if f.GetType() != dto.MetricType_COUNTER {
			t.Errorf("%s should be a counter, got %s", metricNetRxTotal, f.GetType())
		}
	} else {
		t.Errorf("%s missing", metricNetRxTotal)
	}
	// Every series carries the cluster identity label.
	f := byName[metricUp]
	for _, m := range f.Metric {
		if !hasLabel(m, "cluster", "pve1") {
			t.Errorf("%s series missing cluster label: %v", metricUp, m.Label)
		}
	}
}

func TestOTLPExport(t *testing.T) {
	store, _ := collectFixture(t)
	reader := sdkmetric.NewManualReader()
	exp, err := newOTLPExporterWithReader(context.Background(), reader, store, "test")
	if err != nil {
		t.Fatalf("otlp exporter: %v", err)
	}
	if err := exp.EnsureInstruments(); err != nil {
		t.Fatalf("ensure instruments: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	seen := map[string]bool{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			seen[m.Name] = true
		}
	}
	for _, name := range []string{metricUp, metricGuestInfo, metricCPURatio, metricNetRxTotal} {
		if !seen[name] {
			t.Errorf("OTLP output missing %s", name)
		}
	}
}

func hasLabel(m *dto.Metric, name, value string) bool {
	for _, l := range m.Label {
		if l.GetName() == name && l.GetValue() == value {
			return true
		}
	}
	return false
}

func TestCollectionDurationMetric(t *testing.T) {
	_, snap := collectFixture(t)
	samples := snap.SamplesFor(metricCollectionDuration)
	if len(samples) == 0 {
		t.Fatalf("expected %s samples, got none", metricCollectionDuration)
	}
	for _, s := range samples {
		if s.Value < 0 {
			t.Errorf("%s value must be non-negative, got %f", metricCollectionDuration, s.Value)
		}
	}
}

func TestRequestErrorsMetric(t *testing.T) {
	_, snap := collectFixture(t)
	samples := snap.SamplesFor(metricRequestErrors)
	if len(samples) == 0 {
		t.Fatalf("expected %s samples, got none", metricRequestErrors)
	}
}

func TestNodeHAState(t *testing.T) {
	_, snap := collectFixture(t)
	all := snap.SamplesFor(metricHAState)

	var nodeSamples []Sample
	for _, s := range all {
		for _, l := range s.Labels {
			if l.Name == "id" && strings.HasPrefix(l.Value, "node/") {
				nodeSamples = append(nodeSamples, s)
			}
		}
	}
	if len(nodeSamples) != len(haNodeStates) {
		t.Errorf("node ha_state should emit all %d states, got %d", len(haNodeStates), len(nodeSamples))
	}
	var active int
	for _, s := range nodeSamples {
		if s.Value == 1 {
			active++
		}
	}
	if active != 1 {
		t.Errorf("exactly one node ha_state should be active, got %d", active)
	}
}

func TestRequestErrorsIncrement(t *testing.T) {
	// Point client at a server that always returns 500.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "https://")
	cfg := models.ClusterConfig{
		Name:               "err-test",
		Host:               host,
		TokenID:            "exporter@pam!t",
		TokenSecret:        "secret",
		InsecureSkipVerify: true,
	}
	client := NewClient(cfg, false)

	before := client.RequestErrors()
	_ = client.Get(context.Background(), "/api2/json/version", nil)
	after := client.RequestErrors()

	if after <= before {
		t.Errorf("RequestErrors should have incremented: before=%d after=%d", before, after)
	}
}
