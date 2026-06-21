# ADR 0006 — Label-set invariant per metric name

- **Status:** Accepted
- **Date:** 2026-06-21
- **Deciders:** Frederic Jacquet

## Context

Prometheus requires that a given metric name expose a **consistent set of label
keys**. Two failure modes break this:

1. **State-enum metrics.** A metric like `pve_ha_state` has a `state` label. If we
   emitted only the *current* state, the set of `state` values would change over
   time, and a query like `pve_ha_state{state="error"}` would silently return no
   data when the guest is healthy — indistinguishable from "guest not found".
2. **Absent-vs-zero.** The PVE API omits fields that don't apply (e.g. a stopped
   guest has no CPU usage). Emitting `0` for an absent field invents data and makes
   "really zero" indistinguishable from "not reported".

## Decision

### 1. One label-key set per metric name

Every series of a given metric name carries exactly the same label **keys**. New
optional dimensions get their own metric name rather than a sometimes-present label.

### 2. State-enum metrics emit all states with 0/1

`pve_ha_state`, `pve_lock_state` and `pve_subscription_status` emit **one series
per possible state value**, each `0` or `1`. The current state is `1`, the rest are
`0`. The `state`/`status` label key set is therefore constant, and
`pve_ha_state{state="error"} == 1` is a reliable alerting condition.

### 3. Absent-not-zero parsing

When the PVE API omits a field, the corresponding **series is omitted** rather than
reported as `0`. Because we model only the fields we consume (see
[ADR 0003](0003-client-choice.md)), a missing JSON key naturally yields no series.
Consumers treat absent series as "not reported", not as the value `0`.

## Consequences

**Positive**

- Stable label-key sets: no "phantom disappearance" of series, safe enum alerting.
- No invented zeros — `absent()` / missing-series semantics work as intended.
- Cleaner cardinality story: optional dimensions are separate metrics, not
  optional labels.

**Negative / trade-offs**

- State-enum metrics emit N series per object (one per possible state), increasing
  cardinality versus a single current-state series.
- Consumers must understand absent-not-zero — a stopped guest's
  `pve_cpu_usage_ratio` is missing, not `0`. Dashboards may need `or vector(0)` to
  display a default.

## Alternatives considered

- **Emit only the current state for enum metrics** — rejected; breaks the
  label-key invariant and makes enum alerting unreliable.
- **Emit `0` for absent fields** — rejected; conflates "really zero" with "not
  reported".
