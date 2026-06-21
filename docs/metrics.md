# Metrics Reference

All metrics are prefixed `pve_` and exposed on `http://<host>:9221/metrics`. The
same series are also emitted over OTLP as observable gauges (counters as monotonic
sums).

## Label conventions

Every metric carries two identity labels:

| Label     | Meaning                                                                        |
| --------- | ------------------------------------------------------------------------------ |
| `cluster` | The target name from `clusters[].name` in the config — your cluster identity.  |
| `id`      | The Proxmox-native object id (see below).                                       |

`id` values follow the Proxmox `/cluster/resources` convention:

| Object      | `id` form                | Example              |
| ----------- | ------------------------ | -------------------- |
| Node        | `node/<name>`            | `node/pve-a`         |
| QEMU VM     | `qemu/<vmid>`            | `qemu/100`           |
| LXC CT      | `lxc/<vmid>`             | `lxc/200`            |
| Storage     | `storage/<node>/<store>` | `storage/pve-a/local`|
| Cluster     | `cluster/<name>`         | `cluster/prod`       |
| Replication | `<jobid>`                | `1-0`                |

!!! warning "Authoritative source"
    The Proxmox API is the single source of truth. When a field is absent from the
    API response, the corresponding series is **omitted** rather than reported as
    zero (see [ADR 0006](adr/0006-label-invariant.md)). Treat missing series as
    "not reported", not as "value 0".

## State / info metrics

| Metric              | Labels                                                  | HELP                                                                |
| ------------------- | ------------------------------------------------------- | ------------------------------------------------------------------- |
| `pve_up`            | `cluster`, `id`                                         | Node/VM/CT status is online/running (also a `cluster/<name>` series for target scrape health). |
| `pve_guest_info`    | `cluster`, `id`, `node`, `name`, `type`, `template`, `tags` | Guest (QEMU/LXC) metadata.                                     |
| `pve_storage_info`  | `cluster`, `id`, `node`, `storage`, `plugintype`, `content` | Storage metadata.                                             |
| `pve_node_info`     | `cluster`, `id`, `name`, `level`, `nodeid`              | Node metadata.                                                      |
| `pve_cluster_info`  | `cluster`, `id`, `nodes`, `quorate`, `version`          | Cluster metadata.                                                   |
| `pve_version_info`  | `cluster`, `release`, `repoid`, `version`               | PVE version of the target.                                          |
| `pve_storage_shared`| `cluster`, `id`                                         | Whether the storage is shared (1) or local (0).                     |

### State-enum metrics

These emit **one series per possible state**, each `0` or `1` — so the set of
label keys stays stable regardless of the current value (see
[ADR 0006](adr/0006-label-invariant.md)).

| Metric                     | Labels                          | HELP                                          |
| -------------------------- | ------------------------------- | --------------------------------------------- |
| `pve_ha_state`             | `cluster`, `id`, `state`        | HA manager state of the guest/node.           |
| `pve_lock_state`           | `cluster`, `id`, `state`        | Guest lock state.                             |
| `pve_subscription_status`  | `cluster`, `id`, `status`       | Subscription status (active/notfound/...).    |

## Resource metrics (gauges)

| Metric                   | Labels             | HELP                                           |
| ------------------------ | ------------------ | ---------------------------------------------- |
| `pve_disk_size_bytes`    | `cluster`, `id`    | Total disk/storage size in bytes.              |
| `pve_disk_usage_bytes`   | `cluster`, `id`    | Used disk/storage in bytes.                    |
| `pve_memory_size_bytes`  | `cluster`, `id`    | Total memory in bytes.                         |
| `pve_memory_usage_bytes` | `cluster`, `id`    | Used memory in bytes.                          |
| `pve_cpu_usage_ratio`    | `cluster`, `id`    | CPU usage as a ratio (0–1).                    |
| `pve_cpu_usage_limit`    | `cluster`, `id`    | Number of CPUs assigned (the CPU limit).       |
| `pve_uptime_seconds`     | `cluster`, `id`    | Uptime in seconds.                             |

!!! warning "Do not use rate()"
    `pve_cpu_usage_ratio` is an instantaneous gauge already expressed per-second /
    as a ratio. Aggregate it with `sum()` / `avg()`, **never** `rate()` (see
    [ADR 0005](adr/0005-naming-units.md)).

## Counter metrics

Monotonic counters. Use `rate()` / `increase()` over these.

| Metric                              | Labels             | HELP                                  |
| ----------------------------------- | ------------------ | ------------------------------------- |
| `pve_network_receive_bytes_total`   | `cluster`, `id`    | Total bytes received on the guest/node.  |
| `pve_network_transmit_bytes_total`  | `cluster`, `id`    | Total bytes transmitted.              |
| `pve_disk_read_bytes_total`         | `cluster`, `id`    | Total bytes read from disk.           |
| `pve_disk_written_bytes_total`      | `cluster`, `id`    | Total bytes written to disk.          |

## Subscription metrics

| Metric                                          | Labels                       | HELP                                       |
| ----------------------------------------------- | ---------------------------- | ------------------------------------------ |
| `pve_subscription_info`                         | `cluster`, `id`, `level`     | Subscription metadata (per node).          |
| `pve_subscription_status`                       | `cluster`, `id`, `status`    | Subscription status enum (0/1 per state).  |
| `pve_subscription_next_due_timestamp_seconds`   | `cluster`, `id`              | Unix timestamp of the next due date.       |

## Replication metrics

| Metric                                              | Labels                                              | HELP                                          |
| --------------------------------------------------- | --------------------------------------------------- | --------------------------------------------- |
| `pve_replication_info`                              | `cluster`, `id`, `guest`, `source`, `target`, `type`| Replication job metadata.                     |
| `pve_replication_duration_seconds`                  | `cluster`, `id`                                     | Duration of the last replication run.         |
| `pve_replication_last_sync_timestamp_seconds`       | `cluster`, `id`                                     | Unix timestamp of the last successful sync.   |
| `pve_replication_last_try_timestamp_seconds`        | `cluster`, `id`                                     | Unix timestamp of the last attempted sync.    |
| `pve_replication_next_sync_timestamp_seconds`       | `cluster`, `id`                                     | Unix timestamp of the next scheduled sync.    |
| `pve_replication_failed_syncs`                      | `cluster`, `id`                                     | Number of failed sync attempts.               |

## Backup & quorum metrics

| Metric                      | Labels                                         | HELP                                              |
| --------------------------- | ---------------------------------------------- | ------------------------------------------------- |
| `pve_not_backed_up_total`   | `cluster`, `id`                                | Count of guests not covered by any backup job.    |
| `pve_not_backed_up_info`    | `cluster`, `id`                                | Per-guest marker that the guest is not backed up. |
| `pve_qdevice_up`            | `cluster`, `id`                                | Whether the cluster quorum device is up.          |
| `pve_qdevice_info`          | `cluster`, `id`, `model`, `host`, `algorithm`  | Quorum-device metadata.                           |
| `pve_onboot_status`         | `cluster`, `id`, `node`, `type`                | Whether the guest is configured to start on boot. |

## Divergences from the Python community exporter

This exporter deliberately differs from the Python community
[`prometheus-pve-exporter`](https://github.com/prometheus-pve/prometheus-pve-exporter)
in two places:

- **Dropped deprecated gauge aliases.** The deprecated non-`_total` gauge aliases
  — `pve_network_transmit_bytes`, `pve_network_receive_bytes`,
  `pve_disk_write_bytes`, `pve_disk_read_bytes` — are **not** emitted. Only the
  monotonic `_total` counters are exposed. Migrate dashboards to `rate(..._total)`.
  See [ADR 0005](adr/0005-naming-units.md).

!!! note "Known gaps (partial / best-effort)"
    - **Node-level `pve_ha_state`** and the detailed **corosync qdevice** fields
      (algorithm / tie_breaker / state) require the HA and corosync endpoints and
      are only partial / best-effort.
    - `pve_qdevice_info` exposes **model + host + algorithm only**; tie-breaker and
      detailed qdevice state are not surfaced.
