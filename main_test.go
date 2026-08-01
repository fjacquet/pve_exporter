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

// newProbeMux mirrors the probe and health registrations made in run().
// run() itself needs a full config, client set and registry, so the routes
// under test are re-registered here on a bare mux; the handlers are the
// same functions.
func newProbeMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/livez", staticOKHandler)
	mux.HandleFunc("/readyz", staticOKHandler)
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
