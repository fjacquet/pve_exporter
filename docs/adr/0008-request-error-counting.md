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
