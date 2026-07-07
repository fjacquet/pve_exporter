package pve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fjacquet/pve_exporter/internal/models"
)

// clientForStatus returns a Client pointed at a TLS server that answers every
// request with the given HTTP status and no body.
func clientForStatus(t *testing.T, status int) *Client {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	cfg := models.ClusterConfig{
		Name:               "opt-test",
		Host:               strings.TrimPrefix(srv.URL, "https://"),
		TokenID:            "exporter@pam!t",
		TokenSecret:        "secret",
		InsecureSkipVerify: true,
	}
	return NewClient(cfg, false)
}

// TestGetCounterPolicy pins the required/optional counting policy:
// optional 403/404 do not count; optional 5xx and required 403 do.
func TestGetCounterPolicy(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		optional    bool
		wantCounted bool
	}{
		{"optional 403 not counted", http.StatusForbidden, true, false},
		{"optional 404 not counted", http.StatusNotFound, true, false},
		{"optional 500 counted", http.StatusInternalServerError, true, true},
		{"required 403 counted", http.StatusForbidden, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := clientForStatus(t, tc.status)
			before := client.RequestErrors()

			var err error
			if tc.optional {
				err = client.GetOptional(context.Background(), "/cluster/status", nil)
			} else {
				err = client.Get(context.Background(), "/cluster/status", nil)
			}
			if err == nil {
				t.Fatalf("expected an error for status %d", tc.status)
			}

			counted := client.RequestErrors() > before
			if counted != tc.wantCounted {
				t.Errorf("status=%d optional=%v: counted=%v, want %v",
					tc.status, tc.optional, counted, tc.wantCounted)
			}
		})
	}
}
