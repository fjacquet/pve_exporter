# Dashboards

The repository ships a suite of six Grafana dashboards for `pve_exporter` under
`grafana/dashboards/`. They are provisioned automatically into a **Proxmox VE**
Grafana folder by the bundled Compose stack (see
[Deployment › Docker](deployment/docker.md)).

## Bring up the stack

```bash
# Local build
docker compose up -d

# Pre-built images from GHCR
docker compose -f docker-compose.ghcr.yml up -d
```

Grafana is then reachable on `http://localhost:3000`. All six dashboards appear
under the **Proxmox VE** folder and are already pointed at the Compose Prometheus
datasource — no manual import step required.

---

## Dashboard suite overview

| UID | Title | Purpose |
|-----|-------|---------|
| `pve-cluster-overview` | Cluster Overview | Landing page — cluster health, capacity, activity, and inventory |
| `pve-node` | Node Detail | Per-node stats, resource trends, and hosted guests |
| `pve-guest` | Guest Detail | Per-guest header, timeseries, HA/lock timelines, and DR strip |
| `pve-storage` | Storage | Storage inventory, used-% gauges, and usage trends |
| `pve-backup-dr` | Backup & DR | Backup coverage, replication jobs, and subscription expiry |
| `pve-ha-quorum` | HA & Quorum | Quorum state, QDevice, HA-managed guest states, node membership |

---

## Navigation and drilldown

The dashboards form a three-level hierarchy.

```
Cluster Overview
    ├── Nodes table         → Node Detail    (var-node, var-cluster)
    └── Top Guests table    → Guest Detail   (var-guest, var-cluster)
              │
         Node Detail
              └── Guests on Node table → Guest Detail (var-guest, var-cluster)
```

Every drilldown link passes the current `$cluster` variable plus the target
name as `var-node` or `var-guest`. Example URL produced by the Nodes table row:

```
/d/pve-node?var-node=${__data.fields.name}&var-cluster=${cluster}
```

Clicking any row in the **Nodes** or **Top Guests by CPU** tables on the Cluster
Overview (or the **Guests on Node** table on the Node Detail page) opens the
corresponding detail dashboard pre-filtered to that node or guest.

---

## Template variables

Every dashboard exposes the following template variables:

| Variable | Query | Scope |
|----------|-------|-------|
| `datasource` | Prometheus datasource picker | All dashboards |
| `cluster` | `label_values(pve_up, cluster)` | All dashboards |
| `node` | `label_values(pve_node_info{cluster=~"$cluster"}, name)` | Node Detail |
| `guest` | `label_values(pve_guest_info{cluster=~"$cluster"}, name)` | Guest Detail |

---

## Label and join conventions

### Identity label schema

Every `pve_exporter` metric carries a `cluster` label (the target name) and an
`id` label whose value encodes both the object type and the native Proxmox
identifier:

| `id` pattern | Object type |
|---|---|
| `node/<name>` | Proxmox node |
| `qemu/<vmid>` | QEMU virtual machine |
| `lxc/<vmid>` | LXC container |
| `storage/<node>/<store>` | Storage entry |

PromQL selectors use regex to filter by type:

```promql
# Nodes only
pve_up{id=~"node/.*", cluster=~"$cluster"}

# All guests (QEMU + LXC)
pve_up{id=~"(qemu|lxc)/.*", cluster=~"$cluster"}

# Storages only
pve_disk_usage_bytes{id=~"storage/.*", cluster=~"$cluster"}
```

### Info-metric joins

Raw `id` values are not human-readable. The `*_info` family of metrics
(`pve_guest_info`, `pve_node_info`, `pve_storage_info`) carry friendly labels
such as `name`, `node`, `tags`, and `storage`. Panels join these with
`* on(cluster,id) group_left(...)` to expose friendly names while grouping on
the native `id`:

```promql
# CPU usage per node, labelled by name
avg by (id, name) (
  pve_cpu_usage_ratio{id=~"node/.*", cluster=~"$cluster"}
  * on(cluster,id) group_left(name)
  pve_node_info
)

# Top-10 guests by CPU, labelled by name and node
topk(10,
  pve_cpu_usage_ratio{id=~"(qemu|lxc)/.*", cluster=~"$cluster"}
  * on(cluster,id) group_left(name, node)
  pve_guest_info
)
```

---

## PromQL aggregation rules

### Gauges — never use `rate()`

CPU ratio, memory/disk bytes, state enum values, and `pve_cpu_usage_limit` are
instantaneous gauges. Aggregate them with `sum()`, `avg()`, or `max()`:

```promql
# Memory used % per node
(
  pve_memory_usage_bytes{id=~"node/.*", cluster=~"$cluster"}
  / pve_memory_size_bytes{id=~"node/.*", cluster=~"$cluster"}
) * 100

# CPU overcommit ratio (total guest vCPU / total node CPU)
sum(pve_cpu_usage_limit{id=~"(qemu|lxc)/.*", cluster=~"$cluster"})
  / sum(pve_cpu_usage_limit{id=~"node/.*", cluster=~"$cluster"})
```

### Counters — apply `rate()` first

Only the four `*_bytes_total` counters use `rate()`. These are applied before
any aggregation:

```promql
# Cluster-wide network receive throughput
sum(rate(pve_network_receive_bytes_total{cluster=~"$cluster"}[5m]))

# Per-guest disk I/O read rate
avg by (id, name) (
  rate(pve_disk_read_bytes_total{id=~"(qemu|lxc)/.*", cluster=~"$cluster"}[5m])
  * on(cluster,id) group_left(name)
  pve_guest_info
)
```

!!! warning "Mixing gauges and counters"
    `pve_cpu_usage_ratio`, `pve_memory_usage_bytes`, `pve_disk_usage_bytes`,
    and `pve_memory_size_bytes` are **gauges**. Wrapping them in `rate()` yields
    nonsense. Only metrics ending in `_total` are counters. See
    [ADR 0005](adr/0005-naming-units.md) for naming conventions.

---

## State Timeline panels

`pve_ha_state`, `pve_lock_state`, and `pve_subscription_status` are enum gauges
— each value maps to a named state. The dashboards render them as **State
Timeline** panels to show state transitions over the selected time window, paired
with a **current-state table** that shows the live value for each object.

The enum values and colour mappings are defined as value mappings inside each
panel's field configuration in the dashboard JSON.

---

## Dashboard reference

### 1. Cluster Overview (`pve-cluster-overview`)

The landing page for a cluster. Use the `cluster` variable to select a target.

**Health & Status row** — six stat panels:

| Panel | PromQL summary |
|-------|----------------|
| Nodes Online | `sum(pve_up{id=~"node/.*"})` |
| Guests Running | `sum(pve_up{id=~"(qemu|lxc)/.*"})` |
| Storages Available | `sum(pve_up{id=~"storage/.*"})` |
| Replication Failures | `sum(pve_replication_failed_syncs)` |
| Guests Not Backed Up | `sum(pve_not_backed_up_total)` |
| Subscription Active | `min(pve_subscription_status{status="active"})` |

**Capacity row** — three bar-gauge / stat panels:

| Panel | What it shows |
|-------|---------------|
| Memory Used % (per node) | `pve_memory_usage_bytes / pve_memory_size_bytes` per node, as a horizontal bar-gauge |
| Storage Used % | `pve_disk_usage_bytes / pve_disk_size_bytes` per storage |
| CPU Overcommit | Ratio of total guest vCPU allocations to total node CPU cores |

**Activity row** — four timeseries panels (CPU, memory, network I/O, disk I/O).
Network and disk panels use `rate()` over the `*_bytes_total` counters; CPU and
memory panels do not.

**Inventory row** — two tables:

- **Nodes** — one row per node; clicking drills down to Node Detail.
- **Top Guests by CPU** — top 10 guests by current CPU ratio; clicking drills
  down to Guest Detail.

---

### 2. Node Detail (`pve-node`)

Selected by the `$node` variable (populated from `pve_node_info.name` for the
chosen cluster).

**Node Status row** — ten stat panels including: Node Up, CPU Usage, Memory
Used, Memory Total, Root Disk Used %, Uptime, Guest Count, Subscription Days to
Due, and PVE Version. Each joins against `pve_node_info` to filter by `$node`.

**Activity row** — four timeseries panels:

| Panel | Metric(s) |
|-------|-----------|
| CPU Usage | `pve_cpu_usage_ratio` (gauge, no `rate()`) |
| Memory Used vs Total | `pve_memory_usage_bytes` + `pve_memory_size_bytes` |
| Root Disk Usage | `pve_disk_usage_bytes` (node scope) |
| Network I/O | `rate(pve_network_receive/transmit_bytes_total[5m])` |

**Guests on Node row** — a table of all guests on this node, sourced from
`pve_guest_info{node=~"$node"}`. Each row links to Guest Detail via
`var-guest=${__data.fields.name}&var-cluster=${cluster}`.

---

### 3. Guest Detail (`pve-guest`)

Selected by the `$guest` variable (populated from `pve_guest_info.name`).

**Guest Status row** — eleven stat panels: Guest Running, Guest Name, Node,
Guest Type, Tags, Template, On Boot, CPU Limit, Current HA State, and Current
Lock State. Most panels join against `pve_guest_info{name=~"$guest"}` to get
human-readable labels.

**Activity row** — four timeseries panels:

| Panel | Metric(s) |
|-------|-----------|
| CPU Usage | `pve_cpu_usage_ratio` (gauge) |
| Memory Used vs Total | `pve_memory_usage_bytes` + `pve_memory_size_bytes` |
| Disk I/O | `rate(pve_disk_read/written_bytes_total[5m])` |
| Network I/O | `rate(pve_network_receive/transmit_bytes_total[5m])` |

**HA & Lock State History row** — two State Timeline panels:

- **HA State History**: `pve_ha_state` for the selected guest over the time range.
- **Lock State History**: `pve_lock_state` for the selected guest.

Both join `pve_guest_info` via `* on(cluster,id) group_left(name)` to label
series by guest name.

**Disaster Recovery row** — two panels:

- **Backup Status**: stat showing whether the guest appears in
  `pve_not_backed_up_info` (1 = not backed up, 0 = covered).
- **Replication Jobs**: table joining `pve_replication_info` with
  `pve_guest_info` on `(cluster,guest)` to show last-sync timestamps, next-sync
  countdown, and failed-sync count.

---

### 4. Storage (`pve-storage`)

Cluster-wide view of all storage targets.

**Summary row** — three stat panels: Total Capacity, Total Used, Shared Storages
(count of `pve_storage_shared == 1`).

**Storage Inventory row** — a table of all storages, joined with
`pve_storage_info` to expose the `storage` and `node` labels alongside raw byte
values.

**Usage row** — bar-gauge of used % per storage (`pve_disk_usage_bytes /
pve_disk_size_bytes`).

**Trends row** — timeseries of `pve_disk_usage_bytes` and `pve_disk_size_bytes`
per storage, joined with `pve_storage_info` for the human-readable `storage`
label.

---

### 5. Backup & DR (`pve-backup-dr`)

Cluster-wide view of backup coverage, replication health, and subscription
status.

**Backup Status row**:

- **Not Backed Up** stat: `sum(pve_not_backed_up_total)` — a non-zero value
  means at least one guest has no scheduled backup.
- **Guests Not Backed Up** table: lists affected guests by joining
  `pve_not_backed_up_info` with `pve_guest_info` on `(cluster,id)` to expose
  `name` and `node`.

**Replication row**:

| Panel | What it shows |
|-------|---------------|
| Failed Syncs Total | `sum(pve_replication_failed_syncs)` |
| Max Replication Age | `max(time() - pve_replication_last_sync_timestamp_seconds)` |
| Replication Jobs table | All replication jobs with last-sync age, next-sync countdown (`pve_replication_next_sync_timestamp_seconds - time()`), and failed sync count |
| Replication Last Sync Age | Timeseries of sync age per job |

**Subscription row** — **Subscription Expiry** table: shows days until each
node's subscription expires, computed as
`(pve_subscription_next_due_timestamp_seconds - time()) / 86400`, joined with
`pve_node_info` to show the node name.

---

### 6. HA & Quorum (`pve-ha-quorum`)

Quorum health, QDevice state, and HA-managed guest inventory.

**Cluster Status row**:

- **Nodes Online** stat: `sum(pve_up{id=~"node/.*"})`.
- **QDevice Up** stat: `sum(pve_qdevice_up)` (falls back to `vector(0)` when no
  QDevice is configured).
- **Cluster Info** table: sourced from `pve_cluster_info`, exposing cluster-level
  labels such as `quorate` and `version`.

**QDevice row**:

- **QDevice Info** table: sourced from `pve_qdevice_info`, showing QDevice
  connection details.

**High Availability row**:

- **HA-Managed Guest States** State Timeline: `pve_ha_state` for all guests,
  joined with `pve_guest_info` to label series by name.
- **Current HA State per Guest** table: `pve_ha_state == 1` (active/started
  filter) joined with `pve_guest_info{name, node}`.

**Node Membership row**:

- **Node Membership** table: sourced from `pve_node_info`, listing all known
  nodes and their labels.

---

## Known gaps

The following data points are not yet collected by the exporter and therefore
appear as empty or absent in the dashboards:

- **Node-level HA state** — `pve_ha_state` is only collected for guests
  (`id=~"(qemu|lxc)/.*"`). A node-scope HA state panel would require a separate
  exporter endpoint (tracked as an exporter follow-up).
- **QDevice `tie_breaker` and `state` labels** — `pve_qdevice_info` currently
  carries connection metadata but not the live quorum tie-breaker role or
  per-vote state; those fields are exporter follow-ups.

---

## Provisioning

Dashboards are provisioned automatically from `grafana/dashboards/` via the
`grafana/provisioning/dashboards/pve.yaml` provider configuration. No manual
import is required when using the Compose stack.

To add or update a dashboard, place or replace the JSON file in
`grafana/dashboards/` and restart the Grafana container:

```bash
docker compose restart grafana
```

The dashboard JSON files must pass the offline regression suite before being
committed:

```bash
python3 tests/dashboards/validate_dashboards.py
```

This checks that every dashboard parses as valid JSON, uses only known `pve_*`
metric names, does not reference the non-existent `type` label, and sets
`${datasource}` on every non-row panel.
