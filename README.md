# SolarOps

A solar plant real-time monitoring platform, built as a learning/practice MVP using [Claude Code](https://claude.ai/claude-code).

## What it does

- Simulates multiple solar plants, each with configurable panels generating power data every second
- Aggregates and visualizes real-time power output per plant and across the fleet
- Detects panel anomalies (dead, degraded, intermittent) and manages alert lifecycle
- Supports panel control (on/off, reset, fault injection) via WebSocket commands
- Auto-discovers new plants via NATS heartbeat — just add to docker-compose and `up -d`

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend services | Go |
| Frontend | React + TypeScript (Vite) |
| Messaging | NATS |
| Data store | Elasticsearch |
| Log ingestion | Fluentd (sidecar per plant) |
| Containerization | Docker Compose |
| Reverse proxy | Nginx |

## Architecture

```
mock-plant ──→ NATS ──→ alert-service (real-time detection)
     │                    ws-gateway (WebSocket bridge)
     │                    plant-manager (registry + API)
     └──→ Fluentd ──→ Elasticsearch ←── aggregator (10s summary)
                              ↑
                         frontend (React, polls REST + WebSocket)
```

Full architecture documentation with Mermaid diagrams, data flows, API reference, and alert rules is in [`docs/architecture.md`](docs/architecture.md).

## Quick Start

```bash
docker compose up --build
```

Then open http://localhost:3000.

## Services

| Service | Port | Description |
|---------|------|-------------|
| `frontend` | 3000 | Dashboard UI (Nginx reverse proxy) |
| `ws-gateway` | 8080 | WebSocket gateway, bridges NATS and browser |
| `alert-service` | 8081 | Real-time anomaly detection and alert management |
| `plant-manager` | 8082 | ES query gateway, NATS auto-discovery, panel commands |
| `aggregator` | — | Aggregates panel data from ES every 10s |
| `mock-plant` | — | Simulates solar plants (3 pre-configured) |
| `elasticsearch` | 9200 | Data store |
| `nats` | 4222 | Message bus |
| `kibana` | 5601 | ES visualization (dev) |

## License

MIT
