# servprobe

Self-hosted service health monitor. Check HTTP endpoints, TCP ports, ping hosts, and Docker containers — with a dashboard, REST API, alerts, and history.

Single binary. No external dependencies. SQLite storage.

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

---

## Features

- **4 check types** — HTTP (status code + response time), TCP (port connectivity), Ping (ICMP), Docker (container status)
- **Web dashboard** — Dark theme, auto-refresh, uptime %, response time charts
- **REST API** — Service listing, detail, paginated history, health endpoint
- **Webhook alerts** — POST JSON on state change (up→down / down→up) with configurable cooldown
- **SQLite storage** — Check history with WAL mode for performance
- **Per-service scheduling** — Independent check intervals per service
- **CLI tools** — `serve`, `check` (one-off), `status` (table view), `version`
- **Single binary** — Embed dashboard assets, no runtime dependencies

## Quick Start

### Install from source

```bash
git clone https://github.com/hazz-dev/servprobe.git
cd servprobe
make build
```

### Configure

```bash
cp config.example.yml config.yml
# Edit config.yml with your services
```

### Run

```bash
# Start the monitor + dashboard + API
./servprobe serve --config config.yml

# One-off check (no server)
./servprobe check --config config.yml

# Print status table from database
./servprobe status --config config.yml
```

Dashboard: http://localhost:8080

## Configuration

```yaml
services:
  # HTTP — GET request, check status code
  - name: "api"
    type: "http"
    target: "https://api.example.com/health"
    interval: "30s"
    timeout: "5s"
    expected_status: 200
    headers:
      Authorization: "Bearer token"

  # TCP — dial host:port
  - name: "database"
    type: "tcp"
    target: "db.example.com:5432"
    interval: "15s"
    timeout: "3s"

  # Ping — ICMP echo
  - name: "gateway"
    type: "ping"
    target: "10.0.0.1"
    interval: "60s"
    timeout: "5s"

  # Docker — container running status
  - name: "redis"
    type: "docker"
    target: "redis-container"
    interval: "30s"

alerts:
  webhook:
    url: "https://hooks.example.com/alert"
    cooldown: "5m"

server:
  address: ":8080"

storage:
  path: "servprobe.db"
```

### Service types

| Type | Target format | What it checks |
|------|--------------|----------------|
| `http` | URL (`https://...`) | GET request, status code, response time |
| `tcp` | `host:port` | TCP connection, latency |
| `ping` | hostname or IP | ICMP echo, round-trip time |
| `docker` | container name/ID | Running status via Docker socket |

### Defaults

| Setting | Default |
|---------|---------|
| `interval` | `30s` |
| `timeout` | `5s` |
| `expected_status` | `200` (HTTP only) |
| `server.address` | `:8080` |
| `storage.path` | `servprobe.db` |
| `alerts.webhook.cooldown` | `5m` |

## REST API

| Endpoint | Description |
|----------|-------------|
| `GET /api/health` | Service health (uptime, version) |
| `GET /api/services` | All services with current status |
| `GET /api/services/{name}` | Single service + recent history |
| `GET /api/services/{name}/history?limit=50&offset=0` | Paginated check history |

### Example response

```json
GET /api/services

[
  {
    "name": "api",
    "type": "http",
    "target": "https://api.example.com/health",
    "status": "up",
    "response_time_ms": 142,
    "last_check": "2026-02-19T10:30:00Z",
    "uptime_percent": 99.8
  }
]
```

## Dashboard

The built-in dashboard shows:

- Service cards with status (green/red), uptime %, average response time
- Click any service for detailed view with response time history chart
- Auto-refreshes every 30 seconds
- Dark theme

## Alerts

When a service changes state (up→down or down→up), servprobe sends a webhook:

```json
POST https://hooks.example.com/alert

{
  "service": "api",
  "status": "down",
  "previous_status": "up",
  "error": "unexpected status 503",
  "response_time_ms": 2043,
  "timestamp": "2026-02-19T10:30:00Z"
}
```

Cooldown prevents alert spam — same service won't trigger again within the cooldown period.

## Building

```bash
# Build
make build

# Build with version info
make build VERSION=1.0.0

# Run tests
make test

# Format + vet
make fmt
make vet

# All checks
make all
```

## Architecture

```
cmd/servprobe/          CLI (cobra)
internal/
├── checker/            HTTP, TCP, Ping, Docker checkers
├── config/             YAML config loading + validation
├── scheduler/          Per-service goroutine scheduler
├── storage/            SQLite persistence (WAL mode)
├── server/             Chi REST API
├── alert/              Webhook notifications
├── dashboard/          Embedded HTML/CSS/JS (go:embed)
└── version/            Build info (ldflags)
```

## Tech Stack

| Component | Library |
|-----------|---------|
| HTTP router | [chi](https://github.com/go-chi/chi) |
| CLI | [cobra](https://github.com/spf13/cobra) |
| Config | [yaml.v3](https://gopkg.in/yaml.v3) |
| Database | [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go, no CGO) |
| Ping | [go-ping](https://github.com/go-ping/ping) |
| Docker | [docker/docker](https://github.com/moby/moby) client |

## License

MIT
