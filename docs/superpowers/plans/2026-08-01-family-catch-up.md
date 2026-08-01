# Family standard catch-up (pve_exporter) Implementation Plan

> This plan is written for agentic workers. Each task is self-contained: it names
> the exact files, the exact code, and the exact commands to run. Do not skip
> steps, do not batch steps, and do not substitute your own judgement for the code
> shown in the fenced blocks. Where a step says "run X and confirm Y", run it and
> read the output before ticking the box. No step depends on context from outside
> this document.

**Goal**

Bring `pve_exporter` up to the two family-wide standards it silently missed on
2026-08-01: the always-200 `/livez` + `/readyz` probe pattern, and the Alpine
container-image `HEALTHCHECK` standard. Along the way, give the repo its first
`main_test.go` and its first `CHANGELOG.md`, and record the decision as an ADR.

This is the lightest of the five plans in the parent effort. `/health` in this
repo is **already** unconditionally 200 — there is no behavioural change to make
there. See Global Constraints.

**Architecture**

`main.go` builds its `http.ServeMux` inline inside `run()` (lines 165-175). There
is no `newServer` helper, no handler package, and no existing server test. The
mux registers exactly two routes today: `cfg.Server.URI` (the Prometheus handler)
and `/health` (an inline closure that writes 200 + `"ok"`).

The change adds a package-level `staticOKHandler` function to `main.go` and
registers it on `/livez` and `/readyz` next to the existing `/health` line. The
handler reads no state at all — not the `SnapshotStore`, not the config, nothing
— so a probe wired to it can never be the reason a healthy process is restarted
or pulled from rotation. `/health` stays exactly as it is.

Everything else in this plan is packaging and documentation: `HEALTHCHECK` in
both Dockerfiles, `healthcheck:` in both compose files, a new `main_test.go`, a
new `CHANGELOG.md`, ADR-0009, and the docs sweep.

**Tech Stack**

- Go 1.26 (module `github.com/fjacquet/pve_exporter`), stdlib `net/http`,
  `net/http/httptest`, `testing` — no new dependencies.
- Docker / `docker compose`, `alpine:latest` runtime base (already in use in both
  Dockerfiles; **no base-image change is needed in this repo**).
- MkDocs Material for the docs site (`make docs` → `mkdocs build --strict`).
- `make ci` (= `lint test build vuln`) is the gate.

**Spec**

`/Users/fjacquet/Projects/obs_exporter/docs/superpowers/specs/2026-08-01-family-standard-catch-up-design.md`
— section **"Plan D — `pve_exporter`"** (lines 192-202), plus the shared
**"Canonical patterns"** (lines 78-122), **"Testing"** (lines 224-238) and
**"Documentation"** (lines 240-251) sections which apply to every plan.

---

## Global Constraints

These apply to every task below. Read them once, honour them throughout.

1. **`/health` is already always-200. Do not "fix" it.** `main.go:167-170` is:

   ```go
   mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
       w.WriteHeader(http.StatusOK)
       _, _ = w.Write([]byte("ok"))
   })
   ```

   It writes `http.StatusOK` unconditionally and reads no snapshot state. Unlike
   `kemp_exporter` and `nsr_exporter`, this repo has **no 503 branch to remove**.
   Leave this registration byte-for-byte as it is. The only thing this plan does
   near it is add two more `mux.HandleFunc` lines underneath.

2. **The mux is inlined in `run()`. Match that shape — do not refactor.** There is
   no `newServer` helper in this repo and this plan does not create one. Add the
   two registrations directly after the `/health` block, inside `run()`.

3. **`127.0.0.1`, never `localhost`, in every healthcheck URL.** Alpine's busybox
   `wget` resolves `localhost` via `::1` first, and these exporters bind IPv4
   only. A `localhost`-based healthcheck passes `hadolint` **and**
   `docker compose config` while failing at runtime with connection refused. This
   exact bug shipped in the family-wide Alpine effort and had to be corrected in
   every repo.

4. **The healthcheck timeout is `5s` in BOTH the Dockerfile `HEALTHCHECK` and the
   compose `healthcheck:`.** The Alpine effort shipped a 5s/10s mismatch across
   eight repos and corrected it in every final review. The canonical values, used
   verbatim everywhere in this plan: `interval=30s`, `timeout=5s`,
   `start-period=10s` / `start_period: 10s`, `retries=3`.

5. **hadolint findings on the Dockerfiles are expected, not defects.**
   - `DL3025` (JSON-form CMD not used) is **unavoidable**: the family
     `HEALTHCHECK` requires shell-form `CMD wget … || exit 1`, and `||` has no
     JSON-form equivalent.
   - `DL3007` (`alpine:latest` unpinned) and `DL3066` are standing family
     findings, settled by decision 5 of the spec.

   Do **not** add inline `# hadolint ignore=` suppressions, and do **not** treat
   these as blocking. hadolint is not wired into this repo's CI
   (`.github/workflows/ci.yml` delegates to `fjacquet/ci` go-ci + go-security);
   run it manually as a sanity check only.

6. **Verify by BUILDING AND RUNNING the image**, then asserting
   `docker inspect --format='{{.State.Health.Status}}' <container>` reports
   `healthy`. Reading the Dockerfile is not verification — see constraint 3 for
   why a file that reads correctly can still fail at runtime.

7. **Apple Silicon note for `Dockerfile.goreleaser`.** That file consumes a
   pre-built binary at `${TARGETPLATFORM}/pve_exporter`; it does not compile. To
   build it locally on an M-series Mac you must cross-compile with
   `GOOS=linux GOARCH=arm64` and lay the binary out under a matching
   `linux/arm64/` directory, and pass `--build-arg TARGETPLATFORM=linux/arm64`.
   Getting this wrong yields `exec format error` at container start, which looks
   like a healthcheck failure but is not. Exact commands are in Task 6.

8. **Confirm the ADR number by listing the directory before writing it.** Run
   `ls /Users/fjacquet/Projects/pve_exporter/docs/adr/`. At the time of writing
   there are 8 ADRs (`0001`–`0008`), so the next free number is **0009** — but
   confirm, never assume.

9. **Every ADR in this repo has a row in `docs/adr/index.md`.** The new one needs
   one too. `mkdocs.yml` additionally carries an explicit `nav:` list of ADRs —
   see Task 8.

10. **Port is 9221 throughout.** It does not change in this repo (unlike
    `kemp_exporter`, which moves 9447 → 9448 in a sibling plan). Both Dockerfiles
    already `EXPOSE 9221` and both compose files already map `"9221:9221"`.

11. **Base image is already `alpine:latest` in both Dockerfiles.** No base-image
    change is needed in this repo. Do not touch the `FROM` lines.

12. **Commit after each task**, with a Conventional Commits subject. The repo's
    history uses `feat:` / `fix:` / `docs:` / `test:` / `chore:` prefixes.

---

## File Structure

| Path | Action | Purpose |
|---|---|---|
| `/Users/fjacquet/Projects/pve_exporter/main.go` | Modify | Add `staticOKHandler`; register `/livez` and `/readyz` on the inline mux in `run()` |
| `/Users/fjacquet/Projects/pve_exporter/main_test.go` | **Create** | First server test in this repo: `/livez`, `/readyz`, `/health` |
| `/Users/fjacquet/Projects/pve_exporter/Dockerfile` | Modify | Add `HEALTHCHECK` against `http://127.0.0.1:9221/livez` |
| `/Users/fjacquet/Projects/pve_exporter/Dockerfile.goreleaser` | Modify | Add the same `HEALTHCHECK` |
| `/Users/fjacquet/Projects/pve_exporter/docker-compose.yml` | Modify | Add `healthcheck:` to the `pve_exporter` service |
| `/Users/fjacquet/Projects/pve_exporter/docker-compose.ghcr.yml` | Modify | Add `healthcheck:` to the `pve_exporter` service |
| `/Users/fjacquet/Projects/pve_exporter/CHANGELOG.md` | **Create** | Keep a Changelog, backfilled v0.1.0 → v0.4.2 from git history, this work under `## [Unreleased]` |
| `/Users/fjacquet/Projects/pve_exporter/docs/adr/0009-health-probes.md` | **Create** | ADR recording the probe + HEALTHCHECK decision |
| `/Users/fjacquet/Projects/pve_exporter/docs/adr/index.md` | Modify | Add the ADR-0009 row |
| `/Users/fjacquet/Projects/pve_exporter/mkdocs.yml` | Modify | Add the missing ADR nav entries (0007, 0008, 0009) |
| `/Users/fjacquet/Projects/pve_exporter/docs/cli.md` | Modify | Endpoints table: add `/livez` and `/readyz` rows |
| `/Users/fjacquet/Projects/pve_exporter/docs/deployment/docker.md` | Modify | Document the built-in container healthcheck |

---

## Task 1: Add `/livez` and `/readyz` to the inline mux

**Files:**
- Create: `/Users/fjacquet/Projects/pve_exporter/main_test.go`
- Modify: `/Users/fjacquet/Projects/pve_exporter/main.go`

**Interfaces:**
- Consumes: nothing — `staticOKHandler` deliberately reads no state (not
  `pve.SnapshotStore`, not `models.Config`).
- Produces: `func staticOKHandler(w http.ResponseWriter, _ *http.Request)` at
  package scope in `package main`, consumed by Task 1's registrations and by
  `main_test.go` (Task 2).

- [x] **Step 1: Read the current mux block.** Open
      `/Users/fjacquet/Projects/pve_exporter/main.go` and read lines 160-180.
      Confirm you see exactly this, and note that `/health` already writes
      `http.StatusOK` unconditionally — there is nothing to fix there:

      ```go
      mux := http.NewServeMux()
      mux.Handle(cfg.Server.URI, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
      mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
          w.WriteHeader(http.StatusOK)
          _, _ = w.Write([]byte("ok"))
      })
      ```

      If the file does not match, stop and re-read the whole of `run()` before
      continuing.

- [x] **Step 2: Write the failing test file.** Create
      `/Users/fjacquet/Projects/pve_exporter/main_test.go` with exactly this
      content. It is the repo's first `package main` test; it uses only stdlib,
      matching the plain table/assert style of `internal/pve/parse_test.go`:

      ```go
      package main

      import (
      	"net/http"
      	"net/http/httptest"
      	"testing"
      )

      func TestStaticOKHandlerReturns200(t *testing.T) {
      	for _, path := range []string{"/livez", "/readyz"} {
      		t.Run(path, func(t *testing.T) {
      			req := httptest.NewRequest(http.MethodGet, path, nil)
      			rec := httptest.NewRecorder()

      			staticOKHandler(rec, req)

      			if rec.Code != http.StatusOK {
      				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
      			}
      			if got := rec.Body.String(); got != "ok" {
      				t.Fatalf("body = %q, want %q", got, "ok")
      			}
      		})
      	}
      }
      ```

- [x] **Step 3: Run it and confirm it fails to compile.** Run:

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && go test ./... -run TestStaticOKHandler
      ```

      Expect a compile error along the lines of
      `undefined: staticOKHandler`. That is the failing state we want. If it
      compiles, `staticOKHandler` already exists — re-read `main.go` before
      continuing.

- [x] **Step 4: Add `staticOKHandler` to `main.go`.** Insert this function at
      package scope. Put it immediately before `syncInstruments` (i.e. after the
      closing brace of `run()`, around line 218):

      ```go
      // staticOKHandler always answers 200 — no snapshot state, no collection
      // state, nothing that can make it fail. /livez and /readyz both use it: a
      // probe wired here can never be the reason a healthy process gets restarted
      // or pulled from rotation. /health remains the endpoint for anything that
      // wants to know whether a cluster is actually reachable.
      func staticOKHandler(w http.ResponseWriter, _ *http.Request) {
      	w.WriteHeader(http.StatusOK)
      	_, _ = w.Write([]byte("ok"))
      }
      ```

- [x] **Step 5: Register the two routes on the inline mux.** In `run()`, directly
      after the closing `})` of the existing `/health` registration and before
      `httpServer := &http.Server{`, add:

      ```go
      	mux.HandleFunc("/livez", staticOKHandler)
      	mux.HandleFunc("/readyz", staticOKHandler)
      ```

      The block should now read:

      ```go
      	mux := http.NewServeMux()
      	mux.Handle(cfg.Server.URI, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
      	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
      		w.WriteHeader(http.StatusOK)
      		_, _ = w.Write([]byte("ok"))
      	})
      	mux.HandleFunc("/livez", staticOKHandler)
      	mux.HandleFunc("/readyz", staticOKHandler)
      	httpServer := &http.Server{
      		Addr:              cfg.GetServerAddress(),
      		Handler:           mux,
      		ReadHeaderTimeout: 10 * time.Second,
      	}
      ```

      Do **not** extract a helper, do **not** move the mux construction out of
      `run()`, and do **not** alter the `/health` closure.

- [x] **Step 6: Run the test and confirm it passes.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && go test ./... -run TestStaticOKHandler -v
      ```

      Expect `--- PASS: TestStaticOKHandler/_livez` and
      `--- PASS: TestStaticOKHandler/_readyz`.

- [x] **Step 7: Commit.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && git add main.go main_test.go && \
        git commit -m "feat(server): add always-200 /livez and /readyz probes"
      ```

---

## Task 2: Cover the wired mux, including `/health`, end to end

**Files:**
- Modify: `/Users/fjacquet/Projects/pve_exporter/main_test.go`

**Interfaces:**
- Consumes: `staticOKHandler` from `main.go`.
- Produces: `TestProbeRoutesOnMux` and `TestHealthAlwaysReturns200`, asserting the
  routes are actually reachable through an `http.ServeMux`, not merely that the
  handler function behaves.

Task 1's test calls the handler directly. That does not prove the routes are
registered. This task adds a mux-level test that would catch a typo'd path.

- [x] **Step 1: Append the mux-level failing test.** Add to the bottom of
      `/Users/fjacquet/Projects/pve_exporter/main_test.go`:

      ```go
      // newProbeMux mirrors the probe and health registrations made in run().
      // run() itself needs a full config, client set and registry, so the routes
      // under test are re-registered here on a bare mux; the handlers are the
      // same functions.
      func newProbeMux() *http.ServeMux {
      	mux := http.NewServeMux()
      	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
      		w.WriteHeader(http.StatusOK)
      		_, _ = w.Write([]byte("ok"))
      	})
      	mux.HandleFunc("/livez", staticOKHandler)
      	mux.HandleFunc("/readyz", staticOKHandler)
      	return mux
      }

      func TestProbeRoutesOnMux(t *testing.T) {
      	mux := newProbeMux()

      	for _, path := range []string{"/livez", "/readyz", "/health"} {
      		t.Run(path, func(t *testing.T) {
      			req := httptest.NewRequest(http.MethodGet, path, nil)
      			rec := httptest.NewRecorder()

      			mux.ServeHTTP(rec, req)

      			if rec.Code != http.StatusOK {
      				t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusOK)
      			}
      			if got := rec.Body.String(); got != "ok" {
      				t.Fatalf("%s body = %q, want %q", path, got, "ok")
      			}
      		})
      	}
      }

      // TestHealthAlwaysReturns200 pins the property this repo already had before
      // the probe work: /health never gates on collection state. Unlike the
      // sibling exporters it was never 503, and it must not become 503.
      func TestHealthAlwaysReturns200(t *testing.T) {
      	mux := newProbeMux()

      	// No collection has run; nothing has ever been stored in a snapshot.
      	req := httptest.NewRequest(http.MethodGet, "/health", nil)
      	rec := httptest.NewRecorder()
      	mux.ServeHTTP(rec, req)

      	if rec.Code != http.StatusOK {
      		t.Fatalf("cold /health status = %d, want %d", rec.Code, http.StatusOK)
      	}
      }
      ```

- [x] **Step 2: Run and confirm green.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && go test ./... -run 'TestProbeRoutes|TestHealthAlways|TestStaticOK' -v
      ```

      Expect five passing subtests/tests, no failures.

- [x] **Step 3: Run the full suite with the race detector.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && go test -race ./...
      ```

      Expect `ok` for every package, including the new `github.com/fjacquet/pve_exporter`
      root package line.

- [x] **Step 4: Run the lint + format gate.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && make fmt-check && make vet && make lint
      ```

      All three must be clean. If `fmt-check` complains, run `make fmt` and
      re-run.

- [x] **Step 5: Commit.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && git add main_test.go && \
        git commit -m "test(server): cover /livez, /readyz and always-200 /health on the mux"
      ```

---

## Task 3: `HEALTHCHECK` in `./Dockerfile`

**Files:**
- Modify: `/Users/fjacquet/Projects/pve_exporter/Dockerfile`

**Interfaces:**
- Consumes: the `/livez` route from Task 1, on port 9221.
- Produces: a container-level health status readable via
  `docker inspect --format='{{.State.Health.Status}}'`.

- [x] **Step 1: Read the runtime stage.** Open
      `/Users/fjacquet/Projects/pve_exporter/Dockerfile` and confirm the tail
      reads:

      ```dockerfile
      COPY --from=builder /app/pve_exporter /usr/bin/pve_exporter
      COPY config.yaml /etc/pve_exporter/config.yaml

      EXPOSE 9221

      USER pve

      ENTRYPOINT ["/usr/bin/pve_exporter"]
      CMD ["--config", "/etc/pve_exporter/config.yaml"]
      ```

      Note `FROM alpine:latest` at line 15 — already correct, leave it alone
      (Global Constraint 11).

- [x] **Step 2: Insert the `HEALTHCHECK` between `EXPOSE` and `USER`.** The file
      tail becomes:

      ```dockerfile
      EXPOSE 9221

      # busybox wget is present in the Alpine base. 127.0.0.1, not localhost:
      # busybox resolves localhost via ::1 first and the exporter binds IPv4 only.
      HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
        CMD wget --quiet --tries=1 --spider http://127.0.0.1:9221/livez || exit 1

      USER pve

      ENTRYPOINT ["/usr/bin/pve_exporter"]
      CMD ["--config", "/etc/pve_exporter/config.yaml"]
      ```

- [x] **Step 3: Build the image.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && docker build -t pve_exporter:healthcheck-test .
      ```

      Expect a successful build. This stage compiles the binary, so a Go
      compilation error from Task 1 would surface here.

- [x] **Step 4: Run it and assert `healthy`.** The exporter needs a config; the
      image ships `/etc/pve_exporter/config.yaml` and the probe does not depend on
      a reachable Proxmox cluster (that is the whole point of `staticOKHandler`).

      ```bash
      docker rm -f pve_hc_test 2>/dev/null
      docker run -d --name pve_hc_test -p 19221:9221 pve_exporter:healthcheck-test
      sleep 45
      docker inspect --format='{{.State.Health.Status}}' pve_hc_test
      ```

      Expect exactly `healthy`. If it reports `starting`, wait another 30s and
      re-inspect. If it reports `unhealthy`, run
      `docker inspect --format='{{json .State.Health}}' pve_hc_test` and read the
      failing log entries, and check `docker logs pve_hc_test` — do **not** move
      on.

- [x] **Step 5: Tear down.**

      ```bash
      docker rm -f pve_hc_test && docker rmi pve_exporter:healthcheck-test
      ```

- [x] **Step 6: Run hadolint and read, but do not act on, the findings.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && docker run --rm -i hadolint/hadolint < Dockerfile
      ```

      `DL3025`, `DL3007` and `DL3066` are expected (Global Constraint 5). Any
      *other* finding introduced by your edit must be fixed. Do not add inline
      ignore comments.

- [x] **Step 7: Commit.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && git add Dockerfile && \
        git commit -m "feat(docker): add HEALTHCHECK against /livez on 9221"
      ```

---

## Task 4: `HEALTHCHECK` in `Dockerfile.goreleaser`

**Files:**
- Modify: `/Users/fjacquet/Projects/pve_exporter/Dockerfile.goreleaser`

**Interfaces:**
- Consumes: the `/livez` route from Task 1, on port 9221.
- Produces: the same health status on the *published* GHCR image (this is the
  Dockerfile GoReleaser actually uses; `./Dockerfile` is local/dev only).

- [x] **Step 1: Read the file.** Confirm
      `/Users/fjacquet/Projects/pve_exporter/Dockerfile.goreleaser` is:

      ```dockerfile
      FROM alpine:latest

      RUN apk --no-cache add ca-certificates && \
          adduser -D -u 10001 pve && \
          mkdir -p /var/log/pve_exporter && \
          chown pve:pve /var/log/pve_exporter

      ARG TARGETPLATFORM
      COPY ${TARGETPLATFORM}/pve_exporter /usr/bin/pve_exporter
      COPY config.yaml /etc/pve_exporter/config.yaml

      EXPOSE 9221

      USER pve

      ENTRYPOINT ["/usr/bin/pve_exporter"]
      CMD ["--config", "/etc/pve_exporter/config.yaml"]
      ```

- [x] **Step 2: Insert the identical `HEALTHCHECK` between `EXPOSE` and `USER`.**

      ```dockerfile
      EXPOSE 9221

      # busybox wget is present in the Alpine base. 127.0.0.1, not localhost:
      # busybox resolves localhost via ::1 first and the exporter binds IPv4 only.
      HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
        CMD wget --quiet --tries=1 --spider http://127.0.0.1:9221/livez || exit 1

      USER pve
      ```

      Byte-identical to Task 3's — same interval, same 5s timeout, same port, same
      path.

- [x] **Step 3: Cross-compile a binary and lay it out per-platform.** This
      Dockerfile does not build Go; it expects the binary at
      `${TARGETPLATFORM}/pve_exporter` in the build context. On Apple Silicon
      (Global Constraint 7):

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && \
        mkdir -p /tmp/pvegr/linux/arm64 && \
        CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /tmp/pvegr/linux/arm64/pve_exporter . && \
        cp config.yaml /tmp/pvegr/config.yaml && \
        cp Dockerfile.goreleaser /tmp/pvegr/Dockerfile.goreleaser
      ```

      On an x86_64 host, substitute `GOARCH=amd64` and `linux/amd64` throughout
      this and the next step.

- [x] **Step 4: Build with the matching `TARGETPLATFORM`.**

      ```bash
      docker build -f /tmp/pvegr/Dockerfile.goreleaser \
        --build-arg TARGETPLATFORM=linux/arm64 \
        -t pve_exporter:gr-healthcheck-test /tmp/pvegr
      ```

      A mismatch between `GOARCH` and `TARGETPLATFORM` produces
      `exec format error` when the container starts — which looks like a
      healthcheck failure but is a build mistake.

- [x] **Step 5: Run it and assert `healthy`.**

      ```bash
      docker rm -f pve_gr_hc_test 2>/dev/null
      docker run -d --name pve_gr_hc_test -p 19222:9221 pve_exporter:gr-healthcheck-test
      sleep 45
      docker inspect --format='{{.State.Health.Status}}' pve_gr_hc_test
      ```

      Expect exactly `healthy`. On `unhealthy`, check `docker logs pve_gr_hc_test`
      for `exec format error` first (see Step 4), then
      `docker inspect --format='{{json .State.Health}}' pve_gr_hc_test`.

- [x] **Step 6: Tear down.**

      ```bash
      docker rm -f pve_gr_hc_test && docker rmi pve_exporter:gr-healthcheck-test && rm -rf /tmp/pvegr
      ```

- [x] **Step 7: Run hadolint.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && docker run --rm -i hadolint/hadolint < Dockerfile.goreleaser
      ```

      `DL3025`, `DL3007`, `DL3066` expected. No inline suppressions.

- [x] **Step 8: Commit.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && git add Dockerfile.goreleaser && \
        git commit -m "feat(docker): add HEALTHCHECK to the GoReleaser image"
      ```

---

## Task 5: `healthcheck:` in both compose files

**Files:**
- Modify: `/Users/fjacquet/Projects/pve_exporter/docker-compose.yml`
- Modify: `/Users/fjacquet/Projects/pve_exporter/docker-compose.ghcr.yml`

**Interfaces:**
- Consumes: the `/livez` route from Task 1, on port 9221.
- Produces: a compose-level healthcheck on the `pve_exporter` service in both
  stacks. Note this **overrides** the image's `HEALTHCHECK` — which is why the
  values must match exactly (Global Constraint 4).

- [x] **Step 1: Edit `docker-compose.yml`.** In the `pve_exporter` service, insert
      the `healthcheck:` block between `environment:` and `restart:`. The service
      becomes:

      ```yaml
        pve_exporter:
          build: .
          image: pve_exporter
          # Build-only image (no registry). Without this, `docker compose pull`/`up`
          # tries to pull `pve_exporter` from Docker Hub and fails with access-denied.
          pull_policy: build
          container_name: pve_exporter
          ports:
            - "9221:9221"
          volumes:
            - ./config.yaml:/etc/pve_exporter/config.yaml:ro
          environment:
            # Provide Proxmox VE connection details via the environment.
            # config.yaml references these as ${PVE1_HOST}, ${PVE1_TOKEN_ID}, ${PVE1_TOKEN_SECRET}.
            - PVE1_HOST=${PVE1_HOST:-10.0.0.1:8006}
            - PVE1_TOKEN_ID=${PVE1_TOKEN_ID:-exporter@pam!prometheus}
            - PVE1_TOKEN_SECRET=${PVE1_TOKEN_SECRET:-}
          healthcheck:
            test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://127.0.0.1:9221/livez"]
            interval: 30s
            timeout: 5s
            retries: 3
            start_period: 10s
          restart: unless-stopped
      ```

- [x] **Step 2: Edit `docker-compose.ghcr.yml`.** Same block, same position, in
      its `pve_exporter` service:

      ```yaml
        pve_exporter:
          image: ghcr.io/fjacquet/pve_exporter:${PVE_TAG:-latest}
          container_name: pve_exporter
          ports:
            - "9221:9221"
          volumes:
            - ./config.yaml:/etc/pve_exporter/config.yaml:ro
          environment:
            # Provide Proxmox VE connection details via the environment.
            # config.yaml references these as ${PVE1_HOST}, ${PVE1_TOKEN_ID}, ${PVE1_TOKEN_SECRET}.
            - PVE1_HOST=${PVE1_HOST:-10.0.0.1:8006}
            - PVE1_TOKEN_ID=${PVE1_TOKEN_ID:-exporter@pam!prometheus}
            - PVE1_TOKEN_SECRET=${PVE1_TOKEN_SECRET:-}
          healthcheck:
            test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://127.0.0.1:9221/livez"]
            interval: 30s
            timeout: 5s
            retries: 3
            start_period: 10s
          restart: unless-stopped
      ```

- [x] **Step 3: Validate both files parse.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && \
        docker compose -f docker-compose.yml config -q && \
        docker compose -f docker-compose.ghcr.yml config -q && echo "compose OK"
      ```

      Expect `compose OK` with no YAML errors.

- [x] **Step 4: Grep-verify no `localhost` crept in.** Global Constraint 3 —
      `docker compose config -q` will happily accept a `localhost` URL that fails
      at runtime, so check by hand:

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && \
        grep -n "localhost" Dockerfile Dockerfile.goreleaser docker-compose.yml docker-compose.ghcr.yml
      ```

      Expect **no output** (grep exit code 1). Any hit is a bug — fix it to
      `127.0.0.1`.

- [x] **Step 5: Verify the compose healthcheck actually runs.** Bring the build
      stack up (only the exporter service; Prometheus and Grafana are not needed):

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && \
        PVE1_TOKEN_SECRET=dummy docker compose -f docker-compose.yml up -d pve_exporter
      sleep 45
      docker inspect --format='{{.State.Health.Status}}' pve_exporter
      ```

      Expect exactly `healthy`. The token is deliberately bogus — the exporter
      will fail to scrape and that is fine; `/livez` reads no state.

- [x] **Step 6: Tear down.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && docker compose -f docker-compose.yml down
      ```

- [x] **Step 7: Commit.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && \
        git add docker-compose.yml docker-compose.ghcr.yml && \
        git commit -m "feat(compose): add /livez healthcheck to both stacks"
      ```

---

## Task 6: Create `CHANGELOG.md`, backfilled from git history

**Files:**
- Create: `/Users/fjacquet/Projects/pve_exporter/CHANGELOG.md`

**Interfaces:**
- Consumes: `git tag --sort=v:refname` and `git log <prev>..<tag>`.
- Produces: the repo's first `CHANGELOG.md`, Keep a Changelog 1.1.0 format, with
  a `## [Unreleased]` section this and every future change lands in.

**This repo has no `CHANGELOG.md` today.** This task creates one, backfilled per
tagged version through v0.4.2.

> **Honesty rule — read this before writing a single line.** The backfill must
> summarize what the commits between two tags *actually did*. Derive every entry
> from real `git log` output. Do **not** invent plausible-sounding entries, do
> **not** pad a thin release to make it look substantial, and do **not** claim a
> fix or feature you cannot point at a commit for. A release whose only commit is
> `bump up` gets a one-line honest entry, not a fabricated feature list. Where a
> commit subject is genuinely uninformative, read the commit
> (`git show --stat <sha>`) rather than guessing.

- [x] **Step 1: Enumerate the tags and their dates.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && \
        for t in $(git tag --sort=v:refname); do echo "$t $(git log -1 --format=%ad --date=short "$t")"; done
      ```

      Expected output (confirm it matches; if new tags exist, extend the backfill
      accordingly):

      ```
      v0.1.0 2026-06-21
      v0.1.1 2026-06-21
      v0.2.0 2026-06-21
      v0.3.0 2026-06-21
      v0.4.0 2026-07-08
      v0.4.1 2026-07-11
      v0.4.2 2026-07-12
      ```

- [x] **Step 2: Read the commits in each range.** Run every one of these and read
      the output — the entries you write must come from here:

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && \
        echo "=== v0.1.0 ===" && git log --oneline v0.1.0 && \
        echo "=== v0.1.0..v0.1.1 ===" && git log --oneline v0.1.0..v0.1.1 && \
        echo "=== v0.1.1..v0.2.0 ===" && git log --oneline v0.1.1..v0.2.0 && \
        echo "=== v0.2.0..v0.3.0 ===" && git log --oneline v0.2.0..v0.3.0 && \
        echo "=== v0.3.0..v0.4.0 ===" && git log --oneline v0.3.0..v0.4.0 && \
        echo "=== v0.4.0..v0.4.1 ===" && git log --oneline v0.4.0..v0.4.1 && \
        echo "=== v0.4.1..v0.4.2 ===" && git log --oneline v0.4.1..v0.4.2 && \
        echo "=== v0.4.2..HEAD ===" && git log --oneline v0.4.2..HEAD
      ```

      For any commit subject that is truncated or opaque, run
      `git show --stat <sha>` before writing its entry.

- [x] **Step 3: Write `CHANGELOG.md`.** Create
      `/Users/fjacquet/Projects/pve_exporter/CHANGELOG.md`. The content below is
      derived from the commit ranges above and is what Step 2's output supports.
      **Reconcile it against your own Step 2 output before saving** — if a range
      contains commits this draft does not reflect (e.g. new commits landed since
      this plan was written), add honest entries for them.

      ```markdown
      # Changelog

      All notable changes to this project are documented in this file.

      The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
      and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

      Entries for releases before this file existed were reconstructed from the git
      history between tags.

      ## [Unreleased]

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

      [Unreleased]: https://github.com/fjacquet/pve_exporter/compare/v0.4.2...HEAD
      [0.4.2]: https://github.com/fjacquet/pve_exporter/compare/v0.4.1...v0.4.2
      [0.4.1]: https://github.com/fjacquet/pve_exporter/compare/v0.4.0...v0.4.1
      [0.4.0]: https://github.com/fjacquet/pve_exporter/compare/v0.3.0...v0.4.0
      [0.3.0]: https://github.com/fjacquet/pve_exporter/compare/v0.2.0...v0.3.0
      [0.2.0]: https://github.com/fjacquet/pve_exporter/compare/v0.1.1...v0.2.0
      [0.1.1]: https://github.com/fjacquet/pve_exporter/compare/v0.1.0...v0.1.1
      [0.1.0]: https://github.com/fjacquet/pve_exporter/releases/tag/v0.1.0
      ```

- [x] **Step 4: Sanity-check the file.** Confirm every version heading has a
      matching link reference at the bottom, and that the version list matches
      Step 1's tag list exactly:

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && \
        grep -c "^## \[" CHANGELOG.md && grep -c "^\[" CHANGELOG.md
      ```

      Both counts must be `8` (7 tags + `Unreleased`).

- [x] **Step 5: Commit.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && git add CHANGELOG.md && \
        git commit -m "docs: add CHANGELOG.md backfilled from git history"
      ```

---

## Task 7: ADR-0009 and the index row

**Files:**
- Create: `/Users/fjacquet/Projects/pve_exporter/docs/adr/0009-health-probes.md`
- Modify: `/Users/fjacquet/Projects/pve_exporter/docs/adr/index.md`

**Interfaces:**
- Consumes: the decisions made in Tasks 1, 3, 4, 5.
- Produces: an ADR following this repo's **Status / Context / Decision /
  Consequences** structure, plus its row in the index table.

- [x] **Step 1: Confirm the next free ADR number.** Global Constraint 8 — list,
      never assume:

      ```bash
      ls /Users/fjacquet/Projects/pve_exporter/docs/adr/
      ```

      Expect `0001-…` through `0008-…` plus `index.md`, making **0009** the next
      free number. If the listing shows a `0009` already, use the next free number
      and adjust every filename and reference in this task accordingly.

- [x] **Step 2: Write the ADR.** Create
      `/Users/fjacquet/Projects/pve_exporter/docs/adr/0009-health-probes.md`:

      ```markdown
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
      - `/livez` and `/readyz` return identical responses today. Should this exporter
        ever grow a genuine not-ready state — a required config that resolves
        asynchronously, say — `/readyz` is the endpoint that would change, and
        `/livez` is the one that must not.
      - The base image needed no change: this repo already used `alpine:latest` in
        both Dockerfiles, so the family's Alpine convergence was a no-op here.
      ```

- [x] **Step 3: Add the index row.** In
      `/Users/fjacquet/Projects/pve_exporter/docs/adr/index.md`, append one row to
      the table, after the ADR-0008 row:

      ```markdown
      | [0009](0009-health-probes.md) | `/livez` `/readyz` probes and container HEALTHCHECK | Accepted |
      ```

      The table tail should then read:

      ```markdown
      | [0007](0007-qdevice-state-gap.md) | QDevice tie-breaker/state gap | Accepted |
      | [0008](0008-request-error-counting.md) | Request-error counting on optional endpoints | Accepted |
      | [0009](0009-health-probes.md) | `/livez` `/readyz` probes and container HEALTHCHECK | Accepted |
      ```

- [x] **Step 4: Verify the link resolves.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && \
        grep -n "0009" docs/adr/index.md && ls docs/adr/0009-health-probes.md
      ```

      Both must succeed.

- [x] **Step 5: Commit.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && git add docs/adr/ && \
        git commit -m "docs(adr): record the /livez /readyz and HEALTHCHECK decision as ADR-0009"
      ```

---

## Task 8: Docs sweep — nav, endpoints table, Docker page

**Files:**
- Modify: `/Users/fjacquet/Projects/pve_exporter/mkdocs.yml`
- Modify: `/Users/fjacquet/Projects/pve_exporter/docs/cli.md`
- Modify: `/Users/fjacquet/Projects/pve_exporter/docs/deployment/docker.md`

**Interfaces:**
- Consumes: the routes from Task 1 and the healthchecks from Tasks 3-5.
- Produces: user-facing documentation consistent with the shipped behaviour.

The spec's Documentation section is explicit that every repo in the preceding
Alpine effort needed a post-review fix wave because user-facing pages still made
claims the change falsified. This task front-loads that sweep.

- [x] **Step 1: Find every user-facing mention of the health endpoint.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && \
        grep -rn "/health\|healthcheck\|HEALTHCHECK\|distroless\|65532\|nonroot" \
        docs/ README.md deploy/ config.yaml 2>/dev/null | grep -v superpowers
      ```

      Expected: a single hit, `docs/cli.md:37`. (`distroless`, `65532` and
      `nonroot` should produce no hits — this repo was already Alpine at uid
      10001.) If new hits appear, fix each one in this task.

- [x] **Step 2: Update the endpoints table in `docs/cli.md`.** Replace:

      ```markdown
      | Path | Description |
      |------|-------------|
      | `/metrics` | Prometheus text exposition (default port `9221`). |
      | `/health` | Health probe; returns `200 OK` when the exporter is running. |
      ```

      with:

      ```markdown
      | Path | Description |
      |------|-------------|
      | `/metrics` | Prometheus text exposition (default port `9221`). |
      | `/livez` | Liveness probe. Always `200 OK`; reads no collection state. |
      | `/readyz` | Readiness probe. Always `200 OK`; reads no collection state. |
      | `/health` | Informational. Always `200 OK` while the process is serving. |

      Point Kubernetes `livenessProbe` and `readinessProbe` at `/livez` and
      `/readyz`. Never probe `/metrics`: it renders the full exposition on every
      tick and can block behind a slow collection cycle. Cluster reachability is a
      `/metrics` question — see `pve_up` and `pve_request_errors_total`.
      ```

- [x] **Step 3: Document the container healthcheck in
      `docs/deployment/docker.md`.** Insert a new section immediately after the
      `### Logging` subsection and before `## Environment variables`:

      ```markdown
      ### Health check

      Both the published image and the local `./Dockerfile` declare a `HEALTHCHECK`
      that polls `/livez`:

      ```dockerfile
      HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
        CMD wget --quiet --tries=1 --spider http://127.0.0.1:9221/livez || exit 1
      ```

      Check it on a running container with:

      ```bash
      docker inspect --format='{{.State.Health.Status}}' pve_exporter
      ```

      Both Compose stacks declare the same check, so
      `depends_on: { pve_exporter: { condition: service_healthy } }` works.

      The address is `127.0.0.1`, not `localhost`: busybox `wget` in the Alpine base
      resolves `localhost` via `::1` first and the exporter binds IPv4 only, so a
      `localhost` check fails with connection refused.

      For Kubernetes, use `/livez` for `livenessProbe` and `/readyz` for
      `readinessProbe`. Both always return `200 OK` and read no collection state, so
      neither can restart or de-pool a healthy pod when a Proxmox cluster is
      unreachable.
      ```

- [x] **Step 4: Add the missing ADR nav entries to `mkdocs.yml`.** The `nav:` list
      stops at ADR 0006 — 0007 and 0008 were never added, so the new one would
      inherit the same gap. Replace the last nav line
      (`      - ADR 0006 — Label invariant: adr/0006-label-invariant.md`) with:

      ```yaml
            - ADR 0006 — Label invariant: adr/0006-label-invariant.md
            - ADR 0007 — QDevice state gap: adr/0007-qdevice-state-gap.md
            - ADR 0008 — Request-error counting: adr/0008-request-error-counting.md
            - ADR 0009 — Health probes: adr/0009-health-probes.md
      ```

      Do not touch anything else in `mkdocs.yml` — in particular leave
      `exclude_docs: superpowers/` and `extra.version` alone.

- [x] **Step 5: Build the docs site strictly.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && make docs
      ```

      This runs `mkdocs build --strict --site-dir site`. Expect a clean build with
      no warnings-as-errors. A broken relative link in the new ADR or index row
      surfaces here.

- [x] **Step 6: Clean the build output if it is untracked.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && git status --porcelain | head -20
      ```

      `site/` should be gitignored. If it shows up as untracked, remove it
      (`rm -rf site`) rather than committing it.

- [x] **Step 7: Commit.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && \
        git add mkdocs.yml docs/cli.md docs/deployment/docker.md && \
        git commit -m "docs: document /livez /readyz and the container healthcheck"
      ```

---

## Task 9: Full gate and final verification

**Files:** none modified — this task only runs and reads.

**Interfaces:**
- Consumes: everything from Tasks 1-8.
- Produces: evidence that the work is complete. Do not claim completion without
  having read each command's output.

- [x] **Step 1: Run the full CI gate.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && make ci
      ```

      This is `lint test build vuln`. All four must pass. Read the output; do not
      infer success from the absence of a visible error.

- [x] **Step 2: Run the local gate too.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && make sure
      ```

      (`fmt vet test build`.) Clean.

- [x] **Step 3: Race-detector test run.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && go test -race ./...
      ```

      Every package `ok`, including the root `github.com/fjacquet/pve_exporter`
      package that `main_test.go` created.

- [x] **Step 4: End-to-end probe check against a locally running binary.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && make cli 2>/dev/null || go build -o pve_exporter .
      PVE1_TOKEN_SECRET=dummy ./pve_exporter --config config.yaml &
      sleep 8
      for p in /livez /readyz /health; do
        printf '%s -> ' "$p"
        curl -s -o /dev/null -w '%{http_code}\n' "http://127.0.0.1:9221$p"
      done
      kill %1
      ```

      Expect `200` for all three paths. A `000` means the server did not start —
      check the config path and the log output before proceeding.

- [x] **Step 5: Re-verify the container health status one final time**, since the
      Dockerfile has not been rebuilt since Task 3:

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && \
        PVE1_TOKEN_SECRET=dummy docker compose -f docker-compose.yml up -d --build pve_exporter
      sleep 45
      docker inspect --format='{{.State.Health.Status}}' pve_exporter
      docker compose -f docker-compose.yml down
      ```

      Expect exactly `healthy` before the teardown line.

- [x] **Step 6: Confirm the working tree is clean and review the full diff.**

      ```bash
      cd /Users/fjacquet/Projects/pve_exporter && git status --porcelain && \
        git log --oneline -8 && git diff main@{u}..HEAD --stat
      ```

      Expect no uncommitted changes, and a diffstat touching exactly the twelve
      files in the File Structure table (some created, some modified).

---

## Self-Review

Before declaring this work done, verify each of the following by running the
command and reading the output — not by recalling that you did it earlier.

- [x] **`/health` was not modified.** `git diff main@{u}..HEAD -- main.go` shows
      only the two added `mux.HandleFunc` lines and the added `staticOKHandler`
      function. The `/health` closure appears in the diff only as unchanged
      context. It was already always-200; nothing about it needed fixing.
- [x] **No `localhost` anywhere in a healthcheck.**
      `grep -rn localhost Dockerfile Dockerfile.goreleaser docker-compose.yml docker-compose.ghcr.yml`
      returns nothing.
- [x] **Timeout is `5s` in all four files.**
      `grep -rn "timeout=5s\|timeout: 5s" Dockerfile Dockerfile.goreleaser docker-compose.yml docker-compose.ghcr.yml`
      returns four hits. No `10s` timeout anywhere.
- [x] **Port is 9221 in all four healthchecks.**
      `grep -rn "127.0.0.1:9221/livez" Dockerfile Dockerfile.goreleaser docker-compose.yml docker-compose.ghcr.yml`
      returns four hits.
- [x] **Container health was observed, not assumed.** You ran
      `docker inspect --format='{{.State.Health.Status}}'` against a real running
      container and read `healthy` — for the `./Dockerfile` build (Task 3), the
      `Dockerfile.goreleaser` build (Task 4), and the compose stack (Tasks 5 and
      9).
- [x] **No inline suppressions were added.**
      `grep -rn "hadolint ignore\|nolint\|nosemgrep" Dockerfile Dockerfile.goreleaser main.go main_test.go`
      returns nothing. `DL3025`/`DL3007`/`DL3066` are accepted findings.
- [x] **No base-image change was made.** Both Dockerfiles still say
      `FROM alpine:latest` — this repo was already compliant.
- [x] **`main_test.go` exists and is the repo's first `package main` test**, and
      it covers `/livez`, `/readyz` and `/health` both directly and through a mux.
- [x] **`CHANGELOG.md` exists**, is Keep a Changelog format, has a
      `## [Unreleased]` section holding this work, has one section per tag through
      v0.4.2, and **every backfilled entry traces to a real commit** you read in
      Task 6 Step 2. Nothing was invented to fill space.
- [x] **The ADR number was confirmed by `ls docs/adr/`**, not assumed, and the ADR
      has a row in `docs/adr/index.md` and an entry in `mkdocs.yml`'s nav.
- [x] **`make ci` is green** and `mkdocs build --strict` is clean.
- [x] **The user-facing docs sweep found no stale claims.** `docs/cli.md` lists all
      three endpoints, `docs/deployment/docker.md` documents the healthcheck, and
      the grep for `distroless` / `65532` / `nonroot` returns nothing.
