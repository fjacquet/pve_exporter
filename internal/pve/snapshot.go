package pve

import (
	"sync"
	"time"
)

// TargetSnapshot is one target's collected state at one cycle.
type TargetSnapshot struct {
	Cluster     string
	Up          bool
	ScrapeError string
	LastScrape  time.Time
	Samples     []Sample
}

// Snapshot is an immutable, indexed view across all targets, built once per
// cycle and published by atomic pointer swap.
type Snapshot struct {
	PerTarget map[string]*TargetSnapshot
	byName    map[string][]Sample
	names     []string
}

// BuildSnapshot indexes the per-target results by metric name.
func BuildSnapshot(targets []*TargetSnapshot) *Snapshot {
	s := &Snapshot{
		PerTarget: make(map[string]*TargetSnapshot, len(targets)),
		byName:    make(map[string][]Sample),
	}
	for _, t := range targets {
		if t == nil {
			continue
		}
		s.PerTarget[t.Cluster] = t
		for _, sample := range t.Samples {
			if _, ok := s.byName[sample.Name]; !ok {
				s.names = append(s.names, sample.Name)
			}
			s.byName[sample.Name] = append(s.byName[sample.Name], sample)
		}
	}
	return s
}

// MetricNames returns the distinct metric names present in the snapshot.
func (s *Snapshot) MetricNames() []string { return s.names }

// SamplesFor returns all samples for a metric name.
func (s *Snapshot) SamplesFor(name string) []Sample { return s.byName[name] }

// Targets returns the per-target snapshots.
func (s *Snapshot) Targets() map[string]*TargetSnapshot { return s.PerTarget }

// SnapshotStore holds the current snapshot behind an RWMutex pointer swap.
type SnapshotStore struct {
	mu      sync.RWMutex
	current *Snapshot
}

// NewSnapshotStore returns a store seeded with an empty snapshot.
func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{current: BuildSnapshot(nil)}
}

// Load returns the current snapshot pointer for concurrent readers.
func (s *SnapshotStore) Load() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Store atomically swaps in a new snapshot.
func (s *SnapshotStore) Store(snap *Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = snap
}
