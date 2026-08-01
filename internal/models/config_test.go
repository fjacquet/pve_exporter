package models

import (
	"strings"
	"testing"
)

// validConfig returns a Config that passes Validate, for tests that then break
// exactly one field.
func validConfig() *Config {
	c := &Config{}
	c.Clusters = []ClusterConfig{{
		Name:        "pve1",
		Host:        "pve.example.com",
		TokenID:     "exporter@pve!metrics",
		TokenSecret: "00000000-0000-0000-0000-000000000000",
	}}
	return c
}

// TestValidateRejectsReservedURI pins that server.uri cannot collide with a
// route the exporter registers itself. Without this, setting uri to /livez
// panics http.ServeMux at startup with a message naming no config field.
func TestValidateRejectsReservedURI(t *testing.T) {
	for _, uri := range []string{"/", "/health", "/livez", "/readyz"} {
		t.Run(uri, func(t *testing.T) {
			c := validConfig()
			c.Server.URI = uri

			err := c.Validate()
			if err == nil {
				t.Fatalf("server.uri = %q accepted, want rejection", uri)
			}
			if !strings.Contains(err.Error(), "server.uri") {
				t.Fatalf("error %q does not name the offending field", err)
			}
		})
	}
}

func TestValidateRejectsRelativeURI(t *testing.T) {
	c := validConfig()
	c.Server.URI = "metrics"

	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "server.uri") {
		t.Fatalf("err = %v, want a server.uri error", err)
	}
}

func TestValidateAcceptsDefaultAndCustomURI(t *testing.T) {
	for _, uri := range []string{"", "/metrics", "/pve/metrics"} {
		t.Run(uri, func(t *testing.T) {
			c := validConfig()
			c.Server.URI = uri

			if err := c.Validate(); err != nil {
				t.Fatalf("server.uri = %q rejected: %v", uri, err)
			}
		})
	}
}
