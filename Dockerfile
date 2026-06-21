# Stage 1: Build
FROM golang:1.26.4 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static build; CGO_ENABLED=0 produces a fully static binary.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o pve_exporter .

# Stage 2: Runtime
FROM alpine:latest

# Create the runtime user and log dir. These are busybox builtins (no network).
RUN adduser -D -u 10001 pve && \
    mkdir -p /var/log/pve_exporter && \
    chown pve:pve /var/log/pve_exporter

# Copy the CA bundle from the builder stage instead of `apk add ca-certificates`.
# The latter fetches from the Alpine CDN over TLS, which fails behind a corporate
# MITM proxy: the bare alpine image has no CA bundle yet to validate the proxy
# cert (chicken-and-egg). The Debian-based golang builder already ships the bundle.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

COPY --from=builder /app/pve_exporter /usr/bin/pve_exporter
COPY config.yaml /etc/pve_exporter/config.yaml

EXPOSE 9221

USER pve

ENTRYPOINT ["/usr/bin/pve_exporter"]
CMD ["--config", "/etc/pve_exporter/config.yaml"]
