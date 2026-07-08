# Request-Error Counter: Exclude Expected-Absence 403/404 on Optional Endpoints — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `pve_request_errors_total` stop counting expected-absence responses (403 permission-denied, 404 feature-absent) on optional/best-effort endpoints, while still counting them on required endpoints and still counting real failures (5xx, transport) everywhere.

**Architecture:** Add a `GetOptional` sibling to `Client.Get`; both delegate to a private `get(ctx, path, out, optional bool)`. When `optional` is true, a 403/404 does not increment the error counter (but the error is still returned, so callers keep their existing "log at debug and continue" handling). The eight optional call sites switch to `GetOptional`; the three required sites (`/cluster/resources`, `/cluster/status`, `/version`) keep `Get`.

**Tech Stack:** Go, `go-resty/resty/v2` HTTP client, `logrus`, Prometheus `client_golang`, OpenTelemetry SDK metrics, `net/http/httptest` for tests. Module `github.com/fjacquet/pve_exporter`.

## Global Constraints

- **Absent-not-zero (ADR-0006):** a field/endpoint missing from the API yields **no series**, never `0`. A failed optional endpoint must produce no metrics for that endpoint.
- **Label invariant (ADR-0006):** one label-key set per metric name. **Do not add any label** to `pve_request_errors_total`.
- **Retry excludes 4xx:** only 429/5xx are retried; 4xx is terminal. (Existing client behavior — do not change.)
- **Decode errors are not counted today** (client.go: envelope/data unmarshal failures do not increment the counter). Keep that unchanged; it is out of scope.
- **Conventional Commits** for messages (`feat`, `fix`, `docs`, `test`, `refactor`).
- **Every commit ends with** the trailer:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- Work happens on branch `feat/request-errors-optional-403` (already checked out).

## File Structure

- Modify `internal/pve/client.go` — split `Get` into `Get` / `GetOptional` / private `get`; add `isExpectedAbsence`; add `GetOptional` to the `Doer` interface.
- Create `internal/pve/client_test.go` — client-level counter-policy unit tests.
- Modify `internal/pve/collector.go` — 2 optional call sites → `GetOptional`.
- Modify `internal/pve/ha.go` — 1 optional call site → `GetOptional`.
- Modify `internal/pve/node.go` — 5 optional call sites → `GetOptional`.
- Modify `internal/pve/collector_test.go` — parametrize `fakePVE` with per-path status overrides; add one collector-level integration test.
- Modify `internal/pve/help.go` — refine the `pve_request_errors_total` HELP string.
- Modify `docs/metrics.md` — update the table row and the counter note.
- Create `docs/adr/0008-request-error-counting.md`.
- Modify `docs/adr/index.md` — add the 0007 (pre-existing gap) and 0008 rows.

---

## Task 1: Client — `GetOptional` and the counting policy

**Files:**
- Modify: `internal/pve/client.go` (the `Doer` interface at lines 24–32 and the `Get` method at lines 92–117)
- Test: `internal/pve/client_test.go` (create)

**Interfaces:**
- Produces:
  - `func (c *Client) Get(ctx context.Context, path string, out interface{}) error` — unchanged signature; any transport error or non-200 counts.
  - `func (c *Client) GetOptional(ctx context.Context, path string, out interface{}) error` — new; 403/404 do **not** count, everything else does; still returns the error.
  - `Doer` interface gains `GetOptional(ctx context.Context, path string, out interface{}) error`.
- Consumes: nothing new.

- [ ] **Step 1: Write the failing test**

Create `internal/pve/client_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/pve/ -run TestGetCounterPolicy`
Expected: **FAIL — build error** `client.GetOptional undefined (type *Client has no field or method GetOptional)`.

- [ ] **Step 3: Refactor `client.go` — add `GetOptional`, private `get`, `isExpectedAbsence`, and extend `Doer`**

Replace the `Get` method (client.go:92–117) with:

```go
// Get fetches path and unmarshals the "data" field into out. Any transport
// error or non-200 response counts toward RequestErrors.
func (c *Client) Get(ctx context.Context, path string, out interface{}) error {
	return c.get(ctx, path, out, false)
}

// GetOptional is Get for best-effort endpoints whose absence is not an error.
// A 403 (permission denied) or 404 (feature absent) is expected on restricted
// tokens or feature-less clusters and is NOT counted toward RequestErrors; all
// other failures (transport, 5xx, other 4xx) still count. The error is still
// returned, so callers keep their existing "log debug and continue" handling.
func (c *Client) GetOptional(ctx context.Context, path string, out interface{}) error {
	return c.get(ctx, path, out, true)
}

// get is the shared implementation for Get and GetOptional. When optional is
// true, an expected-absence status (403/404) does not increment RequestErrors.
func (c *Client) get(ctx context.Context, path string, out interface{}, optional bool) error {
	resp, err := c.http.R().SetContext(ctx).Get(path)
	if err != nil {
		// resty has already exhausted its retry budget; this counts one logical
		// call failure, not the number of individual wire attempts. A transport
		// failure always counts, even for optional endpoints.
		c.requestErrors.Add(1)
		return fmt.Errorf("GET %s: %w", path, err)
	}
	if code := resp.StatusCode(); code != http.StatusOK {
		// Counted once per logical call after all retries, unless this is an
		// optional endpoint returning an expected-absence status (403/404).
		if !(optional && isExpectedAbsence(code)) {
			c.requestErrors.Add(1)
		}
		return fmt.Errorf("GET %s: unexpected status %d", path, code)
	}
	var env envelope
	if err := json.Unmarshal(resp.Body(), &env); err != nil {
		return fmt.Errorf("GET %s: decode envelope: %w", path, err)
	}
	if out == nil || len(env.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("GET %s: decode data: %w", path, err)
	}
	return nil
}

// isExpectedAbsence reports whether an HTTP status represents an optional
// endpoint that is unavailable by permission (403) or absence (404), as opposed
// to a genuine API failure that should count toward RequestErrors.
func isExpectedAbsence(code int) bool {
	return code == http.StatusForbidden || code == http.StatusNotFound
}
```

Then extend the `Doer` interface (client.go:24–32) to add `GetOptional` right after `Get`:

```go
// Doer performs unwrapped GET requests against one PVE target.
type Doer interface {
	// Get fetches path and unmarshals the response "data" field into out.
	Get(ctx context.Context, path string, out interface{}) error
	// GetOptional is Get for best-effort endpoints; a 403/404 response is not
	// counted toward RequestErrors (see GetOptional on *Client).
	GetOptional(ctx context.Context, path string, out interface{}) error
	// Name returns the configured target (cluster) name.
	Name() string
	// RequestErrors returns the cumulative count of failed PVE API requests.
	RequestErrors() int64
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w internal/pve/client.go internal/pve/client_test.go && go test ./internal/pve/ -run TestGetCounterPolicy -v`
Expected: **PASS** — all four subtests pass.

- [ ] **Step 5: Run the full package suite (nothing else regressed)**

Run: `go build ./... && go test ./internal/pve/`
Expected: **PASS** (existing `TestRequestErrorsIncrement`, `TestCollectorSnapshot`, etc. still green; the widened `Doer` interface is satisfied by `*Client`, and no other type implements it).

- [ ] **Step 6: Commit**

```bash
git add internal/pve/client.go internal/pve/client_test.go
git commit -m "feat(pve): add GetOptional excluding expected-absence 403/404 from the error counter

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Route optional endpoints through `GetOptional`

**Files:**
- Modify: `internal/pve/collector.go:141` (qdevice), `internal/pve/collector.go:150` (backup-info)
- Modify: `internal/pve/ha.go:24` (ha/status/current)
- Modify: `internal/pve/node.go:11, :28, :44, :65, :74` (replication, replication status, subscription, guest list, guest config)
- Test: `internal/pve/collector_test.go` (parametrize `fakePVE`; add integration test)

**Interfaces:**
- Consumes: `Client.GetOptional` from Task 1.
- Produces: no new exported symbols; the collector-level behavior that an optional endpoint's 403 yields `pve_request_errors_total == 0` and no series for that endpoint.

- [ ] **Step 1: Parametrize the test mock with per-path status overrides**

In `internal/pve/collector_test.go`, change the `fakePVE` signature and its handler loop so a path can be forced to a status. Replace the `func fakePVE(t *testing.T) *httptest.Server {` signature (line 19) with `func fakePVE(t *testing.T, statusOverrides map[string]int) *httptest.Server {`, and replace the handler-registration loop (lines 46–52) with:

```go
	mux := http.NewServeMux()
	for path, body := range routes {
		path, body := path, body
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			if code, ok := statusOverrides[path]; ok {
				w.WriteHeader(code)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})
	}
```

Update the one existing caller in `collectFixture` (line 71) from `srv := fakePVE(t)` to `srv := fakePVE(t, nil)`.

- [ ] **Step 2: Run the suite to confirm the refactor is behavior-neutral**

Run: `go test ./internal/pve/`
Expected: **PASS** — passing `nil` overrides changes nothing; all existing tests stay green.

- [ ] **Step 3: Add the failing integration test**

Append to `internal/pve/collector_test.go`:

```go
func TestOptionalEndpoint403NotCounted(t *testing.T) {
	// qdevice is an optional endpoint; a 403 there must neither emit a series
	// (absent-not-zero) nor inflate pve_request_errors_total.
	srv := fakePVE(t, map[string]int{
		"/api2/json/cluster/config/qdevice": http.StatusForbidden,
	})
	t.Cleanup(srv.Close)

	store := NewSnapshotStore()
	c := NewCollector([]Target{testTarget(t, srv)}, store, time.Minute, 10*time.Second, models.CollectorToggles{}, 0)
	snap := c.CollectOnce(context.Background())

	if n := len(snap.SamplesFor(metricQDeviceInfo)); n != 0 {
		t.Errorf("expected no %s samples when qdevice returns 403, got %d", metricQDeviceInfo, n)
	}

	samples := snap.SamplesFor(metricRequestErrors)
	if len(samples) == 0 {
		t.Fatalf("expected %s samples, got none", metricRequestErrors)
	}
	for _, s := range samples {
		if s.Value != 0 {
			t.Errorf("%s = %v, want 0 (403 on an optional endpoint must not count)", metricRequestErrors, s.Value)
		}
	}
}
```

- [ ] **Step 4: Run the integration test to verify it fails**

Run: `go test ./internal/pve/ -run TestOptionalEndpoint403NotCounted -v`
Expected: **FAIL** — `pve_request_errors_total = 1, want 0`, because qdevice still uses `Get`, which counts the 403.

- [ ] **Step 5: Swap the eight optional call sites to `GetOptional`**

Make these exact one-line replacements (`Get` → `GetOptional`; arguments unchanged):

`internal/pve/collector.go:141`
```go
		if err := t.Client.GetOptional(ctx, "/cluster/config/qdevice", &q); err != nil {
```
`internal/pve/collector.go:150`
```go
		if err := t.Client.GetOptional(ctx, "/cluster/backup-info/not-backed-up", &guests); err != nil {
```
`internal/pve/ha.go:24`
```go
	if err := c.GetOptional(ctx, "/cluster/ha/status/current", &entries); err != nil {
```
`internal/pve/node.go:11`
```go
	if err := c.GetOptional(ctx, fmt.Sprintf("/nodes/%s/replication", node), &jobs); err != nil {
```
`internal/pve/node.go:28`
```go
		if err := c.GetOptional(ctx, fmt.Sprintf("/nodes/%s/replication/%s/status", node, j.ID), &st); err != nil {
```
`internal/pve/node.go:44`
```go
	if err := c.GetOptional(ctx, fmt.Sprintf("/nodes/%s/subscription", node), &sub); err != nil {
```
`internal/pve/node.go:65`
```go
		if err := c.GetOptional(ctx, fmt.Sprintf("/nodes/%s/%s", node, typ), &guests); err != nil {
```
`internal/pve/node.go:74`
```go
			if err := c.GetOptional(ctx, fmt.Sprintf("/nodes/%s/%s/%s/config", node, typ, vmid), &cfg); err != nil {
```

Leave `/cluster/resources` (collector.go:108), `/cluster/status` (collector.go:123), and `/version` (collector.go:133) on `Get`.

- [ ] **Step 6: Run the integration test and the full suite to verify green**

Run: `go build ./... && go test ./internal/pve/ -run TestOptionalEndpoint403NotCounted -v && go test ./...`
Expected: **PASS** for the integration test and the whole module.

- [ ] **Step 7: Commit**

```bash
git add internal/pve/collector.go internal/pve/ha.go internal/pve/node.go internal/pve/collector_test.go
git commit -m "fix(metrics): don't count 403/404 from optional endpoints in pve_request_errors_total

Route qdevice, backup-info, HA status, replication, subscription and
onboot through GetOptional so a deliberately-restricted token no longer
pins pve_request_errors_total above zero. Required endpoints and real
5xx/transport failures still count.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Documentation and ADR-0008

**Files:**
- Modify: `internal/pve/help.go:43`
- Modify: `docs/metrics.md` (row at line 121; note at lines 124–125)
- Create: `docs/adr/0008-request-error-counting.md`
- Modify: `docs/adr/index.md` (add the 0007 and 0008 rows)

**Interfaces:** none (documentation only).

- [ ] **Step 1: Refine the HELP string**

In `internal/pve/help.go:43`, replace:

```go
	metricRequestErrors:      "Total number of failed PVE API requests for a target.",
```
with:
```go
	metricRequestErrors:      "Total failed PVE API requests for a target (excludes 403/404 on optional endpoints).",
```

- [ ] **Step 2: Update `docs/metrics.md`**

Replace the table row (line 121):

```markdown
| `pve_request_errors_total`          | counter | `cluster`, `id` | Total number of failed PVE API requests for a target.             |
```
with:
```markdown
| `pve_request_errors_total`          | counter | `cluster`, `id` | Total failed PVE API requests for a target (excludes 403/404 on optional endpoints). |
```

Replace the note (the sentence at lines 124–125):

```markdown
`pve_request_errors_total` is a process-lifetime monotonic counter — it accumulates
across collection cycles and increments on any transport error or non-200 HTTP response.
```
with:
```markdown
`pve_request_errors_total` is a process-lifetime monotonic counter — it accumulates
across collection cycles and increments on any transport error or non-200 HTTP response
on a **required** endpoint (`/cluster/resources`, `/cluster/status`, `/version`).
Expected-absence responses — 403 (permission denied) or 404 (feature absent) — on the
optional endpoints (qdevice, backup-info, HA status, replication, subscription, onboot)
are **not** counted, so a deliberately-restricted token does not inflate the counter;
genuine 5xx or transport failures on those endpoints still count (ADR-0008).
```

- [ ] **Step 3: Create ADR-0008**

Create `docs/adr/0008-request-error-counting.md`:

```markdown
# ADR-0008 — Request-error counting excludes expected-absence on optional endpoints

**Status:** Accepted  
**Date:** 2026-07-08

## Context

`pve_request_errors_total` is the counter operators alert on to detect a sick
target. It is incremented in the client on any transport error or non-200
response. Several endpoints the collector scrapes are best-effort — qdevice,
backup-info, HA status, per-node replication, subscription, and onboot — and
already log their failures at `debug` ("not available") and continue.

A deliberately-restricted API token, or a cluster that simply does not run a
feature, makes those endpoints return `403 Permission check failed` or `404`.
Because the counter lived one layer below the required/optional distinction, a
restricted-but-intentional token pinned `pve_request_errors_total` permanently
above zero — a standing false alert. This was observed in the field: a token
missing `Sys.Audit` produced `pve_request_errors_total = 4` (three optional
endpoints plus `/cluster/status`) while `pve_up` still read `1`.

## Decision

Split the client GET into two methods sharing one implementation:

- `Get` — required endpoints (`/cluster/resources`, `/cluster/status`,
  `/version`). Any failure counts (unchanged).
- `GetOptional` — best-effort endpoints. An expected-absence status (403
  permission-denied, 404 feature-absent) does **not** count; every other
  failure (5xx, transport) still counts. The error is still returned, so callers
  keep logging at `debug` and continuing.

A blanket "never count 403" was rejected: a 403 on a *required* endpoint means
the exporter is blind, and — as the field incident showed — the nonzero counter
was the only signal, because `pve_up` still read `1`. The required/optional
axis, not the status code alone, decides whether a 403 counts.

## Consequences

- A restricted token no longer inflates `pve_request_errors_total`; alerting on
  `rate(pve_request_errors_total[5m]) > 0` is meaningful again.
- Genuine failures (5xx, transport) on optional endpoints still count, so a sick
  optional endpoint is not silently hidden.
- The required/optional classification lives at each call site, which already
  chose its log level accordingly; a new optional endpoint calls `GetOptional`.
- No metric labels changed (ADR-0006 holds); this is a pure counter-semantics
  change, documented in `docs/metrics.md`.
```

- [ ] **Step 4: Add the index rows**

In `docs/adr/index.md`, after the `[0006]` row, append:

```markdown
| [0007](0007-qdevice-state-gap.md) | QDevice tie-breaker/state gap | Accepted |
| [0008](0008-request-error-counting.md) | Request-error counting on optional endpoints | Accepted |
```

- [ ] **Step 5: Verify build/tests and review the docs diff**

Run: `go test ./internal/pve/ && git --no-pager diff --stat`
Expected: **PASS** (no test asserts HELP text, so the string change is safe) and the diff touches only `help.go`, `docs/metrics.md`, `docs/adr/0008-request-error-counting.md`, `docs/adr/index.md`.

- [ ] **Step 6: Commit**

```bash
git add internal/pve/help.go docs/metrics.md docs/adr/0008-request-error-counting.md docs/adr/index.md
git commit -m "docs: document optional-endpoint error-counting policy (ADR-0008)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Policy (optional 403/404 not counted; else counted; required unchanged) → Task 1 (`get`/`isExpectedAbsence`) + Task 2 (call-site routing). ✓
- Transport errors always count; decode errors unchanged → Task 1 `get` (comments + preserved branches). ✓
- `GetOptional` added to `Doer` → Task 1 Step 3. ✓
- 8 optional call sites swapped; 3 required kept → Task 2 Step 5. ✓
- 4 policy test cases → Task 1 `TestGetCounterPolicy` (optional 403, optional 404, optional 500, required 403). ✓
- Integration: optional 403 ⇒ series absent AND counter 0 → Task 2 `TestOptionalEndpoint403NotCounted`. ✓
- Docs (metrics.md HELP + note, help.go, ADR-0008, index) → Task 3. ✓
- Non-goals honored: no new labels, no log-level change, no `/cluster/resources` filtering change. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code and exact commands. ✓

**Type consistency:** `GetOptional(ctx context.Context, path string, out interface{}) error` is identical across the `Doer` interface, the `*Client` method, and every call site. `isExpectedAbsence(code int) bool` used only inside `get`. Test helper `clientForStatus(t, status)` and `fakePVE(t, statusOverrides)` signatures match their call sites. ✓
