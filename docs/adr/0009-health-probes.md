# ADR-0009 — `/livez` and `/readyz` probes, and a container `HEALTHCHECK`

**Status:** Accepted
**Date:** 2026-08-01

## Context

This exporter served exactly one health endpoint, `/health`, which returns
`200 OK` unconditionally and reads no collection state. That was already the
right *behaviour* — unlike several sibling exporters in the family, `/health`
here was never gated on a snapshot being present — but it left two gaps.

First, `/health` is a single path carrying two distinct meanings. An operator
writing a Kubernetes manifest has to decide whether it is a liveness probe, a
readiness probe, or an informational endpoint, and nothing in the name or the
response tells them. The family standard settles this with fixed paths:
`/livez` for liveness, `/readyz` for readiness, `/health` for information.

Second, the published container image declared no `HEALTHCHECK`, so Docker and
Compose reported the container's status purely from the process being alive.
A wedged HTTP server looked healthy. The natural workaround — pointing a probe
at `/metrics` — is worse than nothing: it renders the full exposition on every
probe tick and can block behind a slow collection cycle, turning a slow scrape
into a restart loop.

## Decision

Add `/livez` and `/readyz`, both wired to one package-level handler in
`main.go`:

```go
func staticOKHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
```

The handler reads *nothing* — not the `SnapshotStore`, not the config, not the
collection clock. A probe wired here can never be the reason a healthy process
is restarted or pulled from rotation. Cluster reachability is a `/metrics`
question (`pve_up`, `pve_request_errors_total`), not a probe question.

The registrations go on the existing mux, which is built inline in `run()`.
No helper is extracted and the server construction is not refactored: matching
this file's own shape was chosen over converging on a sibling repo's layout.

`/health` is left exactly as it was. It already answered 200 unconditionally,
so there was nothing to change; `main_test.go` now pins that property so it
cannot regress into a 503 later.

Both `./Dockerfile` and `Dockerfile.goreleaser` gain:

```dockerfile
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9221/livez || exit 1
```

and both compose files gain a `healthcheck:` with the same values. The address
is `127.0.0.1`, never `localhost`: Alpine's busybox `wget` resolves `localhost`
via `::1` first and the exporter binds IPv4 only, so a `localhost`-based check
fails with connection refused — while still passing `hadolint` and
`docker compose config`. The compose `healthcheck:` overrides the image's, so
the two must agree; the `5s` timeout is deliberately identical in all four
files.

## Consequences

- Kubernetes `livenessProbe` and `readinessProbe` have unambiguous, cheap
  targets that never render the exposition.
- `docker ps` and `docker inspect .State.Health.Status` report a real health
  status for the container, and Compose `depends_on: service_healthy` works.
- The shell-form `HEALTHCHECK CMD … || exit 1` unavoidably triggers hadolint
  `DL3025`; `DL3007`/`DL3066` remain standing family findings against the
  unpinned `alpine:latest` base. These are accepted, not suppressed — the repo
  adds no inline `hadolint ignore` comments.
- The real cost of that unpinned base is **build reproducibility**: two builds
  of the same commit can resolve a different `alpine:latest` layer, so the
  runtime image is not a function of the source tree alone. That is accepted
  deliberately, in exchange for picking up Alpine CVE fixes automatically on
  every rebuild instead of waiting for someone to bump a digest. Provenance for
  any particular image is recovered from its digest and published SBOM, not
  from the Dockerfile. Pinning by digest would invert the trade —
  reproducible builds, manual CVE tracking — and is a family-wide decision to
  revisit, not a per-repo one.
- `/livez` and `/readyz` return identical responses today. Should this exporter
  ever grow a genuine not-ready state — a required config that resolves
  asynchronously, say — `/readyz` is the endpoint that would change, and
  `/livez` is the one that must not.
- The base image needed no change: this repo already used `alpine:latest` in
  both Dockerfiles, so the family's Alpine convergence was a no-op here.
- Verifying the `./Dockerfile` `HEALTHCHECK` end to end surfaced an unrelated,
  pre-existing defect: `go.mod` required `go 1.26.5` (since v0.4.2's
  GO-2026-5856 fix) while the Dockerfile's builder stage still pinned
  `golang:1.26.4`, so `docker build .` failed before this change and had
  nothing to do with health probes. It is corrected alongside this work
  because it blocked verification; see `CHANGELOG.md`.
