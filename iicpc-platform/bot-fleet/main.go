/*
****** WHAT THIS FILE DOES ******
* Entry point of the Bot Fleet Service.
* Consumes attack commands from Kafka and stress tests contestant
* engines with real HTTP requests.
*
* FLOW:
* 1. Connects to Redis and Kafka
* 2. Kafka consumer listens on "bot-commands" topic
* 3. When a command arrives:
*    => Runs correctness validation first
*    => Launches 1000 real HTTP bots against contestant endpoint
*    => Calculates p50/p90/p99/TPS from real response times
*    => Updates leaderboard in Redis with real scores + correctness
*
* METRICS EXPOSED (port 2114):
* - botfleet_attacks_total        => total attack runs completed
* - botfleet_active_attacks       => currently running attacks
* - botfleet_p99_latency_ms      => p99 latency of last attack
* - botfleet_tps                  => TPS of last attack
* - botfleet_success_rate         => success rate of last attack
* - botfleet_correctness_score    => correctness score of last attack
*
* WHY KAFKA OVER REDIS LIST?
* Commands persist if bot fleet crashes — no attack commands lost.
* Multiple bot fleet instances can consume in parallel for scale.
*/

package main

import (
	"bot-fleet/attacker"
	kafkapkg "bot-fleet/kafka"
	"bot-fleet/validator"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// BotCommand received from telemetry ingester via Kafka
type BotCommand struct {
	SubmissionID string `json:"submission_id"`
	Contestant   string `json:"contestant"`
	TargetURL    string `json:"target_url"`
	NumBots      int    `json:"num_bots"`
}

// Metrics holds calculated performance data from real bot results
type Metrics struct {
	P50         int64   `json:"p50"`
	P90         int64   `json:"p90"`
	P99         int64   `json:"p99"`
	TPS         float64 `json:"tps"`
	SuccessRate float64 `json:"success_rate"`
	TotalOrders int     `json:"total_orders"`
}

// LeaderboardEntry is the full scored entry pushed to Redis
type LeaderboardEntry struct {
	Contestant   string  `json:"contestant"`
	SubmissionID string  `json:"submission_id"`
	P50          int64   `json:"p50"`
	P90          int64   `json:"p90"`
	P99          int64   `json:"p99"`
	TPS          float64 `json:"tps"`
	SuccessRate  float64 `json:"success_rate"`
	Score        float64 `json:"score"`
}

// ─── Prometheus Metrics ───────────────────────────────────────

var (
	attacksTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "botfleet_attacks_total",
			Help: "Total number of bot fleet attacks completed",
		},
	)

	activeAttacks = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "botfleet_active_attacks",
			Help: "Number of bot fleet attacks currently running",
		},
	)

	lastP99Latency = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "botfleet_p99_latency_ms",
			Help: "P99 latency in milliseconds of last attack",
		},
	)

	lastTPS = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "botfleet_tps",
			Help: "TPS measured in last attack",
		},
	)

	lastSuccessRate = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "botfleet_success_rate",
			Help: "Success rate percentage of last attack",
		},
	)

	lastCorrectnessScore = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "botfleet_correctness_score",
			Help: "Correctness score out of 20 from last attack",
		},
	)
)

func init() {
	prometheus.MustRegister(attacksTotal)
	prometheus.MustRegister(activeAttacks)
	prometheus.MustRegister(lastP99Latency)
	prometheus.MustRegister(lastTPS)
	prometheus.MustRegister(lastSuccessRate)
	prometheus.MustRegister(lastCorrectnessScore)
}

func calculateMetrics(results []attacker.OrderResult) Metrics {
	if len(results) == 0 {
		return Metrics{}
	}

	latencies := make([]int64, len(results))
	successCount := 0

	for i, r := range results {
		latencies[i] = r.Latency
		if r.Success {
			successCount++
		}
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	// calculate time window for TPS
	earliest := results[0].Timestamp
	latest := results[0].Timestamp
	for _, r := range results {
		if r.Timestamp.Before(earliest) {
			earliest = r.Timestamp
		}
		if r.Timestamp.After(latest) {
			latest = r.Timestamp
		}
	}

	duration := latest.Sub(earliest).Seconds()
	tps := 0.0
	if duration > 0 {
		tps = float64(len(results)) / duration
	}

	return Metrics{
		P50:         percentile(latencies, 50),
		P90:         percentile(latencies, 90),
		P99:         percentile(latencies, 99),
		TPS:         tps,
		SuccessRate: float64(successCount) / float64(len(results)) * 100,
		TotalOrders: len(results),
	}
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(p) / 100.0 * float64(len(sorted)))
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func updateLeaderboard(rdb *redis.Client, ctx context.Context, cmd BotCommand, m Metrics, report validator.CorrectnessReport) {
	// composite score: latency 40% + TPS 40% + reliability 20% + correctness 20
	score := (1000.0/float64(m.P99+1))*40 +
		(float64(m.TPS)/100.0)*40 +
		(m.SuccessRate/100.0)*20 +
		report.Score

	entry := LeaderboardEntry{
		Contestant:   cmd.Contestant,
		SubmissionID: cmd.SubmissionID,
		P50:          m.P50,
		P90:          m.P90,
		P99:          m.P99,
		TPS:          m.TPS,
		SuccessRate:  m.SuccessRate,
		Score:        math.Round(score*10) / 10,
	}

	data, _ := json.Marshal(entry)
	rdb.Set(ctx, "contestant:"+cmd.Contestant, data, 0)
	rdb.ZAdd(ctx, "leaderboard", redis.Z{Score: score, Member: cmd.Contestant})
	rdb.Publish(ctx, "leaderboard-updates", data)

	fmt.Printf("🏆 Leaderboard updated for %s → Score: %.1f (Correctness: %.1f/20)\n",
		cmd.Contestant, score, report.Score)
}

func processCommand(rdb *redis.Client, ctx context.Context, cmd BotCommand) {
	fmt.Printf("\n🎯 Starting attack on: %s\n", cmd.TargetURL)
	fmt.Printf("   Contestant: %s | Bots: %d\n", cmd.Contestant, cmd.NumBots)

	activeAttacks.Inc()
	defer activeAttacks.Dec()

	// run correctness validation before load test
	fmt.Println("\n🔍 Running correctness validation...")
	report := validator.ValidateEngine(cmd.TargetURL)

	// run real bot fleet
	results := attacker.RunFleet(cmd.TargetURL, cmd.NumBots)
	metrics := calculateMetrics(results)

	// update prometheus with real measured values
	lastP99Latency.Set(float64(metrics.P99))
	lastTPS.Set(metrics.TPS)
	lastSuccessRate.Set(metrics.SuccessRate)
	lastCorrectnessScore.Set(report.Score)
	attacksTotal.Inc()

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("    REAL BOT FLEET RESULTS")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Total Orders  : %d\n", metrics.TotalOrders)
	fmt.Printf("Success Rate  : %.1f%%\n", metrics.SuccessRate)
	fmt.Printf("TPS           : %.0f orders/sec\n", metrics.TPS)
	fmt.Println("───────────────────────────────")
	fmt.Printf("p50 Latency   : %dms\n", metrics.P50)
	fmt.Printf("p90 Latency   : %dms\n", metrics.P90)
	fmt.Printf("p99 Latency   : %dms\n", metrics.P99)
	fmt.Printf("Correctness   : %.1f/20 points\n", report.Score)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	updateLeaderboard(rdb, ctx, cmd, metrics, report)
}

func main() {
	fmt.Println("IICPC Platform - Bot Fleet Service starting...")

	ctx := context.Background()

	// connect to Redis — still needed for leaderboard sorted set
	rdb := redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Printf("Redis connection error: %v\n", err)
		return
	}
	fmt.Println("Connected to Redis successfully!")

	// initialize Kafka consumer for bot-commands topic
	kafkaConsumer := kafkapkg.NewConsumer("bot-commands", "botfleet-group")
	defer kafkaConsumer.Close()

	// start prometheus metrics server on port 2114
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		fmt.Println("Metrics server running on :2114")
		http.ListenAndServe(":2114", mux)
	}()

	fmt.Println("Bot Fleet ready — listening on Kafka bot-commands...")

	// consume from Kafka — replaces Redis BRPOP
	for {
		data, err := kafkaConsumer.ReadMessage(ctx)
		if err != nil {
			fmt.Printf("Kafka read error: %v\n", err)
			continue
		}

		var cmd BotCommand
		if err := json.Unmarshal(data, &cmd); err != nil {
			fmt.Printf("Error parsing command: %v\n", err)
			continue
		}

		// process each attack concurrently
		go processCommand(rdb, ctx, cmd)
	}
}