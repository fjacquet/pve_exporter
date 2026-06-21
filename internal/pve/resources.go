package pve

// addFlex adds a sample only when the value parsed successfully — an absent
// value yields no sample (never a misleading zero).
func (s *sampleSet) addFlex(name string, f FlexFloat, labels ...Label) {
	if f.Valid {
		s.add(name, f.Value, labels...)
	}
}

// addStateEnum emits one series per possible state with a stable label-key set:
// 1.0 for the active state, 0.0 for the rest.
func (s *sampleSet) addStateEnum(name, id, active string, states []string) {
	for _, st := range states {
		s.add(name, boolToFloat(st == active), idLabel(id), Label{Name: "state", Value: st})
	}
}

// collectResources turns /cluster/resources rows into samples.
func collectResources(s *sampleSet, rows []clusterResource) {
	for i := range rows {
		r := rows[i]
		switch r.Type {
		case "node":
			collectNodeResource(s, r)
		case "qemu", "lxc":
			collectGuestResource(s, r)
		case "storage":
			collectStorageResource(s, r)
		}
	}
}

func collectNodeResource(s *sampleSet, r clusterResource) {
	id := idLabel(r.ID)
	s.add(metricUp, boolToFloat(r.Status == "online"), id)
	s.addFlex(metricCPURatio, r.CPU, id)
	s.addFlex(metricCPULimit, r.MaxCPU, id)
	s.addFlex(metricMemSize, r.MaxMem, id)
	s.addFlex(metricMemUsage, r.Mem, id)
	s.addFlex(metricDiskSize, r.MaxDisk, id)
	s.addFlex(metricDiskUsage, r.Disk, id)
	s.addFlex(metricUptime, r.Uptime, id)
}

func collectGuestResource(s *sampleSet, r clusterResource) {
	id := idLabel(r.ID)
	s.add(metricUp, boolToFloat(r.Status == "running"), id)
	s.addFlex(metricCPURatio, r.CPU, id)
	s.addFlex(metricCPULimit, r.MaxCPU, id)
	s.addFlex(metricMemSize, r.MaxMem, id)
	s.addFlex(metricMemUsage, r.Mem, id)
	s.addFlex(metricDiskSize, r.MaxDisk, id)
	s.addFlex(metricDiskUsage, r.Disk, id)
	s.addFlex(metricUptime, r.Uptime, id)

	// Cumulative IO/network counters (guests only).
	s.addFlex(metricNetRxTotal, r.NetIn, id)
	s.addFlex(metricNetTxTotal, r.NetOut, id)
	s.addFlex(metricDiskReadTotal, r.DiskRead, id)
	s.addFlex(metricDiskWriteTotal, r.DiskWrite, id)

	if r.HAState != "" {
		s.addStateEnum(metricHAState, r.ID, r.HAState, haGuestStates)
	}
	if r.Lock != "" {
		s.addStateEnum(metricLockState, r.ID, r.Lock, lockStates)
	}

	template := "0"
	if r.Template.Valid && r.Template.Value == 1 {
		template = "1"
	}
	s.add(metricGuestInfo, 1,
		id,
		Label{Name: "node", Value: r.Node},
		Label{Name: "name", Value: r.Name},
		Label{Name: "type", Value: r.Type},
		Label{Name: "template", Value: template},
		Label{Name: "tags", Value: r.Tags},
	)
}

func collectStorageResource(s *sampleSet, r clusterResource) {
	id := idLabel(r.ID)
	s.add(metricUp, boolToFloat(r.Status == "available"), id)
	s.addFlex(metricDiskSize, r.MaxDisk, id)
	s.addFlex(metricDiskUsage, r.Disk, id)
	s.addFlex(metricStorShare, r.Shared, id)
	s.add(metricStorageInfo, 1,
		id,
		Label{Name: "node", Value: r.Node},
		Label{Name: "storage", Value: r.Storage},
		Label{Name: "plugintype", Value: r.PluginType},
		Label{Name: "content", Value: r.Content},
	)
}

// collectStatus turns /cluster/status rows into node/cluster info samples.
func collectStatus(s *sampleSet, rows []clusterStatusEntry) {
	for i := range rows {
		e := rows[i]
		switch e.Type {
		case "node":
			nodeid := ""
			if e.NodeID.Valid {
				nodeid = formatInt(e.NodeID.Value)
			}
			s.add(metricNodeInfo, 1,
				idLabel(e.ID),
				Label{Name: "name", Value: e.Name},
				Label{Name: "level", Value: e.Level},
				Label{Name: "nodeid", Value: nodeid},
			)
		case "cluster":
			nodes := ""
			if e.Nodes.Valid {
				nodes = formatInt(e.Nodes.Value)
			}
			quorate := ""
			if e.Quorate.Valid {
				quorate = formatInt(e.Quorate.Value)
			}
			version := ""
			if e.Version.Valid {
				version = formatInt(e.Version.Value)
			}
			s.add(metricClusterInfo, 1,
				idLabel(e.ID),
				Label{Name: "nodes", Value: nodes},
				Label{Name: "quorate", Value: quorate},
				Label{Name: "version", Value: version},
			)
		}
	}
}

// collectVersion turns /version into a version_info sample.
func collectVersion(s *sampleSet, v versionInfo) {
	s.add(metricVersionInfo, 1,
		Label{Name: "release", Value: v.Release},
		Label{Name: "repoid", Value: v.RepoID},
		Label{Name: "version", Value: v.Version},
	)
}

// collectQDevice emits qdevice presence/info when a QDevice is configured.
func collectQDevice(s *sampleSet, clusterID string, q qdeviceConfig) {
	if q.Model == "" {
		return
	}
	id := idLabel(clusterID)
	s.add(metricQDeviceUp, 1, id)
	s.add(metricQDeviceInfo, 1,
		id,
		Label{Name: "model", Value: q.Model},
		Label{Name: "host", Value: q.Network["host"]},
		Label{Name: "algorithm", Value: q.Network["algorithm"]},
	)
}

// collectNotBackedUp emits the not-backed-up total and per-guest info samples.
func collectNotBackedUp(s *sampleSet, clusterID string, guests []notBackedUpGuest) {
	s.add(metricNotBackedUpTotal, float64(len(guests)), idLabel(clusterID))
	for _, g := range guests {
		if !g.VMID.Valid || g.Type == "" {
			continue
		}
		s.add(metricNotBackedUpInfo, 1, idLabel(g.Type+"/"+formatInt(g.VMID.Value)))
	}
}
