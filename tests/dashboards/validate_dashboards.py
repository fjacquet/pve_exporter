#!/usr/bin/env python3
"""Regression checks for the Grafana dashboard suite.

Scans grafana/dashboards/*.json and fails if any dashboard:
  - Contains invalid JSON
  - Uses the non-existent 'type' label in a PromQL selector
  - References a pve_* metric name not in the ALLOWED set
  - Has a non-row panel whose datasource is not set to ${datasource}
"""
import glob
import json
import os
import re
import sys

ALLOWED = {
    "pve_up", "pve_disk_size_bytes", "pve_disk_usage_bytes", "pve_memory_size_bytes",
    "pve_memory_usage_bytes", "pve_cpu_usage_ratio", "pve_cpu_usage_limit", "pve_uptime_seconds",
    "pve_storage_shared", "pve_ha_state", "pve_lock_state", "pve_network_receive_bytes_total",
    "pve_network_transmit_bytes_total", "pve_disk_read_bytes_total", "pve_disk_written_bytes_total",
    "pve_guest_info", "pve_storage_info", "pve_node_info", "pve_cluster_info", "pve_version_info",
    "pve_qdevice_up", "pve_qdevice_info", "pve_subscription_info", "pve_subscription_status",
    "pve_subscription_next_due_timestamp_seconds", "pve_onboot_status", "pve_not_backed_up_total",
    "pve_not_backed_up_info", "pve_replication_info", "pve_replication_duration_seconds",
    "pve_replication_last_sync_timestamp_seconds", "pve_replication_last_try_timestamp_seconds",
    "pve_replication_next_sync_timestamp_seconds", "pve_replication_failed_syncs",
}

HERE = os.path.dirname(__file__)
DASH_DIR = os.path.join(HERE, "..", "..", "grafana", "dashboards")
metric_re = re.compile(r"\bpve_[a-z_]+\b")


def exprs(dash):
    """Yield (panel_title, expr) for every PromQL expression in the dashboard."""
    for p in dash.get("panels", []):
        for t in p.get("targets", []):
            if t.get("expr"):
                yield p.get("title", "?"), t["expr"]


def check(path):
    """Return a list of error strings for the dashboard at path, or [] if clean."""
    errs = []
    with open(path) as f:
        dash = json.load(f)  # raises on invalid JSON

    for title, expr in exprs(dash):
        if re.search(r"\btype\s*=", expr):
            errs.append(f"{title}: uses non-existent 'type' label: {expr}")
        for m in metric_re.findall(expr):
            if m not in ALLOWED:
                errs.append(f"{title}: unknown metric {m}")

    for p in dash.get("panels", []):
        if p.get("type") == "row":
            continue
        ds = json.dumps(p.get("datasource", ""))
        if "${datasource}" not in ds:
            errs.append(f"{p.get('title', '?')}: panel datasource not set to ${{datasource}}")

    return errs


def main():
    files = sorted(glob.glob(os.path.join(DASH_DIR, "*.json")))
    if not files:
        print("no dashboards found yet")
        return 0

    failed = False
    for path in files:
        errs = check(path)
        if errs:
            failed = True
            print(f"FAIL {os.path.basename(path)}")
            for e in errs:
                print(f"  - {e}")
        else:
            print(f"ok   {os.path.basename(path)}")

    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
