# IICPC Summer Hackathon 2026
## Distributed Benchmarking & Hosting Platform

---

## What We Built
A production-grade distributed platform that evaluates contestant-submitted
trading engines under real-world stress conditions — complete pipeline from
code upload to live leaderboard in under 10 seconds.

---

## Live Demo
| Service            | URL                        |
|--------------------|----------------------------|
| Submission Portal  | http://localhost:8080       |
| Live Leaderboard   | http://localhost:3000       |
| Admin Dashboard    | http://localhost:8080/admin |
| Grafana Monitoring | http://localhost:3001       |
| Prometheus Metrics | http://localhost:9090       |

One command to run everything:
```bash
docker compose up --build
```

---

## Architecture at a Glance

| Step | Component | Action |
|------|-----------|--------|
| 1 | **Contestant** | Uploads trading engine code |
| 2 | **API Gateway** | Validates and publishes to Kafka `submission-events` |
| 3 | **Telemetry Ingester** | Consumes from Kafka, triggers bot fleet via `bot-commands` |
| 4 | **Bot Fleet** | 1000 real HTTP bots attack the containerized engine |
| 5 | **Scoring Engine** | Calculates p50/p90/p99, TPS, correctness |
| 6 | **TimescaleDB** | Stores all metrics permanently via hypertables |
| 7 | **Kafka `scored-metrics`** | Broadcasts score to leaderboard service |
| 8 | **Leaderboard Service** | Consumes from Kafka, pushes via WebSocket |
| 9 | **Browser** | Rankings update in under 1 second |

Admin Dashboard monitors all steps in real time via Prometheus + Grafana.

---

## Key Engineering Achievements

**Full Kafka Event Pipeline**
All inter-service communication flows through Kafka topics —
no direct service calls, no message loss, full replay capability.
Three topics: `submission-events` → `bot-commands` → `scored-metrics`.

**Real Distributed Bot Fleet**
1000 concurrent goroutines send real HTTP requests to contestant
engines — measuring actual round-trip latency, not simulated numbers.
Bot fleet runs as a containerized service on the same Docker network.

**4-Factor Composite Scoring**
Score = (1000/p99)  × 40  → latency

+ (TPS/100)   × 40  → throughput

+ successRate × 20  → reliability

+ correctness × 20  → algorithmic accuracy

**Correctness Validation Engine**
5 automated tests verify price-time priority and fill accuracy —
a fast but incorrect trading engine cannot win.

**Real-Time Leaderboard**
Sub-second updates via WebSocket — Kafka consumer pushes score
updates to all connected browsers the moment scoring completes.

**Secure Sandboxing**
Every submission runs in an isolated Docker container with 256MB
memory and 0.5 CPU hard limits — malicious code cannot escape.

**Full Observability**
Prometheus scrapes 5 services every 15 seconds. Grafana dashboard
shows HTTP requests, latency, active attacks, P99 and live viewers
in real time. Dashboard exported as code for one-click import.

**Multiple Submissions**
Contestants can submit up to 100 times. Full history maintained
per contestant — latest score counts for leaderboard.

**Admin Dashboard**
Organizers can view all users, all submissions, delete accounts
and re-test any submission with one click.

---

## Tech Stack
Go · Kafka · Redis · TimescaleDB · Docker · WebSocket · Prometheus · Grafana · Gin · Gorilla WebSocket

---

## Platform Metrics (Live)
- Bots per submission: **1,000 concurrent**
- Measured TPS: **~1,800 orders/sec**
- p99 Latency measured: **~99ms**
- Leaderboard update delay: **< 1 second**
- Kafka topics: **3 (submission-events, bot-commands, scored-metrics)**
- Services monitored: **5 (api-gateway, telemetry, bot-fleet, leaderboard, submission-engine)**
- Platform startup: **1 command**

---

## What Makes Us Different
Most hackathon platforms simulate load. Ours sends **real HTTP requests**
to real containerized engines and measures **real performance** — the same
way production trading infrastructure is evaluated.

Redis pub/sub was the industry default. We went further — **full Kafka
integration** across all services for guaranteed delivery, message
persistence and horizontal scaling. Every architectural decision was
made for production, not just demo.
