# Canonical Go Makefile — fjacquet/ci standard interface (do not rename targets)
.DEFAULT_GOAL := all
BIN     = pve_exporter
DIST    ?= dist
COVER   ?= coverage.out
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -s -w -X main.version=$(VERSION)

# Pinned tool versions (installed by `make tools`).
GOLANGCI_VERSION   ?= v2.12.2
GORELEASER_VERSION ?= v2.16.0

.PHONY: all clean install tools lint format test build vuln sbom security docs \
        coverage-upload release ci \
        fmt-check fmt vet test-race test-coverage sure \
        cli release-snapshot docker run-cli clean-dist

all: clean lint test build

clean:
	rm -rf $(DIST) site $(COVER) *.sarif
	rm -f bin/$(BIN) coverage.html

install:
	go mod download

# Install pinned dev/CI tooling into $GOPATH/bin.
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

lint:
	golangci-lint run --timeout=5m

format:
	golangci-lint fmt

test:
	go test -race -coverprofile=$(COVER) -covermode=atomic ./...

build:
	go build -v ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

sbom:
	mkdir -p $(DIST)
	go run github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest mod -json -output $(DIST)/sbom.cdx.json

security:  # advisory: reports findings but never blocks the build (CodeQL/osv are the blocking gates)
	uvx semgrep scan --config auto --skip-unknown-extensions || true

docs:
	uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict --site-dir site

coverage-upload:
	uvx --from codecov-cli codecov upload-process --file $(COVER) || true

release:
	goreleaser release --clean

# Aggregate gate run by CI.
ci: lint test build vuln

# --- repo-specific convenience targets ---

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed in:"; gofmt -l .; exit 1)

fmt:
	go fmt ./...

vet:
	go vet ./...

test-race: test

test-coverage: test
	go tool cover -html=$(COVER) -o coverage.html

# Local convenience: format, vet, test, build, lint.
sure: fmt vet test
	go build ./...
	golangci-lint run

# Build single binary for local use.
cli:
	go build -ldflags="$(LDFLAGS)" -o bin/$(BIN) .

# Local dry-run: full pipeline (build, archive, SBOM, checksums) without publishing.
release-snapshot:
	goreleaser release --snapshot --clean
	@echo "release artifacts in $(DIST)/"

docker:
	docker build -t $(BIN):$(VERSION) -t $(BIN):latest .

run-cli: cli
	./bin/$(BIN) --config config.yaml

clean-dist:
	rm -rf $(DIST)
