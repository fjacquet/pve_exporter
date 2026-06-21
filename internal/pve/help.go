package pve

import "strings"

// metricHelp maps metric names to HELP text, kept verbatim from the community
// prometheus-pve-exporter so scrape output is drop-in compatible.
var metricHelp = map[string]string{
	metricUp:                 "Node/VM/CT-Status is online/running",
	metricDiskSize:           "Storage size in bytes (for type 'storage'), root image size for VMs (for types 'qemu' and 'lxc').",
	metricDiskUsage:          "Used disk space in bytes (for type 'storage'), used root image space for VMs (for types 'qemu' and 'lxc').",
	metricMemSize:            "Number of available memory in bytes (for types 'node', 'qemu' and 'lxc').",
	metricMemUsage:           "Used memory in bytes (for types 'node', 'qemu' and 'lxc').",
	metricCPURatio:           "CPU utilization (for types 'node', 'qemu' and 'lxc').",
	metricCPULimit:           "Number of available CPUs (for types 'node', 'qemu' and 'lxc').",
	metricUptime:             "Uptime of node or virtual guest in seconds (for types 'node', 'qemu' and 'lxc').",
	metricStorShare:          "Whether or not the storage is shared among cluster nodes",
	metricHAState:            "HA service status (for HA managed VMs).",
	metricLockState:          "The guest's current config lock (for types 'qemu' and 'lxc')",
	metricNetRxTotal:         "The amount of traffic in bytes that was sent to the guest over the network since it was started. (for types 'qemu' and 'lxc')",
	metricNetTxTotal:         "The amount of traffic in bytes that was sent from the guest over the network since it was started. (for types 'qemu' and 'lxc')",
	metricDiskReadTotal:      "The amount of bytes the guest read from its block devices since the guest was started. (for types 'qemu' and 'lxc')",
	metricDiskWriteTotal:     "The amount of bytes the guest wrote to its block devices since the guest was started. (for types 'qemu' and 'lxc')",
	metricGuestInfo:          "VM/CT info",
	metricStorageInfo:        "Storage info",
	metricNodeInfo:           "Node info",
	metricClusterInfo:        "Cluster info",
	metricVersionInfo:        "Proxmox VE version info",
	metricQDeviceUp:          "Proxmox VE QDevice is connected (1) or not (0)",
	metricQDeviceInfo:        "Proxmox VE QDevice info (1 if configured)",
	metricSubInfo:            "Proxmox VE subscription info (1 if present)",
	metricSubStatus:          "Proxmox VE subscription status (1 if matches status)",
	metricSubNextDue:         "Subscription next due date as Unix timestamp",
	metricOnboot:             "Proxmox vm config onboot value",
	metricNotBackedUpTotal:   "Total number of guests not covered by any backup job.",
	metricNotBackedUpInfo:    "Present if guest is not covered by any backup job.",
	metricReplInfo:           "Proxmox vm replication info",
	metricReplDuration:       "Proxmox vm replication duration",
	metricReplLastSync:       "Proxmox vm replication last_sync",
	metricReplLastTry:        "Proxmox vm replication last_try",
	metricReplNextSync:       "Proxmox vm replication next_sync",
	metricReplFailed:         "Proxmox vm replication fail_count",
	metricCollectionDuration: "Duration of the last collection cycle for a target, in seconds.",
	metricRequestErrors:      "Total number of failed PVE API requests for a target.",
}

// helpFor returns the HELP text for a metric name (falling back to the name).
func helpFor(name string) string {
	if h, ok := metricHelp[name]; ok {
		return h
	}
	return name
}

// isCounter reports whether a metric should be exported as a counter.
func isCounter(name string) bool { return strings.HasSuffix(name, "_total") }
