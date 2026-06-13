/*
****** WHAT THIS FILE DOES ******
* Entry point of the Leaderboard Service.
* Serves the live leaderboard frontend and streams real-time
* score updates to all connected browsers via WebSocket.
*
* RESPONSIBILITIES:
* - Serves static frontend at http://localhost:3000
* - Exposes /ws WebSocket endpoint for live score streaming
* - Exposes /metrics endpoint for Prometheus scraping
* - Consumes scored metrics from Kafka "scored-metrics" topic
* - Still uses Redis sorted set for leaderboard queries
*
* FLOW:
* 1. Browser opens http://localhost:3000
* 2. JavaScript connects to ws://localhost:3000/ws
* 3. WebSocket handler sends current leaderboard from Redis immediately
* 4. Kafka consumer receives new score from telemetry-ingester
* 5. Score pushed to all connected browsers via WebSocket
* 6. Browser updates rankings, charts and stats in real time
*
* METRICS EXPOSED (port 2113):
* - leaderboard_websocket_connections_active  => live browser connections
* - leaderboard_updates_total                 => total score updates pushed
*
* WHY KAFKA OVER REDIS PUB/SUB?
* Redis pub/sub drops messages if leaderboard service is temporarily down.
* Kafka retains messages — no score updates lost even during restarts.
*/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	kafkapkg "leaderboard-service/kafka"
	"leaderboard-service/handlers"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// ─── Prometheus Metrics ───────────────────────────────────────

var (
	activeConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "leaderboard_websocket_connections_active",
			Help: "Number of active WebSocket connections",
		},
	)

	updatesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "leaderboard_updates_total",
			Help: "Total leaderboard score updates pushed to browsers",
		},
	)
)

func init() {
	prometheus.MustRegister(activeConnections)
	prometheus.MustRegister(updatesTotal)
}

// ScoredMetrics received from telemetry ingester via Kafka
type ScoredMetrics struct {
	Contestant   string  `json:"contestant"`
	SubmissionID string  `json:"submission_id"`
	P50          int64   `json:"p50"`
	P90          int64   `json:"p90"`
	P99          int64   `json:"p99"`
	TPS          float64 `json:"tps"`
	SuccessRate  float64 `json:"success_rate"`
	Score        float64 `json:"score"`
}

func main() {
	fmt.Println("IICPC Platform - Leaderboard Service starting...")

	ctx := context.Background()

	// connect to Redis — still needed for leaderboard sorted set queries
	rdb := redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Printf("Redis connection error: %v\n", err)
		return
	}
	fmt.Println("Connected to Redis successfully!")

	// initialize Kafka consumer for scored-metrics topic
	kafkaConsumer := kafkapkg.NewConsumer("scored-metrics", "leaderboard-group")
	defer kafkaConsumer.Close()

	// start Kafka consumer — forwards score updates to WebSocket clients
	go func() {
		fmt.Println("Listening for scored metrics on Kafka...")
		for {
			data, err := kafkaConsumer.ReadMessage(ctx)
			if err != nil {
				fmt.Printf("Kafka read error: %v\n", err)
				continue
			}

			var scored ScoredMetrics
			if err := json.Unmarshal(data, &scored); err != nil {
				fmt.Printf("Kafka parse error: %v\n", err)
				continue
			}

			fmt.Printf("📊 Score received from Kafka: %s → %.1f\n", scored.Contestant, scored.Score)

			// update Redis sorted set for leaderboard queries
			rdb.ZAdd(ctx, "leaderboard", redis.Z{
				Score:  scored.Score,
				Member: scored.Contestant,
			})

			// broadcast to all connected WebSocket clients
			handlers.BroadcastScore(data)
			updatesTotal.Inc()
		}
	}()

	// start prometheus metrics server on port 2113
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		fmt.Println("Metrics server running on :2113")
		http.ListenAndServe(":2113", mux)
	}()

	// serve static frontend
	http.Handle("/", http.FileServer(http.Dir("./static")))

	// WebSocket endpoint
	http.HandleFunc("/ws", handlers.WebSocketHandler(rdb))

	fmt.Println("Leaderboard service running on http://localhost:3000")
	if err := http.ListenAndServe(":3000", nil); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}