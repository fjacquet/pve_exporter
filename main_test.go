package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
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

// freePort reserves and immediately releases a loopback port, returning it for
// the process under test to bind.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = ln.Close() }()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	return port
}

// TestLivezAnswersBeforeFirstCollectionCompletes is the regression test for the
// startup ordering: the HTTP listener must bind before the first collection
// cycle. The configured target is a TEST-NET-3 address that blackholes TCP, so
// the first cycle cannot finish before the 25s collection timeout — if the
// listener waited on it, /livez could not answer inside this test's budget.
func TestLivezAnswersBeforeFirstCollectionCompletes(t *testing.T) {
	port := freePort(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := fmt.Sprintf(`server:
  host: "127.0.0.1"
  port: %q
  uri: "/metrics"
collection:
  interval: "30s"
  timeout: "25s"
clusters:
  - name: unreachable
    host: "203.0.113.1:8006"
    tokenID: "exporter@pve!metrics"
    tokenSecret: "00000000-0000-0000-0000-000000000000"
    insecureSkipVerify: true
`, port)
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prevConfig, prevOnce := configFile, once
	t.Cleanup(func() { configFile, once = prevConfig, prevOnce })
	configFile, once = path, false

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("run() did not return after context cancellation")
		}
	})

	// 5s is comfortably below the 25s collection timeout (30s startup deadline)
	// that a blocking first cycle would impose.
	deadline := time.Now().Add(5 * time.Second)
	url := "http://127.0.0.1:" + port + "/livez"
	client := &http.Client{Timeout: time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("build probe request: %v", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			code := resp.StatusCode
			_ = resp.Body.Close()
			if code == http.StatusOK {
				return
			}
			t.Fatalf("/livez status = %d, want %d", code, http.StatusOK)
		}
		if time.Now().After(deadline) {
			t.Fatalf("/livez unreachable within 5s of startup: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
