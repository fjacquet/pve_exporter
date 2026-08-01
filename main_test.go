package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStaticOKHandlerReturns200(t *testing.T) {
	for _, path := range []string{"/livez", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			staticOKHandler(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Body.String(); got != "ok" {
				t.Fatalf("body = %q, want %q", got, "ok")
			}
		})
	}
}

// newProbeMux builds the exporter's real route table via registerEndpoints —
// the same call run() makes. Only the metrics handler is omitted (nil), since
// it is the one route that needs a registry; the probe and health routes under
// test are the shipping registrations, not copies.
func newProbeMux() *http.ServeMux {
	mux := http.NewServeMux()
	registerEndpoints(mux, nil, "/metrics")
	return mux
}

func TestProbeRoutesOnMux(t *testing.T) {
	mux := newProbeMux()

	for _, path := range []string{"/livez", "/readyz", "/health"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusOK)
			}
			if got := rec.Body.String(); got != "ok" {
				t.Fatalf("%s body = %q, want %q", path, got, "ok")
			}
		})
	}
}

// TestHealthAlwaysReturns200 pins the property this repo already had before
// the probe work: /health never gates on collection state. Unlike the
// sibling exporters it was never 503, and it must not become 503.
func TestHealthAlwaysReturns200(t *testing.T) {
	mux := newProbeMux()

	// No collection has run; nothing has ever been stored in a snapshot.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("cold /health status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestRegisterEndpointsServesMetricsAtURI pins that the metrics handler lands
// on the configured exposition path and not on a probe path.
func TestRegisterEndpointsServesMetricsAtURI(t *testing.T) {
	mux := http.NewServeMux()
	registerEndpoints(mux, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("exposition"))
	}), "/custom-metrics")

	req := httptest.NewRequest(http.MethodGet, "/custom-metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "exposition" {
		t.Fatalf("metrics route: status=%d body=%q", rec.Code, rec.Body.String())
	}
}
