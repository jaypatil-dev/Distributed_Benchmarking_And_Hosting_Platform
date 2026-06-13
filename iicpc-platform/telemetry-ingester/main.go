/*
****** WHAT THIS FILE DOES ******
* Entry point of the Telemetry Ingester service.
* Consumes submissions from Kafka, scores them and publishes results.
*
* FLOW:
* 1. Connects to Redis, TimescaleDB and Kafka
* 2. Kafka consumer listens on "submission-events" topic
* 3. When a new submission arrives:
*    => Updates status to RUNNING in Redis
*    => Publishes bot command to Kafka "bot-commands" topic
*    => Runs simulated bots for immediate scoring
*    => Calculates p50/p90/p99 latency and TPS
*    => Saves metrics to TimescaleDB
*    => Publishes scored metrics to Kafka "scored-metrics" topic
*    => Updates status to COMPLETED in Redis
*
* METRICS EXPOSED (port 2112):
* - telemetry_submissions_processed_total  => total submissions scored
* - telemetry_active_submissions           => currently processing count
* - telemetry_scoring_duration_seconds     => time taken to score a submission
* - telemetry_bot_success_rate            => success rate from simulated bots
* - telemetry_p99_latency_ms             => p99 latency of last scored submission
*
* WHY KAFKA OVER REDIS QUEUE?
* Kafka provides guaranteed delivery, message persistence and horizontal
* scaling. Redis queue drops messages if service crashes — Kafka does not.
*
* WHY REDIS STILL USED?
* Redis stores submission status (RUNNING/COMPLETED) so the status
* polling endpoint in api-gateway can check progress in real time.
* Leaderboard sorted set is managed by leaderboard-service via Kafka.
*/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"telemetry-ingester/calculator"
	kafkapkg "telemetry-ingester/kafka"
	"telemetry-ingester/storage"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// Submission matches the structure stored by API Gateway in Redis
type Submission struct {
	ID         string    `json:"id"`
	Contestant string    `json:"contestant"`
	Language   string    `json:"language"`
	Code       string    `json:"code"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// BotCommand is sent to bot fleet via Kafka
type BotCommand struct {
	SubmissionID string `json:"submission_id"`
	Contestant   string `json:"contestant"`
	TargetURL    string `json:"target_url"`
	NumBots      int    `json:"num_bots"`
}

// ScoredMetrics is published to leaderboard service via Kafka
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

// ─── Prometheus Metrics ───────────────────────────────────────

var (
	submissionsProcessed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "telemetry_submissions_processed_total",
			Help: "Total number of submissions scored",
		},
	)

	activeSubmissions = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "telemetry_active_submissions",
			Help: "Number of submissions currently being processed",
		},
	)

	scoringDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "telemetry_scoring_duration_seconds",
			Help:    "Time taken to fully score a submission",
			Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30},
		},
	)

	botSuccessRate = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "telemetry_bot_success_rate",
			Help: "Success rate of simulated bots for last scored submission",
		},
	)

	p99Latency = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "telemetry_p99_latency_ms",
			Help: "P99 latency in milliseconds of last scored submission",
		},
	)
)

func init() {
	prometheus.MustRegister(submissionsProcessed)
	prometheus.MustRegister(activeSubmissions)
	prometheus.MustRegister(scoringDuration)
	prometheus.MustRegister(botSuccessRate)
	prometheus.MustRegister(p99Latency)
}

// simulates a single trading bot with realistic latency
func simulateBot(botID int, results chan calculator.OrderResult, wg *sync.WaitGroup) {
	defer wg.Done()

	latency := int64(rand.Intn(90) + 10)
	if rand.Intn(100) == 0 {
		// 1% slow outliers simulate real network conditions
		latency = int64(rand.Intn(400) + 200)
	}

	time.Sleep(time.Millisecond * time.Duration(latency))

	results <- calculator.OrderResult{
		BotID:     botID,
		Latency:   latency,
		Success:   rand.Intn(100) > 2,
		Price:     float64(rand.Intn(1000)) + rand.Float64(),
		OrderType: []string{"buy", "sell"}[rand.Intn(2)],
		Timestamp: time.Now(),
	}
}

func processSubmission(sub Submission, rdb *redis.Client, db *storage.TimescaleDB, ctx context.Context) {
	fmt.Printf("\n🔄 Processing submission: %s by %s\n", sub.ID, sub.Contestant)

	activeSubmissions.Inc()
	timer := prometheus.NewTimer(scoringDuration)
	defer func() {
		activeSubmissions.Dec()
		timer.ObserveDuration()
	}()

	// update status to running in Redis
	sub.Status = "running"
	data, _ := json.Marshal(sub)
	rdb.Set(ctx, "submission:"+sub.ID, data, 24*time.Hour)

	// publish bot command to Kafka
	botCmd := BotCommand{
		SubmissionID: sub.ID,
		Contestant:   sub.Contestant,
		TargetURL:    "http://test-engine:9000/order",
		NumBots:      1000,
	}
	if err := kafkapkg.PublishBotCommand(botCmd); err != nil {
		fmt.Printf("Kafka bot command error: %v\n", err)
	} else {
		fmt.Printf("🤖 Bot command published to Kafka for %s\n", sub.Contestant)
	}

	// run simulated bots for immediate scoring
	totalBots := 1000
	results := make(chan calculator.OrderResult, totalBots)
	var wg sync.WaitGroup

	for i := 1; i <= totalBots; i++ {
		wg.Add(1)
		go simulateBot(i, results, &wg)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var allResults []calculator.OrderResult
	for result := range results {
		allResults = append(allResults, result)
	}

	// calculate metrics and update prometheus
	metrics := calculator.Calculate(allResults)
	calculator.PrintMetrics(metrics)
	botSuccessRate.Set(metrics.SuccessRate)
	p99Latency.Set(float64(metrics.P99))

	// save to timescaledb
	db.SaveOrderMetrics(sub.ID, sub.Contestant, allResults)
	db.SaveScore(sub.ID, sub.Contestant, metrics)

	// publish scored metrics to Kafka — leaderboard service handles Redis update
	scored := ScoredMetrics{
		Contestant:   sub.Contestant,
		SubmissionID: sub.ID,
		P50:          metrics.P50,
		P90:          metrics.P90,
		P99:          metrics.P99,
		TPS:          metrics.TPS,
		SuccessRate:  metrics.SuccessRate,
		Score:        metrics.Score,
	}
	if err := kafkapkg.PublishScoredMetrics(scored); err != nil {
		fmt.Printf("Kafka scored metrics error: %v\n", err)
	} else {
		fmt.Printf("📊 Scored metrics published to Kafka for %s\n", sub.Contestant)
	}

	// update status to completed in Redis
	sub.Status = "completed"
	data, _ = json.Marshal(sub)
	rdb.Set(ctx, "submission:"+sub.ID, data, 24*time.Hour)

	submissionsProcessed.Inc()
	fmt.Printf("✅ Submission %s completed!\n", sub.ID)
}

func main() {
	fmt.Println("IICPC Platform - Telemetry Ingester starting...")

	ctx := context.Background()

	// connect to Redis — used only for submission status updates
	rdb := redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Printf("Redis error: %v\n", err)
		return
	}
	fmt.Println("Connected to Redis successfully!")

	// connect to TimescaleDB
	db, err := storage.NewTimescaleDB()
	if err != nil {
		fmt.Printf("TimescaleDB error: %v\n", err)
		return
	}
	defer db.Close()

	// initialize Kafka producers for bot-commands and scored-metrics
	kafkapkg.InitProducer()
	defer kafkapkg.CloseProducer()

	// initialize Kafka consumer for submission-events
	kafkaConsumer := kafkapkg.NewConsumer("submission-events", "telemetry-group")
	defer kafkaConsumer.Close()

	// start prometheus metrics server on port 2112
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		fmt.Println("Metrics server running on :2112")
		http.ListenAndServe(":2112", nil)
	}()

	fmt.Println("Telemetry Ingester running — listening on Kafka submission-events...")

	// consume from Kafka — single source, no double processing
	for {
		data, err := kafkaConsumer.ReadMessage(ctx)
		if err != nil {
			fmt.Printf("Kafka read error: %v\n", err)
			continue
		}

		var sub Submission
		if err := json.Unmarshal(data, &sub); err != nil {
			fmt.Printf("Kafka parse error: %v\n", err)
			continue
		}

		fmt.Printf("\n📨 Kafka submission received: %s by %s\n", sub.ID, sub.Contestant)
		go processSubmission(sub, rdb, db, ctx)
	}
}