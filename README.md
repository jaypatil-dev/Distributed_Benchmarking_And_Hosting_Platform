# IICPC Summer Hackathon 2026
## Distributed Benchmarking & Hosting Platform

A production-grade distributed platform that evaluates contestant-submitted trading engines under real-world stress conditions. The platform containerizes submitted code, spawns a distributed fleet of trading bots, captures granular telemetry, and streams results to a live leaderboard in real time.

Built by **Yash & Jay** for IICPC Summer Hackathon 2026.

---

## Table of Contents
- [What It Does](#what-it-does)
- [Architecture](#architecture)
- [Tech Stack](#tech-stack)
- [Services](#services)
- [Getting Started](#getting-started)
- [How It Works](#how-it-works)
- [Scoring Formula](#scoring-formula)
- [Monitoring](#monitoring)
- [Project Structure](#project-structure)
- [Known Limitations](#known-limitations)
- [Future Improvements](#future-improvements)

---

## What It Does

Contestants upload their trading engine code (a simulated order book or matching engine). The platform then:

1. **Containerizes** the submission in an isolated Docker environment with strict resource limits
2. **Spawns 1000 concurrent bots** that send real HTTP requests to the contestant's engine
3. **Measures real performance** — actual round-trip latency, TPS, and correctness
4. **Scores the submission** using a 4-factor composite formula
5. **Updates the live leaderboard** in under 1 second via WebSocket

---

## Architecture
Contestant Code Upload

↓

API Gateway (port 8080)

↓ Kafka: submission-events

Telemetry Ingester

↓                    ↓

Kafka: bot-commands    Kafka: scored-metrics

↓                    ↓

Bot Fleet           Leaderboard Service (port 3000)

↓                    ↓

Real HTTP attacks      WebSocket → Browser

↓

Test Engine (port 9000)

All services communicate via **Kafka topics** — no direct service-to-service calls. Redis is used for submission storage, auth sessions and leaderboard sorted sets.

---

## Tech Stack

| Technology | Purpose |
|------------|---------|
| Go | All backend services — native concurrency via goroutines |
| Gin | HTTP framework for API Gateway — 40x faster than net/http |
| Kafka | Inter-service message bus — guaranteed delivery |
| Redis | Submission storage, auth sessions, leaderboard sorted sets |
| TimescaleDB | Time-series storage for order metrics and scores |
| Docker | Sandboxed execution of contestant code |
| Docker Compose | Single-command platform orchestration |
| WebSocket | Real-time leaderboard updates to browsers |
| Prometheus | Metrics scraping from all 5 services |
| Grafana | Real-time monitoring dashboard |

---

## Services

| Service | Port | Description |
|---------|------|-------------|
| api-gateway | 8080 | Submission portal, auth, admin dashboard |
| leaderboard-service | 3000 | Live leaderboard frontend + WebSocket |
| telemetry-ingester | — | Scores submissions, publishes to Kafka |
| bot-fleet | — | 1000 real HTTP bots per submission |
| submission-engine | — | Docker SDK — containerizes contestant code |
| test-engine | 9000 | Sample trading engine for testing |
| prometheus | 9090 | Metrics collection |
| grafana | 3001 | Monitoring dashboards |
| redis | 6379 | Cache, queue, leaderboard |
| timescaledb | 5432 | Time-series metrics storage |
| kafka | 9092 | Event streaming between services |

---

## Getting Started

### Prerequisites
- Docker Desktop installed and running
- Docker Compose v2+
- Git

### Installation

**1. Clone the repository:**
```bash
git clone https://github.com/your-repo/iicpc-platform.git
cd iicpc-platform
```

**2. Start the platform:**
```bash
docker compose up --build
```

**3. Wait for all services to start (~30 seconds), then open:**

| URL | Description |
|-----|-------------|
| http://localhost:8080 | Submission Portal |
| http://localhost:3000 | Live Leaderboard |
| http://localhost:8080/admin | Admin Dashboard |
| http://localhost:9090 | Prometheus |
| http://localhost:3001 | Grafana (admin / your-password) |

**4. Stop the platform:**
```bash
docker compose down
```

### First Time Setup — Grafana

1. Open http://localhost:3001
2. Login with your configured credentials
3. Go to **Connections → Data Sources → Add → Prometheus**
4. URL: `http://prometheus:9090`
5. Click **Save & Test**
6. Go to **Dashboards → Import**
7. Upload `grafana/dashboard.json`
8. Done — full monitoring dashboard loaded!

---

## How It Works

### Submission Flow

**Step 1 — Register & Login**
Contestants register at http://localhost:8080. Two roles available:
- `CONTESTANT` — no key required
- `ADMIN` — requires secret admin key

**Step 2 — Submit Code**
Contestants upload their trading engine code. Supported languages:
- C++, Rust, Go, Python, Java, JavaScript

**Step 3 — Kafka Pipeline**
API Gateway → Kafka (submission-events) → Telemetry Ingester

↓

Kafka (bot-commands) → Bot Fleet

↓

Kafka (scored-metrics) → Leaderboard

**Step 4 — Bot Fleet Attack**
1000 goroutines simultaneously send real HTTP POST requests to the contestant's engine:
```json
{
  "id": "order-123",
  "type": "buy",
  "price": 107.14,
  "quantity": 42
}
```
Real round-trip latency is measured per request.

**Step 5 — Correctness Validation**
5 automated tests verify the trading engine is algorithmically correct:

| Test | Description | Expected |
|------|-------------|----------|
| Basic Fill | buy@105 with sell@100 queued | filled |
| No Fill | buy@95 with sell@110 queued | queued |
| Exact Price Match | buy@100 with sell@100 | filled |
| Multiple Sellers | buy@102 with 3x sell@99 | filled |
| Price Priority | buy@103 with sell@101 and sell@108 | filled with cheapest |

**Step 6 — Live Leaderboard**
Scores are published to Kafka `scored-metrics` topic → Leaderboard Service consumes → WebSocket pushes to all connected browsers. Rankings update in under 1 second.

---

## Scoring Formula
Score = (1000 / p99)  × 40   →  rewards low latency       (40%)

+ (TPS / 100)   × 40   →  rewards high throughput   (40%)

+ successRate   × 20   →  rewards reliability        (20%)

+ correctness   × 20   →  rewards algorithmic accuracy (bonus)

**Why percentiles over average?**
Average latency is misleading — one 900ms outlier hides 999 great responses. p99 tells the truth about worst-case performance, which matters most in trading systems.

---

## Monitoring

### Prometheus Targets
All 5 services expose `/metrics` endpoints:

| Service | Metrics Port | Key Metrics |
|---------|-------------|-------------|
| api-gateway | 8080 | http_requests_total, request_duration, submissions_total |
| telemetry-ingester | 2112 | submissions_processed, active_submissions, scoring_duration |
| leaderboard-service | 2113 | websocket_connections_active, updates_total |
| bot-fleet | 2114 | attacks_total, active_attacks, p99_latency, tps, success_rate |
| submission-engine | 2115 | containers_created, containers_active, container_errors |

### Grafana Dashboard
Pre-built dashboard at `grafana/dashboard.json` with 6 panels:
- HTTP Requests Total
- API Gateway Latency
- Total Submissions
- Active Bot Attacks
- Bot Fleet P99 Latency
- Live Leaderboard Viewers

---

## Project Structure
iicpc-platform/

├── api-gateway/           # REST API, auth, submission handling

│   ├── handlers/          # Route handlers

│   ├── kafka/             # Kafka producer

│   ├── models/            # Data structures

│   └── static/            # Frontend HTML/CSS/JS

├── bot-fleet/             # 1000 real HTTP bots

│   ├── attacker/          # HTTP bot implementation

│   ├── kafka/             # Kafka consumer

│   └── validator/         # Correctness validation

├── leaderboard-service/   # WebSocket + live leaderboard

│   ├── handlers/          # WebSocket handler + broadcast

│   └── kafka/             # Kafka consumer

├── telemetry-ingester/    # Scoring engine

│   ├── calculator/        # p50/p90/p99/TPS math

│   ├── kafka/             # Producer + consumer

│   └── storage/           # TimescaleDB + Redis store

├── submission-engine/     # Docker SDK sandboxing

│   └── sandbox/           # Container lifecycle

├── test-engine/           # Sample trading engine (port 9000)

├── grafana/

│   └── dashboard.json     # Pre-built monitoring dashboard

├── prometheus/

│   └── prometheus.yml     # Scrape config

├── docker-compose.yml     # Full platform orchestration

├── architecture.md        # Detailed system design document

└── SUMMARY.md             # 1-page judge summary

---

## Known Limitations

| Limitation | Reason | Fix |
|------------|--------|-----|
| TimescaleDB mock on Windows | SASL auth issue with Docker Desktop | Deploy on Linux — works with zero code changes |
| Single Kafka broker | Hackathon simplicity | Add more brokers and partitions for production |

---

## Future Improvements

### High Priority
- **gVisor runtime** — kernel-level isolation for contestant containers, stronger than standard Docker namespaces
- **Network=none** — completely disable network inside contestant containers
- **Rate limiting** — prevent submission spam via API Gateway
- **JWT refresh tokens** — more secure auth with token expiry

### Medium Priority
- **Kafka topic partitioning** — 3 partitions on `submission-events` → 3 telemetry ingester instances process submissions in parallel → 3x throughput
- **TimescaleDB continuous aggregates** — pre-computed metrics for instant historical queries
- **Horizontal scaling** — run multiple bot-fleet instances for 10,000+ concurrent bots
- **Read replicas** — TimescaleDB read replicas for leaderboard queries under high load

### Low Priority
- **Cloud deployment** — Oracle Cloud Free Tier, AWS EC2, or DigitalOcean droplet
- **Kubernetes manifests** — convert Docker Compose to K8s for production orchestration
- **gRPC** — replace REST between internal services for lower latency
- **Seccomp profiles** — whitelist only safe system calls inside contestant containers
- **Multi-region deployment** — run bot fleet from multiple geographic regions for realistic global latency testing
- **Problem sets** — multiple trading problems (order book, market maker, arbitrage) each with different scoring weights

---

## Security

- Contestant code runs in isolated Docker containers — cannot access host or other containers
- 256MB memory limit — prevents memory exhaustion attacks
- 0.5 CPU limit — prevents CPU monopolization
- Bridge network — isolated from host network
- Admin routes protected by secret key
- Submission data expires after 24 hours

---

## License

Built for IICPC Summer Hackathon 2026. Open source — MIT License.
