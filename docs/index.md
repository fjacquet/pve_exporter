# Proxmox VE Exporter

A Prometheus + OTLP exporter for Proxmox VE (PVE) clusters, written in Go.

`pve_exporter` scrapes the Proxmox VE REST API and exposes cluster, node, guest
(QEMU/LXC), storage, replication, backup, subscription and quorum-device metrics
to both **Prometheus** (`/metrics`) and any **OTLP**-compatible collector. It speaks
to one or many clusters from a single process, using read-only PVE API tokens.

## Features

- **Multi-cluster** — poll any number of PVE clusters/hosts from one binary; every
  series carries a `cluster` identity label (the target name).
- **Dual export** — the same in-memory snapshot feeds a Prometheus unchecked
  collector and OTLP observable gauges, so the two backends never disagree.
- **Snapshot collection model** — a background loop polls each target on a fixed
  interval, builds an immutable snapshot and atomically swaps it into a store; the
  HTTP server starts and answers `/metrics` *before* the first collection completes.
- **Token-only auth** — static `Authorization: PVEAPIToken=...` header, no login
  round-trip and no token refresh. The token secret is never logged.
- **Hot-reloadable config** — YAML with `${ENV_VAR}` expansion and `.env`
  autoload; reloads on `SIGHUP` and on config-file change.
- **Proxmox-native identities** — every metric carries the native `id` label
  (`node/<name>`, `qemu/<vmid>`, `lxc/<vmid>`, `storage/<node>/<store>`,
  `cluster/<name>`, replication `<jobid>`).

## Architecture

A background collection loop polls each configured target on
`collection.interval`. For each target it builds an **immutable snapshot** from the
PVE API responses and atomically swaps it into a shared store behind an `RWMutex`
pointer-swap. Both read paths — the Prometheus *unchecked collector* and the OTLP
*observable gauges* — read the current snapshot, so the two backends are always
consistent.

```
                         ┌────────────────────────┐
   PVE API  ◀── GET ─────┤  collector loop        │
 (resty/v2)              │  (per target, interval)│
                         └───────────┬────────────┘
                                     │ build + swap
                                     ▼
                          ┌────────────────────┐
                          │  snapshot store    │  (RWMutex pointer-swap)
                          └────┬───────────┬───┘
                               │           │
                  Prometheus ◀─┘           └─▶ OTLP observable gauges
                  /metrics                     (OTLP exporter)
```

The HTTP server starts **before** the first collection, so `/metrics` is reachable
immediately and the process is healthy from boot.

## Scope

The exporter is **read-only**. It requires a PVE API token with the built-in
**PVEAuditor** role granted at path `/`. It scrapes `/cluster/resources`,
`/cluster/status`, `/version`, `/cluster/config/qdevice`,
`/cluster/backup-info/not-backed-up`, and per-node replication, subscription and
guest-config endpoints. It performs no writes and holds no PVE session state.

See [Metrics Reference](metrics.md) for the full catalogue, or
[Deployment › Docker](deployment/docker.md) to get started.
