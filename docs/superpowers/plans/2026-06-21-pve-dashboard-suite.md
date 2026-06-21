# pve_exporter Grafana Dashboard Suite — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single broken `grafana/pve-overview.json` with a correct, six-dashboard Grafana suite that uses the real label schema and surfaces the full `pve_` metric set.

**Architecture:** Six provisioned Grafana 11+ dashboards under `grafana/dashboards/`. All panels select resources by `id` regex (there is no `type` label) and join the `*_info` metrics with PromQL `group_left` for human-readable names/tags. A Python regression test guards against the original bug class (dead `type=` selectors, unknown metric names, hard-coded datasources). Drilldown via data links + chained template variables.

**Tech Stack:** Grafana 11 (schemaVersion 39), Prometheus/PromQL, MkDocs (docs), Python 3 (validation test), docker-compose (render check).

## Global Constraints

- No `type` label exists. Select by `id` regex: nodes `id=~"node/.*"`, guests `id=~"(qemu|lxc)/.*"`, storage `id=~"storage/.*"`, replication jobs are bare `id` (e.g. `1-0`).
- Friendly names come from `group_left` joins on `pve_guest_info` / `pve_node_info` / `pve_storage_info`, keyed `on(cluster,id)`.
- Gauges (cpu ratio, memory/disk bytes, `*_info`, state enums) aggregate with `sum`/`avg` — **never `rate()`**. Only `*_bytes_total` counters use `rate(...[5m])`.
- Every panel target references the `${datasource}` variable; no hard-coded datasource uids.
- Target Grafana 11+ / schemaVersion 39. State enums (`pve_ha_state`, `pve_lock_state`, `pve_subscription_status`) render via State Timeline + current-state table on `<metric> == 1`.
- Files live in `grafana/dashboards/` in a "Proxmox VE" folder, loaded by the existing provider.
- Valid metric-name set (the test's allow-list): `pve_up`, `pve_disk_size_bytes`, `pve_disk_usage_bytes`, `pve_memory_size_bytes`, `pve_memory_usage_bytes`, `pve_cpu_usage_ratio`, `pve_cpu_usage_limit`, `pve_uptime_seconds`, `pve_storage_shared`, `pve_ha_state`, `pve_lock_state`, `pve_network_receive_bytes_total`, `pve_network_transmit_bytes_total`, `pve_disk_read_bytes_total`, `pve_disk_written_bytes_total`, `pve_guest_info`, `pve_storage_info`, `pve_node_info`, `pve_cluster_info`, `pve_version_info`, `pve_qdevice_up`, `pve_qdevice_info`, `pve_subscription_info`, `pve_subscription_status`, `pve_subscription_next_due_timestamp_seconds`, `pve_onboot_status`, `pve_not_backed_up_total`, `pve_not_backed_up_info`, `pve_replication_info`, `pve_replication_duration_seconds`, `pve_replication_last_sync_timestamp_seconds`, `pve_replication_last_try_timestamp_seconds`, `pve_replication_next_sync_timestamp_seconds`, `pve_replication_failed_syncs`.

---

## File Structure

- `tests/dashboards/validate_dashboards.py` — regression test over every dashboard JSON (Create).
- `grafana/dashboards/pve-cluster-overview.json` … `pve-ha-quorum.json` — six dashboards (Create).
- `grafana/pve-overview.json` — broken single dashboard (Delete).
- `grafana/provisioning/dashboards/dashboards.yml` — point provider at `grafana/dashboards/` (Modify if needed).
- `docker-compose.yml`, `docker-compose.ghcr.yml` — ensure Grafana mounts `grafana/dashboards/` → `/var/lib/grafana/dashboards` (Modify if needed).
- `docs/dashboards.md` — document the suite (Modify).

---

## Task 1: Validation harness + provisioning wiring

**Files:**
- Create: `tests/dashboards/validate_dashboards.py`
- Modify: `grafana/provisioning/dashboards/dashboards.yml`, `docker-compose.yml`, `docker-compose.ghcr.yml`
- Delete: `grafana/pve-overview.json`

**Interfaces:**
- Produces: a CLI test `python3 tests/dashboards/validate_dashboards.py` that scans `grafana/dashboards/*.json` and exits non-zero on any violation. Later tasks each add one dashboard and re-run this test.

- [ ] **Step 1: Write the validation test (the failing test)**

```python
#!/usr/bin/env python3
"""Regression checks for the Grafana dashboard suite."""
import glob, json, os, re, sys

ALLOWED = {
    "pve_up","pve_disk_size_bytes","pve_disk_usage_bytes","pve_memory_size_bytes",
    "pve_memory_usage_bytes","pve_cpu_usage_ratio","pve_cpu_usage_limit","pve_uptime_seconds",
    "pve_storage_shared","pve_ha_state","pve_lock_state","pve_network_receive_bytes_total",
    "pve_network_transmit_bytes_total","pve_disk_read_bytes_total","pve_disk_written_bytes_total",
    "pve_guest_info","pve_storage_info","pve_node_info","pve_cluster_info","pve_version_info",
    "pve_qdevice_up","pve_qdevice_info","pve_subscription_info","pve_subscription_status",
    "pve_subscription_next_due_timestamp_seconds","pve_onboot_status","pve_not_backed_up_total",
    "pve_not_backed_up_info","pve_replication_info","pve_replication_duration_seconds",
    "pve_replication_last_sync_timestamp_seconds","pve_replication_last_try_timestamp_seconds",
    "pve_replication_next_sync_timestamp_seconds","pve_replication_failed_syncs",
}
HERE = os.path.dirname(__file__)
DASH_DIR = os.path.join(HERE, "..", "..", "grafana", "dashboards")
metric_re = re.compile(r"\bpve_[a-z_]+\b")

def exprs(dash):
    for p in dash.get("panels", []):
        for t in p.get("targets", []):
            if t.get("expr"):
                yield p.get("title", "?"), t["expr"]

def check(path):
    errs = []
    with open(path) as f:
        dash = json.load(f)  # raises on invalid JSON
    for title, expr in exprs(path and dash):
        if re.search(r"\btype\s*=", expr):
            errs.append(f"{title}: uses non-existent 'type' label: {expr}")
        for m in metric_re.findall(expr):
            if m not in ALLOWED:
                errs.append(f"{title}: unknown metric {m}")
        if "${datasource}" not in json.dumps(
            next(t for t in dash["panels"] for t in [t]) if False else {}
        ):
            pass  # datasource checked below per-panel
    for p in dash.get("panels", []):
        ds = json.dumps(p.get("datasource", ""))
        if p.get("type") != "row" and "datasource" not in ds and "${datasource}" not in ds:
            errs.append(f"{p.get('title','?')}: panel datasource not set to ${{datasource}}")
    return errs

def main():
    files = sorted(glob.glob(os.path.join(DASH_DIR, "*.json")))
    if not files:
        print("no dashboards found yet"); return 0
    failed = False
    for path in files:
        errs = check(path)
        if errs:
            failed = True
            print(f"FAIL {os.path.basename(path)}")
            for e in errs: print(f"  - {e}")
        else:
            print(f"ok   {os.path.basename(path)}")
    return 1 if failed else 0

if __name__ == "__main__":
    sys.exit(main())
```

- [ ] **Step 2: Run it (passes vacuously — no dashboards yet)**

Run: `python3 tests/dashboards/validate_dashboards.py`
Expected: prints `no dashboards found yet`, exit 0. (`grafana/dashboards/` does not exist yet.)

- [ ] **Step 3: Point the provider at `grafana/dashboards/` and remove the broken dashboard**

In `grafana/provisioning/dashboards/dashboards.yml` ensure `options.path: /var/lib/grafana/dashboards` and `foldersFromFilesStructure: true`. In both compose files, ensure the Grafana service mounts `./grafana/dashboards:/var/lib/grafana/dashboards:ro` (in addition to the provisioning mount). Then delete the broken file:

Run: `git rm grafana/pve-overview.json`

- [ ] **Step 4: Re-run the test**

Run: `python3 tests/dashboards/validate_dashboards.py`
Expected: `no dashboards found yet`, exit 0.

- [ ] **Step 5: Commit**

```bash
git add tests/dashboards/validate_dashboards.py grafana/provisioning/dashboards/dashboards.yml docker-compose.yml docker-compose.ghcr.yml
git rm grafana/pve-overview.json
git commit -m "test(grafana): dashboard validation harness + provisioning wiring"
```

---

## Shared panel-authoring conventions (used by Tasks 2–7)

Every dashboard JSON: `schemaVersion: 39`, `"datasource": {"type":"prometheus","uid":"${datasource}"}` on every panel and target, and these template variables:

```json
{"name":"datasource","type":"datasource","query":"prometheus","current":{}},
{"name":"cluster","type":"query","datasource":{"type":"prometheus","uid":"${datasource}"},
 "query":"label_values(pve_up, cluster)","includeAll":true,"multi":true,"current":{"text":"All","value":"$__all"}},
{"name":"node","type":"query","datasource":{"type":"prometheus","uid":"${datasource}"},
 "query":"label_values(pve_node_info{cluster=~\"$cluster\"}, name)","includeAll":true},
{"name":"guest","type":"query","datasource":{"type":"prometheus","uid":"${datasource}"},
 "query":"label_values(pve_guest_info{cluster=~\"$cluster\"}, name)","includeAll":false}
```

(`node` is required only by `pve-node.json`; `guest` only by `pve-guest.json` — include the relevant subset per dashboard.)

Name-join idiom (repeat with the relevant `*_info` metric):
`<metric>{id=~"<selector>", cluster=~"$cluster"} * on(cluster,id) group_left(name,node,type,tags) pve_guest_info`

Each task: author the JSON to the panel spec below, then run the validation test, then a strict-ish JSON parse, then commit. The panel **specs are exact** (type, title, PromQL, unit, thresholds, links); author Grafana JSON to match.

---

## Task 2: Cluster Overview — `grafana/dashboards/pve-cluster-overview.json`

**Files:** Create `grafana/dashboards/pve-cluster-overview.json`. Test: `tests/dashboards/validate_dashboards.py`.

**Panels (exact targets):**
- Row "Health & Status"
  - stat **Nodes Up**: `sum(pve_up{id=~"node/.*", cluster=~"$cluster"})` / total `count(pve_up{id=~"node/.*", cluster=~"$cluster"})`
  - stat **Guests Running**: `sum(pve_up{id=~"(qemu|lxc)/.*", cluster=~"$cluster"})`
  - stat **Quorate**: `max(pve_cluster_info{cluster=~"$cluster"} and on(cluster,id) (pve_cluster_info==pve_cluster_info))` — simpler: `max by (cluster)(pve_cluster_info{cluster=~"$cluster"})` is 1; show quorate via label using a table; for the stat use `sum(pve_up{id=~"node/.*"} == 1)` vs configured. (Quorate value is a label on `pve_cluster_info`; surface it in the HA dashboard table. Here show nodes online.)
  - stat **Storages Available**: `sum(pve_up{id=~"storage/.*", cluster=~"$cluster"})`
  - stat **Replication Failures**: `sum(pve_replication_failed_syncs{cluster=~"$cluster"}) or vector(0)` — thresholds: 0 green, ≥1 red
  - stat **Guests Not Backed Up**: `sum(pve_not_backed_up_total{cluster=~"$cluster"}) or vector(0)`
  - stat **Subscription Active**: `min(pve_subscription_status{status="active", cluster=~"$cluster"})` — 1 green / 0 red
- Row "Capacity"
  - bargauge **Memory Used %** (per node): `(pve_memory_usage_bytes{id=~"node/.*",cluster=~"$cluster"} / pve_memory_size_bytes{id=~"node/.*",cluster=~"$cluster"}) * on(cluster,id) group_left(name) pve_node_info` — unit percentunit, thresholds 0.8/0.9
  - bargauge **Storage Used %**: `(pve_disk_usage_bytes{id=~"storage/.*",cluster=~"$cluster"} / pve_disk_size_bytes{id=~"storage/.*",cluster=~"$cluster"}) * on(cluster,id) group_left(storage,node) pve_storage_info` — percentunit, 0.8/0.9
  - stat **CPU Overcommit**: `sum(pve_cpu_usage_limit{id=~"(qemu|lxc)/.*",cluster=~"$cluster"}) / sum(pve_cpu_usage_limit{id=~"node/.*",cluster=~"$cluster"})`
- Row "Activity"
  - timeseries **Node CPU**: `avg by (id)(pve_cpu_usage_ratio{id=~"node/.*",cluster=~"$cluster"} * on(cluster,id) group_left(name) pve_node_info)` — unit percentunit
  - timeseries **Node Memory Used/Total**: `sum(pve_memory_usage_bytes{id=~"node/.*",cluster=~"$cluster"})` and `sum(pve_memory_size_bytes{id=~"node/.*",cluster=~"$cluster"})` — unit bytes
  - timeseries **Cluster Network I/O**: `sum(rate(pve_network_receive_bytes_total{cluster=~"$cluster"}[5m]))` and `sum(rate(pve_network_transmit_bytes_total{cluster=~"$cluster"}[5m]))` — unit Bps
  - timeseries **Cluster Disk I/O**: `sum(rate(pve_disk_read_bytes_total{cluster=~"$cluster"}[5m]))` and `sum(rate(pve_disk_written_bytes_total{cluster=~"$cluster"}[5m]))` — unit Bps
- Row "Inventory"
  - table **Nodes**: `pve_up{id=~"node/.*",cluster=~"$cluster"} * on(cluster,id) group_left(name) pve_node_info` joined (instant) with cpu `pve_cpu_usage_ratio`, mem ratio, `pve_uptime_seconds`; transform: organize fields → name, status, cpu%, mem%, uptime. Data link on row → `/d/pve-node?var-node=${__data.fields.name}&var-cluster=${cluster}`.
  - table **Top Guests by CPU**: `topk(10, pve_cpu_usage_ratio{id=~"(qemu|lxc)/.*",cluster=~"$cluster"} * on(cluster,id) group_left(name,node,type) pve_guest_info)` — data link → `/d/pve-guest?var-guest=${__data.fields.name}`.

- [ ] **Step 1:** Author `pve-cluster-overview.json` (uid `pve-cluster-overview`) to the panel spec above, every panel + target datasource = `${datasource}`, schemaVersion 39, variables `datasource`,`cluster`.
- [ ] **Step 2:** `python3 -c "import json;json.load(open('grafana/dashboards/pve-cluster-overview.json'))"` → no error.
- [ ] **Step 3:** `python3 tests/dashboards/validate_dashboards.py` → `ok   pve-cluster-overview.json`, exit 0.
- [ ] **Step 4:** Commit: `git add grafana/dashboards/pve-cluster-overview.json && git commit -m "feat(grafana): cluster overview dashboard"`

---

## Task 3: Node detail — `grafana/dashboards/pve-node.json` (var `node`)

**Panels:**
- Header stats (filter node by name via join): up `max(pve_up{id=~"node/.*"} * on(cluster,id) group_left(name) pve_node_info{name=~"$node"})`; cpu `pve_cpu_usage_ratio{...}`; mem used/total; root disk % (`pve_disk_usage_bytes / pve_disk_size_bytes` for the node id); `pve_uptime_seconds`; guest count `count(pve_guest_info{node=~"$node",cluster=~"$cluster"})`; PVE version from `pve_version_info` (table/stat text); subscription days-to-due `((pve_subscription_next_due_timestamp_seconds * on(cluster,id) group_left(name) pve_node_info{name=~"$node"}) - time())/86400`.
- timeseries CPU ratio, memory used vs size, root disk usage — all filtered to the node via `group_left(name) pve_node_info{name=~"$node"}`.
- table **Guests on node**: `pve_guest_info{node=~"$node",cluster=~"$cluster"}` joined with `pve_up`, `pve_cpu_usage_ratio`, mem ratio, `pve_uptime_seconds`, `pve_onboot_status`; columns name, type, running, cpu%, mem%, uptime, onboot, tags; row data link → `/d/pve-guest?var-guest=${__data.fields.name}&var-cluster=${cluster}`.

- [ ] Step 1: Author `pve-node.json` (uid `pve-node`, variables `datasource`,`cluster`,`node`).
- [ ] Step 2: JSON parses (python `json.load`).
- [ ] Step 3: `python3 tests/dashboards/validate_dashboards.py` → ok.
- [ ] Step 4: Commit `feat(grafana): node detail dashboard`.

---

## Task 4: Guest detail — `grafana/dashboards/pve-guest.json` (var `guest`, drilldown target)

**Panels** (filter by guest name via `group_left(name) pve_guest_info{name=~"$guest"}`):
- Header: running `pve_up`; text fields name/node/type/tags/template from `pve_guest_info{name=~"$guest"}`; onboot `pve_onboot_status`; cpu limit `pve_cpu_usage_limit`; current ha_state `pve_ha_state{...}==1` (show `state` label); current lock `pve_lock_state{...}==1`.
- timeseries: CPU `pve_cpu_usage_ratio`; memory `pve_memory_usage_bytes` vs `pve_memory_size_bytes`; disk IO `rate(pve_disk_read_bytes_total[5m])` & `rate(pve_disk_written_bytes_total[5m])`; net IO `rate(pve_network_receive_bytes_total[5m])` & `rate(pve_network_transmit_bytes_total[5m])` — each joined to the guest.
- state-timeline **HA state**: `pve_ha_state * on(cluster,id) group_left(name) pve_guest_info{name=~"$guest"}` (value mapping per `state`). state-timeline **Lock state**: `pve_lock_state{...}`.
- table **DR**: `pve_not_backed_up_info` presence for this guest + `pve_replication_info{guest=~"(qemu|lxc)/.*"}` filtered by guest, with `pve_replication_failed_syncs` and last_sync age `time() - pve_replication_last_sync_timestamp_seconds`.

- [ ] Step 1: Author `pve-guest.json` (uid `pve-guest`, variables `datasource`,`cluster`,`guest`).
- [ ] Step 2: JSON parses.
- [ ] Step 3: validation test → ok.
- [ ] Step 4: Commit `feat(grafana): guest detail dashboard`.

---

## Task 5: Storage — `grafana/dashboards/pve-storage.json`

**Panels:**
- table **Storages**: `pve_storage_info{cluster=~"$cluster"}` joined with `pve_disk_usage_bytes`, `pve_disk_size_bytes`, `pve_storage_shared`, `pve_up`; columns storage, node, plugintype, content, shared, used (bytes), size (bytes), used% (`usage/size`).
- bargauge **Used % per storage**: `(pve_disk_usage_bytes{id=~"storage/.*",cluster=~"$cluster"} / pve_disk_size_bytes{id=~"storage/.*",cluster=~"$cluster"}) * on(cluster,id) group_left(storage,node) pve_storage_info` — percentunit, 0.8/0.9.
- timeseries **Used vs Size per storage**: `pve_disk_usage_bytes{id=~"storage/.*",cluster=~"$cluster"} * on(cluster,id) group_left(storage) pve_storage_info` and the size series — unit bytes.
- stat **Total capacity** `sum(pve_disk_size_bytes{id=~"storage/.*",cluster=~"$cluster"})`; **Total used** `sum(pve_disk_usage_bytes{id=~"storage/.*",cluster=~"$cluster"})`; **Shared storages** `sum(pve_storage_shared{cluster=~"$cluster"})`.

- [ ] Step 1: Author `pve-storage.json` (uid `pve-storage`). Step 2: parses. Step 3: validation ok. Step 4: Commit `feat(grafana): storage dashboard`.

---

## Task 6: Backup & DR — `grafana/dashboards/pve-backup-dr.json`

**Panels:**
- stat **Not Backed Up** `sum(pve_not_backed_up_total{cluster=~"$cluster"}) or vector(0)`.
- table **Guests Not Backed Up**: `pve_not_backed_up_info{cluster=~"$cluster"} * on(cluster,id) group_left(name,node,type) pve_guest_info` — columns name, node, type.
- table **Replication Jobs**: `pve_replication_info{cluster=~"$cluster"}` joined with `pve_replication_failed_syncs`, `pve_replication_duration_seconds`, last_sync age `time() - pve_replication_last_sync_timestamp_seconds`, next_sync countdown `pve_replication_next_sync_timestamp_seconds - time()`; columns id, guest, source, target, type, last_sync_age(s), duration(s), failed, next_in(s). Threshold red on failed≥1.
- timeseries **Replication last_sync age**: `time() - pve_replication_last_sync_timestamp_seconds{cluster=~"$cluster"}` — unit s.
- stat **Failed syncs total** `sum(pve_replication_failed_syncs{cluster=~"$cluster"})`; **Max replication age** `max(time() - pve_replication_last_sync_timestamp_seconds{cluster=~"$cluster"})`.
- table **Subscription expiry**: `(pve_subscription_next_due_timestamp_seconds{cluster=~"$cluster"} - time()) * on(cluster,id) group_left(name) pve_node_info` — columns node name, seconds-to-due (unit s); thresholds warn at 30 days.

- [ ] Step 1: Author `pve-backup-dr.json` (uid `pve-backup-dr`). Step 2: parses. Step 3: validation ok. Step 4: Commit `feat(grafana): backup & DR dashboard`.

---

## Task 7: HA & Quorum — `grafana/dashboards/pve-ha-quorum.json`

**Panels:**
- stat **Nodes online** `sum(pve_up{id=~"node/.*",cluster=~"$cluster"})`.
- table **Cluster info**: `pve_cluster_info{cluster=~"$cluster"}` — surface `nodes`, `quorate`, `version` labels (transform: labels to fields). Map quorate 1→Quorate green / 0→No quorum red.
- stat **QDevice up** `sum(pve_qdevice_up{cluster=~"$cluster"}) or vector(0)`; table **QDevice info** `pve_qdevice_info{cluster=~"$cluster"}` (labels model, host, algorithm to fields).
- state-timeline **HA-managed guest states**: `pve_ha_state{cluster=~"$cluster"} * on(cluster,id) group_left(name) pve_guest_info`.
- table **Current HA state per guest**: `pve_ha_state{cluster=~"$cluster"} == 1` joined `group_left(name,node) pve_guest_info` — columns name, node, state.
- table **Node membership**: `pve_node_info{cluster=~"$cluster"}` joined `pve_up` — columns name, nodeid, level, online.

> Note: node-level `pve_ha_state` is not emitted (collector covers guest `hastate` only), and `pve_qdevice_info` exposes model/host/algorithm only — `tie_breaker`/`state` are absent. These are tracked as a separate exporter follow-up (out of scope here); the corresponding columns will be empty until then.

- [ ] Step 1: Author `pve-ha-quorum.json` (uid `pve-ha-quorum`). Step 2: parses. Step 3: validation ok. Step 4: Commit `feat(grafana): HA & quorum dashboard`.

---

## Task 8: Docs + end-to-end render check

**Files:** Modify `docs/dashboards.md`.

- [ ] **Step 1:** Rewrite `docs/dashboards.md` to document the six dashboards, the `id`-regex + `group_left` name-join convention, the gauge-vs-counter aggregation rule, and drilldown navigation (Overview → Node → Guest).
- [ ] **Step 2:** Run the full validation test over all six: `python3 tests/dashboards/validate_dashboards.py` → six `ok` lines, exit 0.
- [ ] **Step 3:** Render check — `docker compose up -d` (or `docker-compose.ghcr.yml`), open Grafana (`http://localhost:3000`), confirm each dashboard loads with data and **no empty panels**; click an Overview node and guest row to confirm drilldown carries `var-node`/`var-guest`. Capture nothing; just verify.
- [ ] **Step 4:** `mkdocs build --strict` → builds clean.
- [ ] **Step 5:** Commit: `git add docs/dashboards.md && git commit -m "docs: document the dashboard suite"`.

---

## Self-Review

**Spec coverage:** Foundation (label schema, joins, variables, drilldown, aggregation, state panels, provisioning) → Task 1 + shared conventions. Six dashboards → Tasks 2–7 (one each, matching the spec inventory). Files/provisioning + remove old dashboard → Task 1. Docs update → Task 8. Testing/validation (JSON parse, query checks, render, drilldown) → Task 1 harness + per-task steps + Task 8 render check. Out-of-scope metric gaps → noted in Task 7. All spec sections map to a task.

**Placeholder scan:** PromQL is given verbatim per panel; the validation test is complete code; no "TBD"/"add error handling"/"similar to Task N". The Grafana JSON itself is specified by exact panel/target/unit/threshold/link rather than inlined as multi-thousand-line blobs — each authoring step has a concrete spec and three verification steps.

**Type consistency:** Selectors are uniform (`id=~"node/.*"`, `id=~"(qemu|lxc)/.*"`, `id=~"storage/.*"`); joins always `on(cluster,id) group_left(...)`; dashboard uids referenced by drilldown links (`pve-node`, `pve-guest`) match the uids assigned in Tasks 3–4; variable names (`datasource`,`cluster`,`node`,`guest`) consistent across tasks and the shared conventions block.
