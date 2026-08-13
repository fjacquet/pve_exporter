# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Entries for releases before this file existed were reconstructed from the git
history between tags.

## [Unreleased]

### Added

- `${VAR:-default}` fallbacks in config env references, ported from `pscale_exporter`.
  Shell / docker-compose semantics: the variable falls back when unset *or* empty, and
  such a reference never aborts startup. A bare `${VAR}` still fails loudly when the
  variable is *unset*; an exported-but-empty one expands to the empty string, as it
  always has.
  Credential fields are stricter: a field written as an env reference that resolves to
  nothing is now rejected, so a stray `PVE1_PASSWORD=` line fails at startup instead
  of authenticating with an empty credential. The error names only the config field:
  config-load failures are logged, and every part of a credential field — the variable
  name included — is potentially sensitive.

## [0.5.0] - 2026-08-01

### Added

- `/livez` and `/readyz` endpoints, both wired to a handler that always returns
  `200 OK` and reads no collection state, so a liveness or readiness probe can
  never restart or de-pool a healthy process.
- `HEALTHCHECK` against `http://127.0.0.1:9221/livez` in both `./Dockerfile` and
  `Dockerfile.goreleaser`, and a matching `healthcheck:` on the `pve_exporter`
  service in `docker-compose.yml` and `docker-compose.ghcr.yml`.
- `main_test.go` — the repo's first server-level test, covering `/livez`,
  `/readyz` and `/health`.
- This `CHANGELOG.md`.
- ADR-0009 recording the probe and container-healthcheck decision.

### Fixed

- The HTTP listener now binds **before** the first collection cycle. It
  previously waited on a synchronous startup collection, so with the default
  `collection.timeout: 25s` nothing — including `/livez` and `/readyz` — was
  reachable for 30s whenever a Proxmox cluster was unreachable. `--once` is
  unaffected and still runs a single synchronous cycle with no HTTP server.
- `server.uri` is validated against the routes the exporter registers itself.
  Setting it to `/`, `/health`, `/livez` or `/readyz` now fails startup with an
  explicit `server.uri: …` error instead of panicking `http.ServeMux`.
- `./Dockerfile`'s builder stage bumped from `golang:1.26.4` to `golang:1.26.5`
  to match the `go 1.26.5` directive in `go.mod` (added in v0.4.2 for
  GO-2026-5856) — `docker build .` had been broken since that release.
- Bumped `grpc` to v1.83.0, closing GO-2026-6061.

### Documentation

- `docs/deployment/docker.md` gained a concrete Kubernetes
  `livenessProbe`/`readinessProbe` snippet and an explanation of why no
  `startupProbe` or long `initialDelaySeconds` is warranted, now that the
  listener binds before the first collection cycle.

### Notes

- `/health` is unchanged. It already returned `200 OK` unconditionally; it is
  not, and never was, gated on collection state in this exporter.

## [0.4.2] - 2026-07-12

### Fixed

- Bumped the Go toolchain to 1.26.5 to pick up the fix for GO-2026-5856.

## [0.4.1] - 2026-07-11

### Fixed

- Used a `<ENV_VAR>` placeholder in the shipped `config.yaml` comment so the
  example no longer crashed the exporter at startup when copied verbatim.

### Documentation

- Added Grafana dashboard screenshots to the dashboards page.

## [0.4.0] - 2026-07-08

### Fixed

- `403`/`404` responses from *optional* endpoints no longer increment
  `pve_request_errors_total`. A deliberately-restricted API token used to pin
  the counter permanently above zero — a standing false alert — because the
  counter lived below the required/optional distinction.

### Added

- `GetOptional` on the PVE client: a best-effort GET that excludes
  expected-absence statuses from the error counter while still returning the
  error to the caller. Required endpoints keep counting every failure.

### Documentation

- ADR-0008 records the optional-endpoint error-counting policy.
- MkDocs site now uses the brand icon as favicon and logo.

### Testing

- Every optional endpoint is guarded against counting a `403`; added `404`
  collector coverage and resolved a staticcheck `QF1001` finding.

## [0.3.0] - 2026-06-21

### Added

- Node-level HA state from `/cluster/ha/status/current`.
- Self-observability metrics: a request-error counter and a collection-duration
  metric.

### Fixed

- Self-observability metrics now use a stable target id.

### Documentation

- ADR-0007 records the Feature-3 qdevice-state gap; metrics reference updated.

## [0.2.0] - 2026-06-21

### Added

- The six-dashboard Grafana suite: cluster overview, node detail, guest detail,
  storage, backup & DR, and HA & quorum, provisioned into a "Proxmox PVE"
  folder.
- A dashboard validation harness wired into the provisioning setup.

### Fixed

- Corrected the guest replication filter and the HA/lock precedence in the
  dashboards; follow-up review fixes to legends, the version join and table
  columns.

### Documentation

- Quickstart and CLI/validation (trace run) pages; the dashboard suite is
  documented. Internal planning artifacts are excluded from the published site.

## [0.1.1] - 2026-06-21

### Fixed

- Release pipeline switched to GoReleaser `dockers_v2` with a buildx
  `TARGETPLATFORM` Dockerfile, fixing the multi-arch image build.

## [0.1.0] - 2026-06-21

### Added

- Initial release: Prometheus + OTLP exporter for Proxmox VE.

[Unreleased]: https://github.com/fjacquet/pve_exporter/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/fjacquet/pve_exporter/compare/v0.4.3...v0.5.0
[0.4.2]: https://github.com/fjacquet/pve_exporter/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/fjacquet/pve_exporter/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/fjacquet/pve_exporter/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/fjacquet/pve_exporter/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/fjacquet/pve_exporter/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/fjacquet/pve_exporter/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/fjacquet/pve_exporter/releases/tag/v0.1.0
