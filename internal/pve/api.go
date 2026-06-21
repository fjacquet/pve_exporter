package pve

// PVE wraps every response in {"data": ...}; the client unwraps "data" into the
// types below. Numeric fields use FlexFloat so absent/odd values become absent
// samples instead of fake zeros.

// clusterResource is one row of GET /cluster/resources.
type clusterResource struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"` // node, qemu, lxc, storage, sdn, pool
	Node       string    `json:"node"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	CPU        FlexFloat `json:"cpu"`
	MaxCPU     FlexFloat `json:"maxcpu"`
	Mem        FlexFloat `json:"mem"`
	MaxMem     FlexFloat `json:"maxmem"`
	Disk       FlexFloat `json:"disk"`
	MaxDisk    FlexFloat `json:"maxdisk"`
	Uptime     FlexFloat `json:"uptime"`
	DiskRead   FlexFloat `json:"diskread"`
	DiskWrite  FlexFloat `json:"diskwrite"`
	NetIn      FlexFloat `json:"netin"`
	NetOut     FlexFloat `json:"netout"`
	Template   FlexFloat `json:"template"`
	Shared     FlexFloat `json:"shared"`
	Tags       string    `json:"tags"`
	Storage    string    `json:"storage"`
	PluginType string    `json:"plugintype"`
	Content    string    `json:"content"`
	HAState    string    `json:"hastate"`
	Lock       string    `json:"lock"`
	VMID       FlexFloat `json:"vmid"`
}

// clusterStatusEntry is one row of GET /cluster/status.
type clusterStatusEntry struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"` // "cluster" or "node"
	Name    string    `json:"name"`
	Nodes   FlexFloat `json:"nodes"`
	Quorate FlexFloat `json:"quorate"`
	Version FlexFloat `json:"version"`
	NodeID  FlexFloat `json:"nodeid"`
	Level   string    `json:"level"`
	Online  FlexFloat `json:"online"`
}

// versionInfo is GET /version.
type versionInfo struct {
	Release string `json:"release"`
	RepoID  string `json:"repoid"`
	Version string `json:"version"`
}

// qdeviceConfig is GET /cluster/config/qdevice (present only when configured).
type qdeviceConfig struct {
	Model   string            `json:"model"`
	Network map[string]string `json:"network"`
}

// notBackedUpGuest is one row of GET /cluster/backup-info/not-backed-up.
type notBackedUpGuest struct {
	VMID FlexFloat `json:"vmid"`
	Type string    `json:"type"`
	Name string    `json:"name"`
}

// subscriptionInfo is GET /nodes/{node}/subscription.
type subscriptionInfo struct {
	Status      string `json:"status"`
	Level       string `json:"level"`
	NextDueDate string `json:"nextduedate"`
}

// guestRef is one row of GET /nodes/{node}/qemu or /lxc (vmid listing).
type guestRef struct {
	VMID FlexFloat `json:"vmid"`
}

// guestConfig is GET /nodes/{node}/{qemu,lxc}/{vmid}/config (onboot field).
type guestConfig struct {
	Onboot FlexFloat `json:"onboot"`
}

// replicationJob is one row of GET /nodes/{node}/replication.
type replicationJob struct {
	ID     string    `json:"id"`     // e.g. "1-0"
	Type   string    `json:"type"`   // e.g. "local"
	Source string    `json:"source"` // node name
	Target string    `json:"target"` // node name
	Guest  FlexFloat `json:"guest"`
}

// replicationStatus is GET /nodes/{node}/replication/{id}/status.
type replicationStatus struct {
	Duration  FlexFloat `json:"duration"`
	LastSync  FlexFloat `json:"last_sync"`
	LastTry   FlexFloat `json:"last_try"`
	NextSync  FlexFloat `json:"next_sync"`
	FailCount FlexFloat `json:"fail_count"`
}
