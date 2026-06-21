package pve

// Metric name constants — kept identical to the de-facto community
// prometheus-pve-exporter so existing dashboards and alerts keep working.
const (
	metricUp        = "pve_up"
	metricDiskSize  = "pve_disk_size_bytes"
	metricDiskUsage = "pve_disk_usage_bytes"
	metricMemSize   = "pve_memory_size_bytes"
	metricMemUsage  = "pve_memory_usage_bytes"
	metricCPURatio  = "pve_cpu_usage_ratio"
	metricCPULimit  = "pve_cpu_usage_limit"
	metricUptime    = "pve_uptime_seconds"
	metricStorShare = "pve_storage_shared"
	metricHAState   = "pve_ha_state"
	metricLockState = "pve_lock_state"

	metricNetRxTotal     = "pve_network_receive_bytes_total"
	metricNetTxTotal     = "pve_network_transmit_bytes_total"
	metricDiskReadTotal  = "pve_disk_read_bytes_total"
	metricDiskWriteTotal = "pve_disk_written_bytes_total"

	metricGuestInfo   = "pve_guest_info"
	metricStorageInfo = "pve_storage_info"
	metricNodeInfo    = "pve_node_info"
	metricClusterInfo = "pve_cluster_info"
	metricVersionInfo = "pve_version_info"

	metricQDeviceUp   = "pve_qdevice_up"
	metricQDeviceInfo = "pve_qdevice_info"

	metricSubInfo    = "pve_subscription_info"
	metricSubStatus  = "pve_subscription_status"
	metricSubNextDue = "pve_subscription_next_due_timestamp_seconds"
	metricOnboot     = "pve_onboot_status"

	metricNotBackedUpTotal = "pve_not_backed_up_total"
	metricNotBackedUpInfo  = "pve_not_backed_up_info"

	metricReplInfo     = "pve_replication_info"
	metricReplDuration = "pve_replication_duration_seconds"
	metricReplLastSync = "pve_replication_last_sync_timestamp_seconds"
	metricReplLastTry  = "pve_replication_last_try_timestamp_seconds"
	metricReplNextSync = "pve_replication_next_sync_timestamp_seconds"
	metricReplFailed   = "pve_replication_failed_syncs"

	metricCollectionDuration = "pve_collection_duration_seconds"
	metricRequestErrors      = "pve_request_errors_total"
)

// haNodeStates enumerates the HA states reported for nodes via /cluster/ha/status/current.
var haNodeStates = []string{"online", "maintenance", "unknown", "fence", "gone"}

// haGuestStates enumerates the HA states reported for guests; every series is
// emitted (1 for the active state, 0 otherwise) so the label-key set is stable.
var haGuestStates = []string{
	"stopped", "request_stop", "request_start", "request_start_balance",
	"started", "fence", "recovery", "migrate", "relocate", "freeze", "error",
}

// lockStates enumerates the guest config lock states.
var lockStates = []string{
	"backup", "clone", "create", "migrate", "rollback",
	"snapshot", "snapshot-delete", "suspended", "suspending",
}

// subStates enumerates the subscription status values.
var subStates = []string{"new", "notfound", "active", "invalid", "expired", "suspended"}

// Label is a single name/value pair on a sample.
type Label struct {
	Name  string
	Value string
}

// Sample is one metric data point: a name, an ordered label set (the first
// label is always "cluster"), and a float value.
type Sample struct {
	Name   string
	Labels []Label
	Value  float64
}

// sampleSet accumulates samples for one target, always prepending the cluster
// identity label so every series carries it in a stable position.
type sampleSet struct {
	cluster string
	out     []Sample
}

func newSampleSet(cluster string) *sampleSet {
	return &sampleSet{cluster: cluster}
}

// add appends one sample with the cluster label prepended to labels.
func (s *sampleSet) add(name string, value float64, labels ...Label) {
	full := make([]Label, 0, len(labels)+1)
	full = append(full, Label{Name: "cluster", Value: s.cluster})
	full = append(full, labels...)
	s.out = append(s.out, Sample{Name: name, Labels: full, Value: value})
}

// id builds the canonical "type/identifier" id label used by the PVE exporter.
func idLabel(value string) Label { return Label{Name: "id", Value: value} }
