# CLI & Validation

## Flags Reference

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--config` | `-c` | Yes | Path to `config.yaml`. |
| `--debug` | `-d` | No | Enable debug-level logging; dumps every collected metric sample to stdout in sorted exposition format. |
| `--once` | | No | Run exactly one collection cycle and exit (useful for validation and CI). |
| `--trace` | | No | Log each PVE API response body (method, path, HTTP status, body) to stderr. |

## Live Validation — the "Trace Run"

The combination of `--once`, `--debug`, and `--trace` gives you a complete
picture of one collection cycle without running a persistent server:

```bash
pve_exporter --config real.yaml --once --debug --trace 2>trace.log | sort > samples.txt
```

| Output | Contents |
|--------|----------|
| `samples.txt` | The full metric set in sorted exposition format. Diff it against the [Metrics Reference](metrics.md) to catch silently-absent metrics. |
| `trace.log` | Raw PVE API responses (method / path / status / body) for every call made during the cycle. |

### Security note

The **API token is never logged**. Authentication is header-only
(`Authorization: PVEAPIToken=…`) and `--trace` logs only *response bodies* —
not request headers. Trace output is therefore safe to share when opening a bug report.

## Endpoints

| Path | Description |
|------|-------------|
| `/metrics` | Prometheus text exposition (default port `9221`). The path is `server.uri`. |
| `/livez` | Liveness probe. Always `200 OK`; reads no collection state. |
| `/readyz` | Readiness probe. Always `200 OK`; reads no collection state. |
| `/health` | Informational. Always `200 OK` while the process is serving. |

Point Kubernetes `livenessProbe` and `readinessProbe` at `/livez` and
`/readyz`. Never probe `/metrics`: it renders the full exposition on every
tick and can block behind a slow collection cycle. Cluster reachability is a
`/metrics` question — see `pve_up` and `pve_request_errors_total`.

`server.uri` must start with a slash and may not be `/`, `/health`, `/livez`
or `/readyz`: those are reserved for the routes above. Startup fails with an
explicit `server.uri: …` error rather than a mux panic.

## Troubleshooting

**Empty `/metrics` or no `pve_` series**

- Confirm the API token has the **PVEAuditor** role at path `/` in Datacenter → Permissions.
- Verify the PVE host is reachable from the exporter: `curl -k https://<host>:8006`.
- Check exporter logs; re-run with `--debug` for more detail.

**`pve_up{id="cluster/..."} 0`**

The exporter reached the target but the scrape failed. Run `--once --trace` and inspect
`trace.log` for non-`2xx` responses from the PVE API.

**TLS errors (`x509: certificate signed by unknown authority`)**

Set `insecureSkipVerify: true` in the relevant cluster entry in `config.yaml`.
Proxmox VE ships with a self-signed certificate by default; this flag suppresses
verification for that cluster only.

**Port already in use**

Another process is bound to `9221`. Change the listen address via your process
supervisor or Compose override, or stop the conflicting process.
