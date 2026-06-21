# CLAUDE.md

Project memory for `pve_exporter` — a Prometheus + OTLP exporter for Proxmox VE.

## Overview

Module `github.com/fjacquet/pve_exporter`, binary `pve_exporter`. Metric prefix
`pve_`, default listen `:9221`, metrics on `/metrics`. Scrapes the Proxmox VE REST
API (read-only) for one or many clusters and exports to both Prometheus and OTLP.

## Commands

```bash
make build            # build ./pve_exporter
make test             # run unit tests
make lint             # golangci-lint
make sbom             # CycloneDX SBOM
make release-snapshot # local GoReleaser build (no publish)

./pve_exporter --config config.yaml      # run
./pve_exporter -c config.yaml --once     # collect once, print, exit
./pve_exporter -c config.yaml --trace    # token-safe HTTP tracing
mkdocs serve          # preview docs site
```

CLI flags (cobra): `--config/-c` (required), `--debug/-d`, `--once`, `--trace`.

## Architecture

A background loop polls each target on `collection.interval`, builds an **immutable
snapshot**, and atomically swaps it into a store behind an `RWMutex` pointer-swap.
The Prometheus **unchecked collector** and the OTLP **observable gauges** both read
the *same* snapshot, so the backends never disagree. The HTTP server starts
**before** the first collection (see ADR 0002).

Packages under `internal/`:

- `pve` — hand-rolled `resty/v2` client (no SDK; see ADR 0003), endpoint calls,
  the snapshot collector, and the Prometheus collector / OTLP gauges.
- `models` — typed structs for the PVE API responses we consume (only the fields
  we use — this gives absent-not-zero for free).
- `config` — YAML loader with `${ENV_VAR}` expansion + `.env` autoload; SIGHUP and
  file-watch hot reload; `clusters[]` targets.
- `utils` — small shared helpers.
- `logging` — structured logger setup; `--debug` / `--trace` wiring.
- `telemetry` — OTLP exporter / meter provider setup.

Endpoints scraped: `/cluster/resources` (bulk nodes/guests/storage),
`/cluster/status`, `/version`, `/cluster/config/qdevice`,
`/cluster/backup-info/not-backed-up`, and per-node
`/nodes/{node}/replication[/{id}/status]`, `/nodes/{node}/subscription`,
`/nodes/{node}/{qemu,lxc}` + `/config` (onboot).

## Conventions and non-obvious constraints

- **Token-safe trace.** The `PVEAPIToken` secret lives only in the `Authorization`
  header and is never logged; `--trace` scrubs it. Auth is a static header — no
  login, no refresh (ADR 0004).
- **Retry excludes 4xx.** Only `429`/`5xx` are retried; `4xx` is a config error.
- **Absent-not-zero.** A field missing from the API yields **no series**, not `0`.
  Model only the fields you consume (ADR 0006).
- **Label invariant.** One label-key set per metric name. State enums
  (`ha_state`, `lock_state`, `subscription_status`) emit all states with `0/1` to
  keep label keys stable (ADR 0006).
- **Identity labels.** Every metric carries `cluster` (target name) and the
  Proxmox-native `id` (`node/<name>`, `qemu/<vmid>`, `lxc/<vmid>`,
  `storage/<node>/<store>`, `cluster/<name>`, replication `<jobid>`).
- **No deprecated gauge aliases.** Only `*_total` counters are emitted (ADR 0005).

## Adding metrics or object types

1. Add the consumed fields to the relevant `models` struct (omit fields you don't
   use — absent-not-zero).
2. Populate them when building the snapshot in `pve`.
3. Emit from both the Prometheus collector and the OTLP gauges, reading the
   snapshot. Keep the label-key set stable (ADR 0006).
4. Document the metric in `docs/metrics.md` (name, HELP, labels).

## Testing

`httptest` mock PVE server returns canned `{"data": ...}` fixtures. Tests assert
the resulting metrics via **both** read paths: scrape the Prometheus registry
(`testutil` / `CollectAndCompare`) **and** read an OTLP `ManualReader`, asserting
both produce the expected series — this guards the "both backends read the same
snapshot" invariant.

## CI/CD

Hardened reusable workflow **`fjacquet/ci@v1`**, SHA-pinned actions, Dependabot,
GoReleaser releases with a CycloneDX SBOM, multi-arch GHCR image with provenance
attestations (ADR 0001).
