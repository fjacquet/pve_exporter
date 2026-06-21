// Package models holds the configuration and shared data types for the exporter.
package models

import (
	"fmt"
	"strings"
	"time"
)

// Default values applied by SetDefaults when a field is left empty.
const (
	defaultHost         = "0.0.0.0"
	defaultPort         = "9221"
	defaultURI          = "/metrics"
	defaultInterval     = "30s"
	defaultTimeout      = "25s"
	defaultMetricsPush  = "10s"
	defaultPVEPort      = "8006"
	defaultSamplingRate = 0.1
	defaultOTLPEndpoint = "localhost:4317"
)

// ClusterConfig describes a single Proxmox VE target (a node or a cluster
// reachable through one API endpoint). Secret-bearing fields accept ${ENV_VAR}
// references that are expanded at load time.
type ClusterConfig struct {
	Name               string `yaml:"name"`
	Host               string `yaml:"host"`            // host or host:port; ${ENV_VAR} allowed
	TokenID            string `yaml:"tokenID"`         // user@realm!tokenname; ${ENV_VAR} allowed
	TokenSecret        string `yaml:"tokenSecret"`     // secret UUID; ${ENV_VAR} allowed
	TokenSecretFile    string `yaml:"tokenSecretFile"` // alternative: read secret from file
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
}

// BaseURL returns the API root for the target, defaulting the port to 8006.
func (c ClusterConfig) BaseURL() string {
	host := c.Host
	if !strings.Contains(host, ":") {
		host = host + ":" + defaultPVEPort
	}
	return "https://" + host + "/api2/json"
}

// AuthHeader returns the value for the Authorization header per the PVE API
// token scheme: "PVEAPIToken=USER@REALM!TOKENID=SECRET".
func (c ClusterConfig) AuthHeader() string {
	return "PVEAPIToken=" + c.TokenID + "=" + c.TokenSecret
}

// CollectorToggles enable or disable the optional (more expensive) collectors.
// All default to true via SetDefaults semantics (pointers distinguish unset).
type CollectorToggles struct {
	Onboot       *bool `yaml:"onboot"`
	Replication  *bool `yaml:"replication"`
	Subscription *bool `yaml:"subscription"`
	BackupInfo   *bool `yaml:"backupInfo"`
	QDevice      *bool `yaml:"qdevice"`
}

func boolOrTrue(p *bool) bool { return p == nil || *p }

// OnbootEnabled reports whether the per-guest onboot collector runs.
func (t CollectorToggles) OnbootEnabled() bool { return boolOrTrue(t.Onboot) }

// ReplicationEnabled reports whether the replication collector runs.
func (t CollectorToggles) ReplicationEnabled() bool { return boolOrTrue(t.Replication) }

// SubscriptionEnabled reports whether the subscription collector runs.
func (t CollectorToggles) SubscriptionEnabled() bool { return boolOrTrue(t.Subscription) }

// BackupInfoEnabled reports whether the not-backed-up collector runs.
func (t CollectorToggles) BackupInfoEnabled() bool { return boolOrTrue(t.BackupInfo) }

// QDeviceEnabled reports whether the QDevice collector runs.
func (t CollectorToggles) QDeviceEnabled() bool { return boolOrTrue(t.QDevice) }

// OTelExportConfig configures one OTLP signal (metrics or tracing).
type OTelExportConfig struct {
	Enabled      bool    `yaml:"enabled"`
	Endpoint     string  `yaml:"endpoint"`
	Insecure     bool    `yaml:"insecure"`
	Interval     string  `yaml:"interval"`     // metrics push interval
	SamplingRate float64 `yaml:"samplingRate"` // tracing only
}

// Config is the top-level exporter configuration.
type Config struct {
	Server struct {
		Host    string `yaml:"host"`
		Port    string `yaml:"port"`
		URI     string `yaml:"uri"`
		LogName string `yaml:"logName"`
	} `yaml:"server"`

	Collection struct {
		Interval             string `yaml:"interval"`
		Timeout              string `yaml:"timeout"`
		MaxConcurrentTargets int    `yaml:"maxConcurrentTargets"`
	} `yaml:"collection"`

	Collectors CollectorToggles `yaml:"collectors"`

	OpenTelemetry struct {
		Metrics OTelExportConfig `yaml:"metrics"`
		Tracing OTelExportConfig `yaml:"tracing"`
	} `yaml:"opentelemetry"`

	Clusters []ClusterConfig `yaml:"clusters"`
}

// SetDefaults fills empty optional fields with sensible defaults.
func (c *Config) SetDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = defaultHost
	}
	if c.Server.Port == "" {
		c.Server.Port = defaultPort
	}
	if c.Server.URI == "" {
		c.Server.URI = defaultURI
	}
	if c.Collection.Interval == "" {
		c.Collection.Interval = defaultInterval
	}
	if c.Collection.Timeout == "" {
		c.Collection.Timeout = defaultTimeout
	}
	if c.OpenTelemetry.Metrics.Endpoint == "" {
		c.OpenTelemetry.Metrics.Endpoint = defaultOTLPEndpoint
	}
	if c.OpenTelemetry.Metrics.Interval == "" {
		c.OpenTelemetry.Metrics.Interval = defaultMetricsPush
	}
	if c.OpenTelemetry.Tracing.Endpoint == "" {
		c.OpenTelemetry.Tracing.Endpoint = defaultOTLPEndpoint
	}
	if c.OpenTelemetry.Tracing.SamplingRate == 0 {
		c.OpenTelemetry.Tracing.SamplingRate = defaultSamplingRate
	}
}

// Validate applies defaults and verifies required fields.
func (c *Config) Validate() error {
	c.SetDefaults()

	if _, err := time.ParseDuration(c.Collection.Interval); err != nil {
		return fmt.Errorf("collection.interval: %w", err)
	}
	if _, err := time.ParseDuration(c.Collection.Timeout); err != nil {
		return fmt.Errorf("collection.timeout: %w", err)
	}
	if len(c.Clusters) == 0 {
		return fmt.Errorf("at least one cluster target must be configured")
	}
	seen := make(map[string]struct{}, len(c.Clusters))
	for i, cl := range c.Clusters {
		if cl.Name == "" {
			return fmt.Errorf("clusters[%d]: name is required", i)
		}
		if _, dup := seen[cl.Name]; dup {
			return fmt.Errorf("clusters[%d]: duplicate name %q", i, cl.Name)
		}
		seen[cl.Name] = struct{}{}
		if cl.Host == "" {
			return fmt.Errorf("clusters[%d] (%s): host is required", i, cl.Name)
		}
		if cl.TokenID == "" {
			return fmt.Errorf("clusters[%d] (%s): tokenID is required", i, cl.Name)
		}
		if cl.TokenSecret == "" && cl.TokenSecretFile == "" {
			return fmt.Errorf("clusters[%d] (%s): tokenSecret or tokenSecretFile is required", i, cl.Name)
		}
	}
	if c.OpenTelemetry.Metrics.Enabled {
		if _, err := time.ParseDuration(c.OpenTelemetry.Metrics.Interval); err != nil {
			return fmt.Errorf("opentelemetry.metrics.interval: %w", err)
		}
	}
	return nil
}

// GetServerAddress returns "host:port" for the HTTP listener.
func (c *Config) GetServerAddress() string {
	return c.Server.Host + ":" + c.Server.Port
}

func mustDuration(s, fallback string) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	d, _ := time.ParseDuration(fallback)
	return d
}

// GetCollectionInterval returns the parsed collection interval.
func (c *Config) GetCollectionInterval() time.Duration {
	return mustDuration(c.Collection.Interval, defaultInterval)
}

// GetCollectionTimeout returns the parsed per-cycle timeout.
func (c *Config) GetCollectionTimeout() time.Duration {
	return mustDuration(c.Collection.Timeout, defaultTimeout)
}

// GetMetricsPushInterval returns the OTLP metrics push interval.
func (c *Config) GetMetricsPushInterval() time.Duration {
	return mustDuration(c.OpenTelemetry.Metrics.Interval, defaultMetricsPush)
}

// GetMaxConcurrentTargets returns the cap on parallel target polls (0 = unlimited).
func (c *Config) GetMaxConcurrentTargets() int { return c.Collection.MaxConcurrentTargets }

// IsOTelMetricsEnabled reports whether OTLP metric export is enabled.
func (c *Config) IsOTelMetricsEnabled() bool { return c.OpenTelemetry.Metrics.Enabled }

// IsOTelTracingEnabled reports whether OTLP tracing is enabled.
func (c *Config) IsOTelTracingEnabled() bool { return c.OpenTelemetry.Tracing.Enabled }
