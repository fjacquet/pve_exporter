# Quickstart

Get `pve_exporter` scraping your Proxmox VE cluster in a few minutes.

## 1. Create a PVE API Token

In the Proxmox web UI:

1. Go to **Datacenter → Permissions → API Tokens** and click **Add**.
2. Choose a user (e.g. `exporter@pam`), set a Token ID (e.g. `prometheus`), and uncheck *Privilege Separation* so the token inherits the user's roles.
3. Copy the generated secret — it is shown only once.
4. Go to **Datacenter → Permissions → Add → API Token Permission**, set Path `/`, select the token, and assign the built-in **PVEAuditor** role.

The token is passed in the HTTP header `Authorization: PVEAPIToken=USER@REALM!TOKENID=SECRET` — it is never written to logs.

## 2. Write `config.yaml` and `.env`

**`config.yaml`** (supports `${ENV_VAR}` expansion and auto-loads `.env`):

```yaml
clusters:
  - name: pve1
    host: "${PVE1_HOST}"          # e.g. 10.0.0.1:8006
    tokenID: "${PVE1_TOKEN_ID}"   # e.g. exporter@pam!prometheus
    tokenSecret: "${PVE1_TOKEN_SECRET}"
    insecureSkipVerify: false     # set true for self-signed PVE certs
```

**`.env`** (same directory, loaded automatically):

```dotenv
PVE1_HOST=10.0.0.1:8006
PVE1_TOKEN_ID=exporter@pam!prometheus
PVE1_TOKEN_SECRET=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```

## 3. Run the Exporter

=== "Binary"

    Download the latest release binary (v0.1.1) from the
    [GitHub Releases page](https://github.com/fjacquet/pve_exporter/releases), or install
    via Homebrew:

    ```bash
    brew install --cask fjacquet/tap/pve_exporter
    ```

    Then start it:

    ```bash
    pve_exporter --config config.yaml
    ```

=== "Docker Compose (local build)"

    ```bash
    # Builds the exporter image locally, also starts Prometheus + Grafana
    docker compose up -d
    ```

=== "Docker Compose (pre-built image)"

    ```bash
    # Pulls ghcr.io/fjacquet/pve_exporter:latest — no build required
    docker compose -f docker-compose.ghcr.yml up -d
    ```

## 4. Verify

```bash
curl localhost:9221/metrics | grep pve_up
```

You should see one `pve_up` line per configured cluster (value `1` = healthy).

Open Grafana at **http://localhost:3000** (default credentials `admin` / `admin`) to
explore the pre-bundled dashboards.

---

**Next steps:**

- [Deployment / Docker](deployment/docker.md) — production-grade Compose setup and image tags.
- [Metrics Reference](metrics.md) — full list of exported metrics and labels.
- [CLI & Validation](cli.md) — flags reference and the trace-run recipe for debugging.
