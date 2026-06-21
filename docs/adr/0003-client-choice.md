# ADR 0003 — Hand-rolled PVE client vs. community Go SDK

- **Status:** Accepted
- **Date:** 2026-06-21
- **Deciders:** Frederic Jacquet

## Context

The exporter needs to talk to the Proxmox VE REST API. The data it requires is a
small, fixed set of **read-only JSON GETs**, each returning a `{"data": ...}`
envelope:

- `/cluster/resources` (bulk: nodes, guests, storage)
- `/cluster/status`, `/version`, `/cluster/config/qdevice`
- `/cluster/backup-info/not-backed-up`
- per-node `/nodes/{node}/replication[/{id}/status]`,
  `/nodes/{node}/subscription`, `/nodes/{node}/{qemu,lxc}` + `/config` (onboot)

There is **no official Proxmox Go SDK**. The main option to evaluate is the
community library `github.com/luthermonson/go-proxmox`.

## Decision

**Hand-roll a thin client on `github.com/go-resty/resty/v2`** rather than depend
on `go-proxmox`.

The PVE surface we touch is a handful of GETs over a stable JSON envelope. A small
resty-based client gives us exactly those calls, a `{"data": ...}` decode helper,
and full control of the HTTP transport — which the snapshot model and the
token-safe `--trace` hook both depend on. This matches the approach already taken
by the sibling `pflex_exporter` and `ppdd_exporter`.

### Why not `github.com/luthermonson/go-proxmox`

- **Exporter-irrelevant dependencies.** It pulls `go-diskfs`,
  `gorilla/websocket`, `buger/goterm`, and `jinzhu/copier` — none of which a
  read-only metrics scraper needs, all of which enlarge the dependency and CVE
  surface.
- **Maintenance mode.** Last release was in 2024; the project is effectively in
  maintenance mode.
- **Go version floor.** It imposes a **Go 1.25** minimum, constraining our
  toolchain for no benefit to a read-only client.
- **HTTP-layer control.** It owns its HTTP client, which would cost us the control
  needed for the **token-safe `--trace`** request/response hook (which must scrub
  the `PVEAPIToken` secret) and for the resty-level retry policy used across the
  siblings.

## Consequences

**Positive**

- Minimal, auditable dependency tree; no exporter-irrelevant transitive deps.
- Full control of the HTTP layer for `--trace` token scrubbing and retry policy
  (see [ADR 0004](0004-auth.md)).
- Consistent client shape across the three sibling exporters.
- No externally-imposed Go version floor.

**Negative / trade-offs**

- We own the (small) request/decode code and must extend it when adding endpoints.
- No SDK-provided types — we model only the fields we consume (which is also a
  deliberate "absent-not-zero" benefit; see [ADR 0006](0006-label-invariant.md)).

## Alternatives considered

- **`github.com/luthermonson/go-proxmox`** — rejected for the dependency weight,
  maintenance status, Go 1.25 floor, and loss of HTTP-layer control above.
- **Generate a client from the PVE API schema** — rejected as overkill for a
  fixed handful of read-only GETs.
