package pve

import (
	"context"
	"sync"
	"time"

	"github.com/fjacquet/pve_exporter/internal/models"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// Target pairs a target's config with its API client.
type Target struct {
	Cfg    models.ClusterConfig
	Client Doer
}

// Collector polls every target on an interval and publishes snapshots.
type Collector struct {
	mu            sync.RWMutex
	targets       []Target
	store         *SnapshotStore
	interval      time.Duration
	timeout       time.Duration
	toggles       models.CollectorToggles
	maxConcurrent int
}

// SetTargets atomically swaps the target set (used on config hot reload).
func (c *Collector) SetTargets(targets []Target) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.targets = targets
}

func (c *Collector) snapshotTargets() []Target {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Target, len(c.targets))
	copy(out, c.targets)
	return out
}

// NewCollector builds a Collector.
func NewCollector(targets []Target, store *SnapshotStore, interval, timeout time.Duration, toggles models.CollectorToggles, maxConcurrent int) *Collector {
	return &Collector{
		targets:       targets,
		store:         store,
		interval:      interval,
		timeout:       timeout,
		toggles:       toggles,
		maxConcurrent: maxConcurrent,
	}
}

// CollectOnce runs a single cycle, publishes it, and returns the snapshot.
func (c *Collector) CollectOnce(ctx context.Context) *Snapshot {
	snap := c.collectAll(ctx)
	c.store.Store(snap)
	return snap
}

// Run drives collection on the configured interval until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.store.Store(c.collectAll(ctx))
		}
	}
}

func (c *Collector) collectAll(ctx context.Context) *Snapshot {
	targets := c.snapshotTargets()
	results := make([]*TargetSnapshot, len(targets))
	g, gctx := errgroup.WithContext(ctx)
	if c.maxConcurrent > 0 {
		g.SetLimit(c.maxConcurrent)
	}
	for i := range targets {
		i := i
		g.Go(func() error {
			results[i] = c.collectTarget(gctx, targets[i])
			return nil
		})
	}
	_ = g.Wait()
	return BuildSnapshot(results)
}

// collectTarget polls one target. A hard failure of the primary call yields a
// degraded snapshot (pve_up=0) rather than failing the whole cycle.
func (c *Collector) collectTarget(ctx context.Context, t Target) *TargetSnapshot {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	name := t.Cfg.Name
	ts := &TargetSnapshot{Cluster: name, LastScrape: time.Now()}
	set := newSampleSet(name)

	var resources []clusterResource
	if err := t.Client.Get(ctx, "/cluster/resources", &resources); err != nil {
		log.WithField("cluster", name).WithError(err).Error("failed to fetch /cluster/resources")
		ts.Up = false
		ts.ScrapeError = err.Error()
		set.add(metricUp, 0, idLabel("cluster/"+name))
		ts.Samples = set.out
		return ts
	}
	collectResources(set, resources)

	clusterID := "cluster/" + name
	var status []clusterStatusEntry
	if err := t.Client.Get(ctx, "/cluster/status", &status); err != nil {
		log.WithField("cluster", name).WithError(err).Warn("failed to fetch /cluster/status")
	} else {
		collectStatus(set, status)
		if cn := clusterNameFromStatus(status); cn != "" {
			clusterID = "cluster/" + cn
		}
	}

	var ver versionInfo
	if err := t.Client.Get(ctx, "/version", &ver); err != nil {
		log.WithField("cluster", name).WithError(err).Warn("failed to fetch /version")
	} else {
		collectVersion(set, ver)
	}

	if c.toggles.QDeviceEnabled() {
		var q qdeviceConfig
		if err := t.Client.Get(ctx, "/cluster/config/qdevice", &q); err != nil {
			log.WithField("cluster", name).WithError(err).Debug("qdevice not available")
		} else {
			collectQDevice(set, clusterID, q)
		}
	}

	if c.toggles.BackupInfoEnabled() {
		var guests []notBackedUpGuest
		if err := t.Client.Get(ctx, "/cluster/backup-info/not-backed-up", &guests); err != nil {
			log.WithField("cluster", name).WithError(err).Debug("backup-info not available")
		} else {
			collectNotBackedUp(set, clusterID, guests)
		}
	}

	c.collectNodes(ctx, t, nodeNames(resources), set)

	set.add(metricUp, 1, idLabel(clusterID))
	ts.Up = true
	ts.Samples = set.out
	return ts
}

// collectNodes fans out per-node collectors and merges their samples.
func (c *Collector) collectNodes(ctx context.Context, t Target, nodes []string, set *sampleSet) {
	if len(nodes) == 0 {
		return
	}
	if !c.toggles.ReplicationEnabled() && !c.toggles.SubscriptionEnabled() && !c.toggles.OnbootEnabled() {
		return
	}

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	if c.maxConcurrent > 0 {
		g.SetLimit(c.maxConcurrent)
	}
	for _, node := range nodes {
		node := node
		g.Go(func() error {
			local := newSampleSet(t.Cfg.Name)
			if c.toggles.ReplicationEnabled() {
				if err := collectReplication(gctx, t.Client, node, local); err != nil {
					log.WithFields(log.Fields{"cluster": t.Cfg.Name, "node": node}).WithError(err).Debug("replication collect failed")
				}
			}
			if c.toggles.SubscriptionEnabled() {
				if err := collectSubscription(gctx, t.Client, node, local); err != nil {
					log.WithFields(log.Fields{"cluster": t.Cfg.Name, "node": node}).WithError(err).Debug("subscription collect failed")
				}
			}
			if c.toggles.OnbootEnabled() {
				if err := collectOnboot(gctx, t.Client, node, local); err != nil {
					log.WithFields(log.Fields{"cluster": t.Cfg.Name, "node": node}).WithError(err).Debug("onboot collect failed")
				}
			}
			mu.Lock()
			set.out = append(set.out, local.out...)
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
}

// nodeNames extracts online node names from cluster resources.
func nodeNames(rows []clusterResource) []string {
	var names []string
	for _, r := range rows {
		if r.Type == "node" && r.Node != "" {
			names = append(names, r.Node)
		}
	}
	return names
}

// clusterNameFromStatus returns the cluster name from a /cluster/status result.
func clusterNameFromStatus(rows []clusterStatusEntry) string {
	for _, e := range rows {
		if e.Type == "cluster" {
			return e.Name
		}
	}
	return ""
}
