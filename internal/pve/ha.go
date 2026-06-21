package pve

import (
	"context"
	"strings"

	log "github.com/sirupsen/logrus"
)

// haNodeStatusMap maps PVE node HA status strings to canonical haNodeStates.
var haNodeStatusMap = map[string]string{
	"online":      "online",
	"maintenance": "maintenance",
	"unknown":     "unknown",
	"fence":       "fence",
	"gone":        "gone",
}

// collectHAStatus emits node-level HA state metrics from /cluster/ha/status/current.
// Guest HA state is handled separately via /cluster/resources hastate field.
// Returns silently when the endpoint is absent or returns no node entries.
func collectHAStatus(ctx context.Context, c Doer, set *sampleSet) {
	var entries []haStatusEntry
	if err := c.Get(ctx, "/cluster/ha/status/current", &entries); err != nil {
		log.WithField("cluster", c.Name()).WithError(err).Debug("ha/status/current not available")
		return
	}
	for _, e := range entries {
		// Only "node" entries carry node HA state; lrm/service/quorum entries
		// represent other HA subsystems and are not node-level state.
		if e.Type != "node" || e.Node == "" {
			continue
		}
		canonical := canonicalNodeHAState(e.Status)
		if canonical == "" {
			continue
		}
		set.addStateEnum(metricHAState, "node/"+e.Node, canonical, haNodeStates)
	}
}

// canonicalNodeHAState maps a PVE node HA status string to a canonical haNodeState.
// Returns "" when the status cannot be mapped cleanly (skip rather than fabricate).
func canonicalNodeHAState(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	if mapped, ok := haNodeStatusMap[s]; ok {
		return mapped
	}
	return ""
}
