/*
****** WHAT THIS FILE DOES ******
* This file handles real-time leaderboard updates via Redis.
* It is the bridge between the telemetry engine and the live frontend.
*
* STRUCTURES:
* - RedisStore => holds the Redis client and context.
* - LeaderboardEntry => the full scored entry pushed to the leaderboard.
*   Contains all metrics plus a composite score.
*
* FUNCTIONS:
* - NewRedisStore() => connects to Redis and pings to verify connection.
*
* - UpdateLeaderboard() => three operations in one:
*   1. Calculates composite score:
*      → 40% weight on p99 latency (lower is better)
*      → 40% weight on TPS (higher is better)
*      → 20% weight on success rate (higher is better)
*   2. Stores full entry as JSON => instant retrieval for any client
*   3. Publishes update to "leaderboard-updates" channel => all
*      connected browsers receive it instantly via WebSocket
*
* - GetLeaderboard() => returns top 10 contestants from Redis sorted set.
*   ZRevRangeWithScores returns members in descending score order.
*
* WHY REDIS SORTED SETS?
* Redis ZSets automatically maintain sorted order by score.
* Adding a contestant or updating their score is O(log N) — extremely
* fast even with thousands of contestants.
* ZRevRange gives top N contestants in O(log N + N) time.
*
* WHY REDIS PUB/SUB FOR LEADERBOARD?
* Instead of the frontend polling every second (wasteful), Redis pub/sub
* pushes updates only when scores change. The leaderboard service
* subscribes to the channel and forwards updates to browsers via
* WebSocket — achieving true real-time updates with zero polling.
*
* COMPOSITE SCORE FORMULA:
* Score = (1000/p99) × 40 + (TPS/100) × 40 + successRate × 20
* This rewards systems that are simultaneously fast, high throughput
* and reliable — matching real-world trading engine requirements.
*/

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"telemetry-ingester/calculator"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
	ctx    context.Context
}

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

func NewRedisStore() (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})

	ctx := context.Background()

	// test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %v", err)
	}

	fmt.Println("Connected to Redis successfully!")
	return &RedisStore{client: client, ctx: ctx}, nil
}

func (r *RedisStore) UpdateLeaderboard(submissionID, contestant string, m calculator.Metrics) error {
	// calculate composite score
	// lower latency = higher score, higher TPS = higher score
	score := (1000.0/float64(m.P99+1))*40 +
		(float64(m.TPS)/100.0)*40 +
		(m.SuccessRate/100.0)*20

	entry := LeaderboardEntry{
		Contestant:   contestant,
		SubmissionID: submissionID,
		P50:          m.P50,
		P90:          m.P90,
		P99:          m.P99,
		TPS:          m.TPS,
		SuccessRate:  m.SuccessRate,
		Score:        score,
	}

	// save entry as JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	// store in Redis with contestant as key
	r.client.Set(r.ctx, "contestant:"+contestant, data, 0)

	// add to sorted leaderboard set
	r.client.ZAdd(r.ctx, "leaderboard", redis.Z{
		Score:  score,
		Member: contestant,
	})

	// publish live update to leaderboard frontend
	r.client.Publish(r.ctx, "leaderboard-updates", data)

	fmt.Printf("Leaderboard updated for %s → Score: %.1f\n", contestant, score)
	return nil
}

func (r *RedisStore) GetLeaderboard() ([]LeaderboardEntry, error) {
	// get top 10 contestants sorted by score
	results, err := r.client.ZRevRangeWithScores(r.ctx, "leaderboard", 0, 9).Result()
	if err != nil {
		return nil, err
	}

	var entries []LeaderboardEntry
	for _, result := range results {
		data, err := r.client.Get(r.ctx, "contestant:"+result.Member.(string)).Result()
		if err != nil {
			continue
		}

		var entry LeaderboardEntry
		if err := json.Unmarshal([]byte(data), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (r *RedisStore) Close() {
	r.client.Close()
}