# ADR 0001 — CI/CD supply-chain hardening: SHA-pinned Actions + GoReleaser + SBOM

- **Status:** Accepted
- **Date:** 2026-06-21
- **Deciders:** Frederic Jacquet

## Context

`pve_exporter` ships binaries and a container image to public consumers. A
release pipeline is an attractive supply-chain target: a compromised GitHub Action
runs with the workflow token and secrets, and an unverifiable release artifact
gives downstream users nothing to audit against. The project must also stay
consistent with its sibling exporters (`pflex_exporter`, `ppdd_exporter`), which
share a common CI approach.

## Decision

### 1. Pin every GitHub Action to a full commit SHA

All `uses:` references are pinned to a 40-character commit SHA with a trailing
`# vX.Y.Z` comment:

```yaml
- uses: actions/checkout@<40-char-sha> # v6.0.3
```

A `.github/dependabot.yml` (ecosystems `github-actions`, `gomod`, `docker`) reads
the version comment and bumps both the SHA and the comment, so pinning does not
mean stagnation.

### 2. Build releases with GoReleaser, attach a CycloneDX SBOM

A `.goreleaser.yaml` (schema v2) owns cross-compilation
(`linux,darwin × amd64,arm64`), `tar.gz` archives bundling `LICENSE`, `README.md`
and `config.yaml`, `checksums.txt`, the CycloneDX SBOM
(`cyclonedx-gomod mod -licenses -json`), and the GitHub Release with auto-generated
notes. Reproducible-build flags (`-trimpath`, `mod_timestamp`) are set. The
multi-arch GHCR image is built with build-time SBOM and provenance attestations.

### 3. Use the hardened reusable CI workflow

CI delegates to the shared, hardened reusable workflow **`fjacquet/ci@v1`**, which
all sibling exporters consume. This centralises lint/test/build/scan steps and
their SHA pins in one reviewed place rather than duplicating them per repo.

## Consequences

**Positive**

- Workflows execute only reviewed, immutable action code; tag-repoint attacks are
  neutralised, and Dependabot keeps pins fresh with reviewable PRs.
- Every release ships a verifiable SBOM and checksums; the GHCR image carries
  provenance/SBOM attestations.
- A single reusable workflow keeps the three sibling exporters consistent and
  cheap to maintain.

**Negative / trade-offs**

- Release CI depends on GoReleaser and a `cyclonedx-gomod` install step.
- Changes to shared CI behaviour land in `fjacquet/ci`, a second repo, rather than
  inline here.

## Alternatives considered

- **Keep mutable action tags** — rejected; cheap, high-value hardening.
- **Per-repo bespoke CI** — rejected to avoid drift across the sibling exporters.
- **GoReleaser default SBOM (syft)** — rejected to keep SBOM content identical to
  the `make sbom` artifact across siblings.
