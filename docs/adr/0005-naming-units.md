# ADR 0005 — Metric naming & units

- **Status:** Accepted
- **Date:** 2026-06-21
- **Deciders:** Frederic Jacquet

## Context

Operators already run the Python community
[`prometheus-pve-exporter`](https://github.com/prometheus-pve/prometheus-pve-exporter)
and have dashboards and alerts built on its metric names. A drop-in-ish naming
scheme lowers migration cost. At the same time, the exporter must follow Prometheus
naming/units conventions and avoid the community exporter's deprecated aliases.

## Decision

### 1. Match the community `pve_` names

Metric names mirror the community exporter (`pve_up`, `pve_cpu_usage_ratio`,
`pve_memory_usage_bytes`, `pve_disk_size_bytes`, …) so existing queries port over
with minimal change.

### 2. Per-second / ratio values are gauges — aggregate, don't `rate()`

`pve_cpu_usage_ratio` and the `*_seconds` timestamp/duration metrics are
**instantaneous gauges**. Aggregate them with `sum()` / `avg()`. Applying `rate()`
to them is a category error. Only the `*_total` metrics are counters.

### 3. Cluster identity label

Every series carries a `cluster` label (the target name from `clusters[].name`),
in addition to the Proxmox-native `id` label. This lets a single exporter front
many clusters without label collisions.

### 4. Drop the deprecated gauge aliases

The community exporter emits deprecated non-`_total` gauge aliases alongside its
counters. We emit **only the `_total` counters** and drop the aliases:

| Dropped alias                  | Use instead                          |
| ------------------------------ | ------------------------------------ |
| `pve_network_transmit_bytes`   | `pve_network_transmit_bytes_total`   |
| `pve_network_receive_bytes`    | `pve_network_receive_bytes_total`    |
| `pve_disk_write_bytes`         | `pve_disk_written_bytes_total`       |
| `pve_disk_read_bytes`          | `pve_disk_read_bytes_total`          |

## Consequences

**Positive**

- Low migration cost from the community exporter for most metrics.
- Correct Prometheus semantics: counters carry `_total` and are the only series you
  `rate()`; gauges are aggregated.
- The `cluster` label makes multi-cluster scraping unambiguous.

**Negative / trade-offs**

- Dashboards relying on the dropped aliases must switch to `rate(..._total)`.
- The extra `cluster` label means queries copied verbatim from the community
  exporter may need a `cluster=` selector.

## Alternatives considered

- **Emit the deprecated aliases for full drop-in compatibility** — rejected;
  perpetuates double-counting risk and non-conventional gauge counters.
- **Invent a fresh naming scheme** — rejected; needless migration cost.
