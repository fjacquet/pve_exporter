# Docker

## Image

Multi-arch images are published to GHCR:

```
ghcr.io/fjacquet/pve_exporter:latest
```

Run a single container:

```bash
docker run --rm -p 9221:9221 \
  -v "$PWD/config.yaml:/etc/pve_exporter/config.yaml:ro" \
  -e PVE1_HOST=pve.example.com \
  -e PVE1_TOKEN_ID='exporter@pve!metrics' \
  -e PVE1_TOKEN_SECRET='xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx' \
  ghcr.io/fjacquet/pve_exporter:latest --config /etc/pve_exporter/config.yaml
```

### Logging

Add `--debug` for verbose logs, or `--trace` for token-safe HTTP
request/response tracing (the `PVEAPIToken` secret is never printed).

## Environment variables

The config file uses `${ENV_VAR}` placeholders, expanded at load time from the
process environment and an autoloaded `.env`:

| Variable            | Maps to                       | Example                                   |
| ------------------- | ----------------------------- | ----------------------------------------- |
| `PVE1_HOST`         | `clusters[0].host`            | `pve.example.com`                         |
| `PVE1_TOKEN_ID`     | `clusters[0].tokenID`         | `exporter@pve!metrics`                    |
| `PVE1_TOKEN_SECRET` | `clusters[0].tokenSecret`     | `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`     |

For additional clusters, follow the same `PVE2_*`, `PVE3_*` … pattern and add a
matching entry under `clusters:` in the config.

## Compose stack

The repository ships two Compose files:

| File                       | Purpose                                              |
| -------------------------- | ---------------------------------------------------- |
| `docker-compose.yml`       | Build the exporter locally + observability stack.    |
| `docker-compose.ghcr.yml`  | Pull the published GHCR image instead of building.   |

```bash
cp .env.example .env       # fill in PVE1_HOST / PVE1_TOKEN_ID / PVE1_TOKEN_SECRET
docker compose up -d                              # local build
docker compose -f docker-compose.ghcr.yml up -d   # GHCR image
```

The observability stack wires:

| Service       | Port   | Role                                              |
| ------------- | ------ | ------------------------------------------------- |
| `pve_exporter`| `9221` | Exposes `/metrics`.                               |
| `prometheus`  | `9090` | Scrapes the exporter.                             |
| `grafana`     | `3000` | Pre-provisioned PVE dashboard.                    |

### Grafana login

| Field    | Value     |
| -------- | --------- |
| URL      | `http://localhost:3000` |
| Username | `admin`   |
| Password | `admin`   |

!!! warning "Change the default credentials"
    The Compose Grafana ships with `admin`/`admin` for local use only. Change the
    password (or set `GF_SECURITY_ADMIN_PASSWORD`) before exposing it anywhere.
