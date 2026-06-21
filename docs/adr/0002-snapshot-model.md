# ADR 0002 — Background snapshot collection model

- **Status:** Accepted
- **Date:** 2026-06-21
- **Deciders:** Frederic Jacquet

## Context

Two backends consume the same PVE data: a Prometheus collector (pulled on each
scrape) and an OTLP exporter (observable gauges, read on each export cycle). The
naive design — query the PVE API synchronously inside each scrape — has three
problems:

1. **Latency coupling.** A slow or unreachable PVE API stalls the Prometheus
   scrape, risking scrape timeouts and gaps.
2. **Inconsistency.** If Prometheus and OTLP each query the API independently, the
   two backends can report different values for the same instant.
3. **Cold start.** Scraping on-demand means `/metrics` blocks until the first PVE
   round-trip completes, so the process looks unhealthy at boot.

## Decision

Run a **background collection loop**. On `collection.interval`, for each target,
the loop queries the PVE API, builds an **immutable snapshot** value, and stores it
via an **`RWMutex`-guarded pointer-swap** into a shared store. Reads take the read
lock, copy the current pointer, and release — they never block on the API.

Both backends read the **same snapshot**:

- The Prometheus collector is an **unchecked collector** that, on `Collect()`,
  reads the current snapshot and emits its series.
- The OTLP **observable gauges** read the current snapshot in their callbacks.

The **HTTP server starts before the first collection**. Until the first snapshot
lands, the store returns an empty snapshot, so `/metrics` is reachable immediately
and the process is healthy from boot.

## Consequences

**Positive**

- Scrapes are O(1) memory reads — never blocked by PVE API latency.
- Prometheus and OTLP are always consistent: they read the identical snapshot.
- Immutable snapshots + pointer-swap means no torn reads and no per-series locking.
- The process is healthy at boot; collection failures degrade gracefully (stale or
  empty snapshot) rather than failing scrapes.

**Negative / trade-offs**

- Metrics are as fresh as `collection.interval`, not as fresh as the scrape.
- A target unreachable since boot exposes an empty/partial snapshot — consumers
  must treat absent series as "not reported" (see
  [ADR 0006](0006-label-invariant.md)).

## Alternatives considered

- **Synchronous scrape-time collection** — rejected for the latency coupling and
  Prometheus/OTLP inconsistency above.
- **Per-metric locking instead of pointer-swap** — rejected; more lock contention
  and complexity than swapping one immutable pointer.
