# Dashboards

The repository ships a Grafana dashboard for `pve_exporter` under `grafana/`. It
is provisioned automatically by the bundled Compose stack (see
[Deployment › Docker](deployment/docker.md)).

## Run

```bash
docker compose up -d
```

Grafana is then reachable on `http://localhost:3000` with the PVE dashboard
pre-loaded and pointed at the Compose Prometheus.

## Layout & panel types

The dashboard is organised by object scope — cluster, nodes, guests, storage — and
filtered by a `cluster` template variable (the target name) plus an `id`/`node`
variable for drill-down.

| Panel group     | Source metrics                                                       | Aggregation                              |
| --------------- | -------------------------------------------------------------------- | ---------------------------------------- |
| Cluster health  | `pve_up`, `pve_cluster_info`, `pve_qdevice_up`                       | `max`/`min` by `cluster`                 |
| Node resources  | `pve_cpu_usage_ratio`, `pve_memory_usage_bytes`, `pve_disk_usage_bytes` | `avg`/`sum` by `id`                  |
| Guest inventory | `pve_guest_info`, `pve_onboot_status`                                | table / `count` by `type`                |
| Storage         | `pve_disk_size_bytes`, `pve_disk_usage_bytes`, `pve_storage_info`    | `sum` by `id`                            |
| Throughput      | `pve_network_*_bytes_total`, `pve_disk_{read,written}_bytes_total`   | `rate()` then `sum` by `id`              |
| Replication     | `pve_replication_*`                                                  | timestamp panels, `pve_replication_failed_syncs` |
| Backup coverage | `pve_not_backed_up_total`, `pve_not_backed_up_info`                  | stat / table                             |

## PromQL conventions

**Gauges** (sizes, usage, ratios) are instantaneous — aggregate with `sum()` /
`avg()`, never `rate()`:

```promql
# Average CPU usage per node in a cluster
avg by (id) (pve_cpu_usage_ratio{cluster="prod", id=~"node/.*"})

# Total used storage per cluster
sum by (cluster) (pve_disk_usage_bytes{id=~"storage/.*"})
```

**Counters** (`_total`) — apply `rate()` / `increase()` first, then aggregate:

```promql
# Network receive throughput per guest
sum by (id) (rate(pve_network_receive_bytes_total{cluster="prod"}[5m]))
```

!!! warning "Counters vs. gauges"
    `pve_cpu_usage_ratio` and the `*_seconds` / `*_bytes` (non-`_total`) series are
    gauges. Only the `*_total` metrics are counters. Mixing the two up (e.g.
    `rate()` over a gauge) yields nonsense. See
    [ADR 0005](adr/0005-naming-units.md).
