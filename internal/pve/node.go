package pve

import (
	"context"
	"fmt"
)

// collectReplication emits replication job info and status for one node.
func collectReplication(ctx context.Context, c Doer, node string, s *sampleSet) error {
	var jobs []replicationJob
	if err := c.Get(ctx, fmt.Sprintf("/nodes/%s/replication", node), &jobs); err != nil {
		return err
	}
	for _, j := range jobs {
		guest := ""
		if j.Guest.Valid {
			guest = "qemu/" + formatInt(j.Guest.Value)
		}
		s.add(metricReplInfo, 1,
			idLabel(j.ID),
			Label{Name: "guest", Value: guest},
			Label{Name: "source", Value: "node/" + j.Source},
			Label{Name: "target", Value: "node/" + j.Target},
			Label{Name: "type", Value: j.Type},
		)

		var st replicationStatus
		if err := c.Get(ctx, fmt.Sprintf("/nodes/%s/replication/%s/status", node, j.ID), &st); err != nil {
			continue // status is best-effort per job
		}
		id := idLabel(j.ID)
		s.addFlex(metricReplDuration, st.Duration, id)
		s.addFlex(metricReplLastSync, st.LastSync, id)
		s.addFlex(metricReplLastTry, st.LastTry, id)
		s.addFlex(metricReplNextSync, st.NextSync, id)
		s.addFlex(metricReplFailed, st.FailCount, id)
	}
	return nil
}

// collectSubscription emits subscription info/status/due for one node.
func collectSubscription(ctx context.Context, c Doer, node string, s *sampleSet) error {
	var sub subscriptionInfo
	if err := c.Get(ctx, fmt.Sprintf("/nodes/%s/subscription", node), &sub); err != nil {
		return err
	}
	id := idLabel("node/" + node)
	if sub.Level != "" {
		s.add(metricSubInfo, 1, id, Label{Name: "level", Value: sub.Level})
	}
	if sub.Status != "" {
		s.addStateEnum(metricSubStatus, "node/"+node, sub.Status, subStates)
	}
	if ts, ok := parseDueDate(sub.NextDueDate); ok {
		s.add(metricSubNextDue, ts, id)
	}
	return nil
}

// collectOnboot emits the configured onboot value for every guest on one node.
// This is the most expensive collector (one config call per guest).
func collectOnboot(ctx context.Context, c Doer, node string, s *sampleSet) error {
	for _, typ := range []string{"qemu", "lxc"} {
		var guests []guestRef
		if err := c.Get(ctx, fmt.Sprintf("/nodes/%s/%s", node, typ), &guests); err != nil {
			return err
		}
		for _, g := range guests {
			if !g.VMID.Valid {
				continue
			}
			vmid := formatInt(g.VMID.Value)
			var cfg guestConfig
			if err := c.Get(ctx, fmt.Sprintf("/nodes/%s/%s/%s/config", node, typ, vmid), &cfg); err != nil {
				continue
			}
			onboot := 0.0
			if cfg.Onboot.Valid {
				onboot = cfg.Onboot.Value
			}
			s.add(metricOnboot, onboot,
				idLabel(typ+"/"+vmid),
				Label{Name: "node", Value: node},
				Label{Name: "type", Value: typ},
			)
		}
	}
	return nil
}
