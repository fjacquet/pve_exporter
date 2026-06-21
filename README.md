# Proxmox VE Exporter

A Prometheus + OTLP exporter for Proxmox VE (PVE) clusters, written in Go.

[![CI](https://github.com/fjacquet/pve_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/fjacquet/pve_exporter/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/fjacquet/pve_exporter?include_prereleases&sort=semver)](https://github.com/fjacquet/pve_exporter/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/fjacquet/pve_exporter)](https://goreportcard.com/report/github.com/fjacquet/pve_exporter)
[![Go Version](https://img.shields.io/github/go-mod/go-version/fjacquet/pve_exporter)](go.mod)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Docs](https://img.shields.io/badge/docs-GitHub%20Pages-blue.svg)](https://fjacquet.github.io/pve_exporter/)

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

## Quick start

### Docker Compose

```bash
cp .env.example .env        # fill in PVE1_HOST / PVE1_TOKEN_ID / PVE1_TOKEN_SECRET
docker compose up -d
curl -s localhost:9221/metrics | grep '^pve_'
```

The bundled stack also wires Prometheus and Grafana — see
[Deployment › Docker](https://fjacquet.github.io/pve_exporter/deployment/docker/).

### Binary

```bash
go build -o pve_exporter .
./pve_exporter --config config.yaml
```

Useful flags:

| Flag             | Description                                                  |
| ---------------- | ------------------------------------------------------------ |
| `--config`, `-c` | Path to the YAML config file (**required**).                 |
| `--debug`, `-d`  | Enable debug logging.                                        |
| `--once`         | Collect once, print metrics, then exit (good for CI/cron).   |
| `--trace`        | Token-safe HTTP request/response tracing (no secrets logged).|

## PVE API token setup

The exporter needs a **read-only** API token with the built-in **PVEAuditor**
role granted at path `/`:

```bash
# On a PVE node, as root:
pveum user add exporter@pve
pveum aclmod / -user exporter@pve -role PVEAuditor
pveum user token add exporter@pve metrics --privsep 0
```

The last command prints the token ID (`exporter@pve!metrics`) and secret. The
secret is shown **once** — copy it. The exporter sends it verbatim as:

```
Authorization: PVEAPIToken=exporter@pve!metrics=<secret>
```

## Configuration overview

```yaml
collection:
  interval: 30s

server:
  listen: ":9221"

clusters:
  - name: pve1                       # becomes the `cluster` label
    host: https://${PVE1_HOST}:8006
    tokenID: ${PVE1_TOKEN_ID}        # e.g. exporter@pve!metrics
    tokenSecret: ${PVE1_TOKEN_SECRET}
    # tokenSecretFile: /run/secrets/pve1_token   # alternative to tokenSecret
    insecureSkipVerify: true         # for self-signed PVE certs
```

`${ENV_VAR}` placeholders are expanded from the environment (and an autoloaded
`.env`). The config hot-reloads on `SIGHUP` and on file change.

## Metric families

| Family               | Example metrics                                                            |
| -------------------- | ------------------------------------------------------------------------- |
| Status / info        | `pve_up`, `pve_node_info`, `pve_cluster_info`, `pve_guest_info`            |
| Resources            | `pve_cpu_usage_ratio`, `pve_memory_usage_bytes`, `pve_disk_usage_bytes`   |
| Counters             | `pve_network_*_bytes_total`, `pve_disk_{read,written}_bytes_total`        |
| State enums          | `pve_ha_state`, `pve_lock_state`, `pve_subscription_status`               |
| Subscription         | `pve_subscription_info`, `pve_subscription_next_due_timestamp_seconds`    |
| Replication          | `pve_replication_info`, `pve_replication_duration_seconds`, ...           |
| Backup / quorum      | `pve_not_backed_up_total`, `pve_qdevice_up`, `pve_qdevice_info`           |

The full catalogue — names, HELP and labels — lives in
[Metrics Reference](https://fjacquet.github.io/pve_exporter/metrics/).

## Documentation

- [Documentation site](https://fjacquet.github.io/pve_exporter/)
- [Metrics reference](https://fjacquet.github.io/pve_exporter/metrics/)
- [Dashboards](https://fjacquet.github.io/pve_exporter/dashboards/)
- [Docker deployment](https://fjacquet.github.io/pve_exporter/deployment/docker/)
- [Architecture Decision Records](https://fjacquet.github.io/pve_exporter/adr/)

## License

Apache-2.0. See [LICENSE](LICENSE).
