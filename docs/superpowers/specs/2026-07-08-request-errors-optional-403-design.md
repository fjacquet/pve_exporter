# Design: exclude expected-absence (403/404) on optional endpoints from `pve_request_errors_total`

**Date:** 2026-07-08
**Status:** Approved (design), pending implementation
**Scope:** `internal/pve` client + collector; `docs/metrics.md`; new ADR-0008

## Problem

`pve_request_errors_total` conflates two very different things: genuine API
failures (5xx, transport errors) and *expected absence* — a `403` because the
token is intentionally restricted, or a feature the cluster simply does not
run.

Evidence, from a live `--trace` run against `pve1` captured in two passes ~80s
apart:

- **Run 1** (token missing `Sys.Audit`): four endpoints returned
  `403 Permission check failed (/, Sys.Audit)` — `/cluster/status`,
  `/cluster/config/qdevice`, `/cluster/backup-info/not-backed-up`,
  `/cluster/ha/status/current`. Result: `pve_request_errors_total = 4`, while
  `pve_up{...} = 1` for both the cluster and the node. The counter was the only
  signal anything was wrong.
- **Run 2** (after granting the audit role): every endpoint returned `200`;
  `pve_request_errors_total = 0`.

The counter increments inside `Client.Get` (client.go:101–104) on **any**
non-200, one layer below the collector. It cannot tell a required endpoint from
an optional one, so a deliberately-restricted token pins the counter above zero
forever — a standing false-alert source — even though the three optional
endpoints already log their 403 at `debug` as "not available" and continue.

## Goal

Make `pve_request_errors_total` count only *actionable* failures:

- **Required endpoints** — any failure counts (unchanged). A 403 here means the
  exporter is blinded and the operator must know; this is the run-1 signal and
  it must be preserved.
- **Optional endpoints** — an *expected-absence* status (`403` permission
  denied, `404` feature absent) does **not** count. Every other failure (5xx,
  transport, timeout) still counts, because those are real API problems even on
  an optional endpoint.

## Non-goals

- No new labels on `pve_request_errors_total` (ADR-0006 label invariant) and no
  new metric.
- No change to log levels — the required/optional split is already expressed
  there and stays as-is.
- No change to `/cluster/resources` permission-filtering behavior (a separate
  quirk: a narrow token yields a partial cluster silently; explicitly out of
  scope).
- Decode-error counting is unchanged (it is not counted today; see below).

## Endpoint classification

The required/optional split already exists in the code, encoded in the log
level chosen at each call site:

| Endpoint | Log level today | Class | Counts failures? |
|---|---|---|---|
| `/cluster/resources` | Error (sets `up=0`) | required | yes (unchanged) |
| `/cluster/status` | Warn | required | yes (unchanged) |
| `/version` | Warn | required | yes (unchanged) |
| `/cluster/config/qdevice` | Debug | optional | 403/404 no; else yes |
| `/cluster/backup-info/not-backed-up` | Debug | optional | 403/404 no; else yes |
| `/cluster/ha/status/current` | Debug | optional | 403/404 no; else yes |
| `/nodes/*/replication` (+ `/*/status`) | Debug | optional | 403/404 no; else yes |
| `/nodes/*/subscription` | Debug | optional | 403/404 no; else yes |
| `/nodes/*/{qemu,lxc}` (+ `/*/config`) | Debug | optional | 403/404 no; else yes |

## Design (Approach A: optional-GET at the client boundary)

Add `GetOptional` beside `Get`; both delegate to a private `get(..., optional
bool)`. The only behavioral difference is the counter side-effect on a non-200:

```go
// Get fetches path and unmarshals the "data" field into out. Any non-200
// response or transport error counts toward RequestErrors.
func (c *Client) Get(ctx context.Context, path string, out interface{}) error {
    return c.get(ctx, path, out, false)
}

// GetOptional is Get for best-effort endpoints whose absence is not an error.
// A 403 (permission denied) or 404 (feature absent) is expected on restricted
// tokens or feature-less clusters and is NOT counted toward RequestErrors; all
// other failures (5xx, transport) still count. The error is still returned, so
// callers keep their existing "log debug + continue" handling.
func (c *Client) GetOptional(ctx context.Context, path string, out interface{}) error {
    return c.get(ctx, path, out, true)
}

func (c *Client) get(ctx context.Context, path string, out interface{}, optional bool) error {
    resp, err := c.http.R().SetContext(ctx).Get(path)
    if err != nil {
        c.requestErrors.Add(1) // transport failure always counts
        return fmt.Errorf("GET %s: %w", path, err)
    }
    if code := resp.StatusCode(); code != http.StatusOK {
        if !(optional && (code == http.StatusForbidden || code == http.StatusNotFound)) {
            c.requestErrors.Add(1)
        }
        return fmt.Errorf("GET %s: unexpected status %d", path, code)
    }
    // envelope decode unchanged from today (decode failures are not counted).
    ...
}
```

Key properties:

- **`GetOptional` still returns the error.** Optional callers keep their exact
  existing line (`if err != nil { log.Debug("...not available") }`); only the
  counter side-effect changes.
- **Transport errors always count**, even for optional endpoints — a network
  failure is real regardless of endpoint class. This matches today.
- **Decode errors stay uncounted** — client.go:106–116 already does not
  increment on envelope/data unmarshal failure. Identical for both methods;
  out of scope to change.
- **`GetOptional` is added to the `Doer` interface** (client.go:25), because
  `ha.go` and `node.go` (and `Collector.Client`) call through `Doer`. No test
  defines a `Doer` mock, so the interface widening breaks nothing.

## Call-site changes

Swap `Get` → `GetOptional` at the eight optional sites; leave the three
required sites on `Get`. Pure mechanical change guided by the table above — no
logic moves.

Optional → `GetOptional`:

- `internal/pve/collector.go` — `/cluster/config/qdevice`,
  `/cluster/backup-info/not-backed-up`
- `internal/pve/ha.go` — `/cluster/ha/status/current`
- `internal/pve/node.go` — `/nodes/*/replication`,
  `/nodes/*/replication/*/status`, `/nodes/*/subscription`,
  `/nodes/*/{qemu,lxc}`, `/nodes/*/{qemu,lxc}/*/config`

Required → unchanged (`Get`):

- `/cluster/resources`, `/cluster/status`, `/version`

## Testing

Using the existing `httptest` mock (canned `{"data": ...}` fixtures), add
cases that encode the policy as regression guards:

1. Optional endpoint returns **403** ⇒ its series are absent **and**
   `pve_request_errors_total == 0`.
2. Optional endpoint returns **404** ⇒ same (counter `0`).
3. Optional endpoint returns **500** ⇒ `pve_request_errors_total` **increments**
   (real failure still counts).
4. Required endpoint (`/cluster/status`) returns **403** ⇒
   `pve_request_errors_total` **increments** (preserves the run-1 signal;
   this is the case that rules out a blanket "never count 403").

Asserting the counter's value via the Prometheus scrape is sufficient for these
cases. Existing dual-backend (`CollectAndCompare` + OTLP `ManualReader`) tests
remain intact and continue to guard the "both backends read the same snapshot"
invariant.

## Documentation

- Update `docs/metrics.md` (the `pve_request_errors_total` table row at line 121
  and the lifetime-counter note at line 124) and the HELP string in
  `internal/pve/help.go` to state the counter excludes expected-absence
  (403/404) on optional endpoints.
- Add `docs/adr/0008-request-error-counting.md` recording the required-vs-optional
  counter policy and why a blanket "don't count 403" was rejected (it would
  silence a 403 on a required endpoint — the only run-1 signal). Add its line to
  `docs/adr/index.md`.

## Rejected alternatives

- **B — Typed API errors + caller-side counting.** `Get` returns a typed
  `*APIError{Status}` and stops counting itself; the collector increments based
  on its required/optional knowledge. More flexible, but decentralizes a
  currently-centralized counter across ~9 sites — more places to get wrong for
  no additional benefit here.
- **C — Globally stop counting 403 in `Get`.** One line, but wrong: it also
  silences a 403 on `/cluster/resources` or `/cluster/status`, destroying the
  run-1 signal where `pve_request_errors_total > 0` was the only indication the
  token was broken while `pve_up` still read `1`.
