package models

import (
	"fmt"
	"os"
	"reflect"
	"sync"

	yaml "gopkg.in/yaml.v2"
)

// SecretResolver expands ${ENV_VAR} references and loads any *File secrets in
// place on the supplied config. It is applied after YAML decoding.
type SecretResolver func(*Config) error

// SafeConfig wraps a Config for thread-safe access and hot reload.
type SafeConfig struct {
	mu       sync.RWMutex
	c        *Config
	resolver SecretResolver
}

// NewSafeConfig returns a SafeConfig wrapping cfg with the given resolver.
func NewSafeConfig(cfg *Config, resolver SecretResolver) *SafeConfig {
	return &SafeConfig{c: cfg, resolver: resolver}
}

// Get returns the current config. Callers must not mutate the returned pointer.
func (s *SafeConfig) Get() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.c
}

// LoadConfig reads, resolves and validates a config file, returning it.
func LoadConfig(path string, resolver SecretResolver) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is an operator-supplied config file
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if resolver != nil {
		if err := resolver(&cfg); err != nil {
			return nil, fmt.Errorf("resolve secrets: %w", err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}

// ReloadConfig re-reads the config file and atomically swaps it in. It reports
// whether the set of cluster targets changed (so the caller can rebuild clients).
func (s *SafeConfig) ReloadConfig(path string) (clustersChanged bool, err error) {
	cfg, err := LoadConfig(path, s.resolver)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	clustersChanged = !reflect.DeepEqual(s.c.Clusters, cfg.Clusters)
	s.c = cfg
	return clustersChanged, nil
}
