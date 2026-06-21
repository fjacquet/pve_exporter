# ADR-0007 — QDevice tie-breaker / state fields: not available via stable REST API

**Status:** Accepted  
**Date:** 2026-06-21

## Context

The Python community `prometheus-pve-exporter` exposes two additional fields on
`pve_qdevice_info`:

- `tie_breaker` — whether the qdevice acts as a tie-breaker vote.
- `state` — the current qdevice connection state (e.g. `connected`, `disconnected`).

These values come from **corosync quorum status** (`corosync-quorumtool -s`), not
from the PVE REST API. The investigation checked every candidate REST endpoint:

| Endpoint | Contains qdevice state? |
| -------- | ----------------------- |
| `GET /cluster/config/qdevice` | Model, network (host, algorithm) only — no state, no tie-breaker. |
| `GET /cluster/ha/status/current` | HA quorum/LRM/node/service status — no qdevice connection state. |
| `GET /cluster/status` | Node/cluster membership — no qdevice state. |
| `GET /nodes/{node}/status` | Per-node system status — no qdevice fields. |

There is no stable, documented PVE REST endpoint that surfaces qdevice `state` or
`tie_breaker`. The data lives in the corosync layer, accessible only via the
`corosync-quorumtool` CLI or the corosync D-Bus/IPC interface — neither of which is
reachable from the PVE REST API.

## Decision

Do **not** add `tie_breaker` or `state` labels to `pve_qdevice_info`.

The project's absent-not-zero invariant (ADR-0006) prohibits emitting fabricated or
placeholder values. Adding empty-string labels would bloat the label-key set and
mislead dashboards that pivot on these fields.

If a future PVE release exposes corosync qdevice state via the REST API, this ADR
should be revisited. The label keys `state` and `tie_breaker` are reserved for that
future addition so existing label-key sets remain stable on upgrade.

## Consequences

- `pve_qdevice_info` exposes `model`, `host`, and `algorithm` only — matching the
  data available from `GET /cluster/config/qdevice`.
- Users who need qdevice state must query corosync directly (e.g. via node exporter
  textfile collector running `corosync-quorumtool`).
- The gap is documented in `docs/metrics.md` so operators know where the boundary is.
