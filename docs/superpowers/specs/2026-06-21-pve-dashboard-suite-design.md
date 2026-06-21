# Design — Grafana dashboard suite for pve_exporter

**Date:** 2026-06-21
**Status:** Approved (brainstorming) — ready for implementation plan

## Context

The exporter ships a single `grafana/pve-overview.json`. Two problems prompted this work:

1. **It is broken against real data.** Every panel filters on a `type` label
   (`pve_up{type="node"}`, `{type=~"qemu|lxc"}`, `{type="storage"}`), but the
   exporter emits **no `type` label** — the resource type lives inside the `id`
   (`node/proxmox`, `qemu/100`, `storage/proxmox/local`). Those panels return no data.
2. **It under-uses the metric set.** We collect subscription status/expiry, backup
   gaps, HA/lock state, replication freshness, qdevice health, uptime, per-guest
   counters, and rich `*_info` metadata — almost none of it is visualized.

Outcome: replace the single dashboard with a **six-dashboard suite** that is correct
(real label schema + friendly names) and surfaces the full metric set, targeting
**Grafana 11+** (schemaVersion 39).

## Label schema (ground truth)

Every series carries `cluster` (identity) + an `id`. Type is encoded in `id`:
`node/<name>`, `qemu/<vmid>`, `lxc/<vmid>`, `storage/<node>/<store>`, `cluster/<name>`,
replication job `<jobid>` (e.g. `1-0`). There is **no `type` label**.

## Technical foundation (applies to all dashboards)

- **Select by `id` regex:** nodes `id=~"node/.*"`, guests `id=~"(qemu|lxc)/.*"`,
  storage `id=~"storage/.*"`.
- **Friendly names via PromQL `group_left` joins** against the `*_info` metrics:
  ```promql
  pve_cpu_usage_ratio{id=~"(qemu|lxc)/.*"}
    * on(cluster,id) group_left(name,node,type,tags) pve_guest_info
  ```
  Analogous joins use `pve_node_info` (name, level, nodeid) and `pve_storage_info`
  (storage, plugintype, content, node). This is the highest-impact change — every
  table/timeseries shows real names/tags instead of raw ids.
- **Chained template variables:** `datasource`, `cluster` =
  `label_values(pve_up, cluster)`, `node` =
  `label_values(pve_node_info{cluster=~"$cluster"}, name)`, `guest` =
  `label_values(pve_guest_info{cluster=~"$cluster"}, name)`.
- **Drilldown:** data links on Overview/Node tables open the Node/Guest detail
  dashboard with `$cluster`/`$node`/`$guest` pre-filled.
- **Aggregation (family standard):** gauges (cpu ratio, memory/disk bytes, `*_info`,
  state enums) aggregate with `sum`/`avg` — never `rate()`. Only the four
  `*_bytes_total` counters use `rate(...[5m])`. Percentages = `usage / size`.
- **State enums** (`pve_ha_state`, `pve_lock_state`, `pve_subscription_status`) emit
  all states with one = 1 → render with **State Timeline** (history) and a current
  table via `<metric> == 1` reading the `state`/`status` label.
- **Datasource:** every panel/target references the `$datasource` variable (provisioned).

## Dashboard inventory

### 1. Cluster Overview (landing, `grafana/dashboards/pve-cluster-overview.json`)
- Health stats: nodes up/total, guests running/total, cluster quorate
  (`pve_cluster_info`), storages available, replication failures
  (`sum(pve_replication_failed_syncs)`), guests not-backed-up
  (`pve_not_backed_up_total`), subscription active.
- Capacity bar-gauges: memory used vs total (nodes), storage used % (top storages),
  CPU overcommit (Σ guest `cpu_usage_limit` ÷ Σ node `cpu_usage_limit`).
- Activity timeseries: node CPU avg, memory used/total, cluster network & disk I/O
  (rate of guest counters).
- Inventory tables: nodes (name, status, cpu %, mem %, uptime) and top guests by
  cpu/mem — rows data-link to Node/Guest detail.

### 2. Node detail (`$node`, `pve-node.json`)
- Header stats: up, cpu %, mem used/total, root-disk %, uptime, #guests, PVE version
  (`pve_version_info`), subscription status + days-to-due
  (`(pve_subscription_next_due_timestamp_seconds - time())/86400`).
- Timeseries: CPU ratio, memory used vs size, root disk usage.
- Guests-on-node table: `pve_guest_info{node="$node"}` joined with running state, cpu %,
  mem %, disk, uptime, onboot (`pve_onboot_status`), tags, ha_state → drilldown to Guest detail.

### 3. Guest detail (`$guest`, drilldown target, `pve-guest.json`)
- Header: running, name, node, type, tags, template, onboot, cpu limit, current
  ha_state, current lock.
- Timeseries: CPU, memory used vs size, disk read/write rate, network rx/tx rate.
- State Timelines: ha_state history, lock_state history.
- DR strip: backup coverage (`pve_not_backed_up_info`), replication jobs for this guest
  (`pve_replication_info{guest=...}` + last_sync age, failed_syncs).

### 4. Storage (`pve-storage.json`)
- Table: all storages (name, node, plugintype, content, shared, used, size, used %)
  via `pve_storage_info` join.
- Bar-gauge: used % per storage (thresholds 80/90). Timeseries: used vs size per
  storage. Stats: total capacity/used; shared (`pve_storage_shared`) vs local split.

### 5. Backup & DR (`pve-backup-dr.json`)
- Not-backed-up: total stat + table (names via `guest_info` join).
- Replication table: job id, guest→name, source, target, type, last_sync age
  (`time() - pve_replication_last_sync_timestamp_seconds`), duration, failed_syncs,
  next_sync countdown. Timeseries: last_sync age per job. Stats: failed total, max age.
- Subscription expiry countdown table
  (`pve_subscription_next_due_timestamp_seconds - time()`).

### 6. HA & Quorum (`pve-ha-quorum.json`)
- Quorum stats: quorate, nodes configured (`pve_cluster_info` nodes) vs online
  (`count(pve_up{id=~"node/.*"} == 1)`). QDevice: `pve_qdevice_up` + `pve_qdevice_info` table.
- State Timeline: HA-managed guest states. Table: current HA state per guest
  (`pve_ha_state == 1` → `state` label, names joined). Node membership table
  (`pve_node_info`: name, nodeid, level; online via `pve_up`).

## Files & provisioning

- Six JSONs under `grafana/dashboards/` in a "Proxmox VE" folder, loaded by the existing
  provider (`grafana/provisioning/dashboards/dashboards.yml`, `foldersFromFilesStructure: true`).
- **Remove** the broken `grafana/pve-overview.json` (replaced by the suite).
- Update `docs/dashboards.md` to document the six dashboards, the join pattern, and
  drilldown navigation. Update the docker-compose mount path if needed so
  `grafana/dashboards/` is served.

## Out of scope (logged follow-ups — require new *metrics*, not dashboards)

These two panels are intentionally limited by current collector coverage; tracked as an
optional exporter enhancement, not part of this dashboard work:
- **Node-level `pve_ha_state`** — needs `/cluster/ha/status/current`; today only guest
  `hastate` is emitted. The HA dashboard shows guest HA state only.
- **QDevice `tie_breaker`/`state`** — collector populates only model/host/algorithm;
  full corosync fields need `/cluster/ha/status` or corosync parsing.

## Testing / validation

- `python -c "import json"` (or `jq`) parses every dashboard JSON.
- Bring up the compose stack (`docker-compose.yml`) against a real PVE (or load the
  sample exposition into Prometheus) and confirm each dashboard renders with data — no
  empty panels caused by label/selector mistakes.
- Spot-check representative PromQL (`group_left` joins resolve names; counters use
  `rate`, gauges do not) with `promtool query instant` or the Grafana Explore view.
- Verify drilldown links carry `$cluster`/`$node`/`$guest` between dashboards.
- Confirm all panels use the `$datasource` variable (no hard-coded datasource uid).
