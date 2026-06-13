/*
****** WHAT THIS FILE DOES ******
* This is the core of the real bot fleet service.
* Unlike the simulated bots in telemetry-ingester, these bots
* send REAL HTTP requests to contestant's trading engine endpoints.
*
* FUNCTIONS:
* - Bot => represents a single trading bot with an ID and target URL
*
* - NewBot() => creates a new bot with a unique ID and target endpoint
*
* - SendOrder() => sends a real HTTP POST request to contestant's engine
*   Measures actual round-trip latency including:
*   → Network time
*   → Contestant's parsing time
*   → Order book processing time
*   → Response serialization time
*   This is TRUE latency — not simulated
*
* - GenerateOrder() => generates a realistic trading order
*   Random buy/sell, random price between 90-110, random quantity
*   Simulates diverse market participants
*
* - RunFleet() => spawns N bots concurrently using goroutines
*   Each bot sends one order and reports result via channel
*   Uses WaitGroup to wait for all bots to finish
*   Returns slice of all results for telemetry calculation
*
* WHY REAL HTTP REQUESTS?
* Simulated latency tells us nothing about contestant code quality.
* Real HTTP requests measure actual performance under load —
* how fast their order book processes concurrent requests,
* whether they have race conditions, memory leaks, or bottlenecks.
*
* WHY 1000 CONCURRENT BOTS?
* Real trading exchanges handle thousands of concurrent orders.
* 1000 goroutines in Go uses ~2MB of memory total — extremely efficient.
* This simulates peak market volatility as required by IICPC.
*/

package attacker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// OrderResult represents the result of one bot's order
type OrderResult struct {
	BotID     int
	Latency   int64 // milliseconds
	Success   bool
	OrderType string
	Price     float64
	Timestamp time.Time
	Error     string
}

// Order represents a trading order sent to contestant engine
type Order struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

// Bot represents a single trading bot
type Bot struct {
	ID        int
	TargetURL string
	Client    *http.Client
}

// NewBot creates a new bot with timeout
func NewBot(id int, targetURL string) *Bot {
	return &Bot{
		ID:        id,
		TargetURL: targetURL,
		Client: &http.Client{
			Timeout: 5 * time.Second, // 5 second timeout per request
		},
	}
}

// GenerateOrder creates a realistic random trading order
func (b *Bot) GenerateOrder() Order {
	orderTypes := []string{"buy", "sell"}
	return Order{
		ID:       fmt.Sprintf("order-%d-%d", b.ID, time.Now().UnixNano()),
		Type:     orderTypes[rand.Intn(2)],
		Price:    90.0 + rand.Float64()*20.0, // price between 90-110
		Quantity: rand.Intn(100) + 1,          // quantity between 1-100
	}
}

// SendOrder sends a real HTTP request to contestant's engine
func (b *Bot) SendOrder(endpoint string) OrderResult {
	order := b.GenerateOrder()

	// marshal order to JSON
	body, err := json.Marshal(order)
	if err != nil {
		return OrderResult{
			BotID:     b.ID,
			Success:   false,
			Timestamp: time.Now(),
			Error:     err.Error(),
		}
	}

	// measure real latency
	start := time.Now()

	resp, err := b.Client.Post(
		endpoint,
		"application/json",
		bytes.NewBuffer(body),
	)

	latency := time.Since(start).Milliseconds()

	if err != nil {
		return OrderResult{
			BotID:     b.ID,
			Latency:   latency,
			Success:   false,
			OrderType: order.Type,
			Price:     order.Price,
			Timestamp: time.Now(),
			Error:     err.Error(),
		}
	}
	defer resp.Body.Close()

	success := resp.StatusCode == http.StatusOK

	return OrderResult{
		BotID:     b.ID,
		Latency:   latency,
		Success:   success,
		OrderType: order.Type,
		Price:     order.Price,
		Timestamp: time.Now(),
	}
}

// RunFleet spawns N bots and attacks the target endpoint concurrently
func RunFleet(targetURL string, numBots int) []OrderResult {
	fmt.Printf("🤖 Launching %d bots against %s\n", numBots, targetURL)

	results := make(chan OrderResult, numBots)
	var wg sync.WaitGroup

	start := time.Now()

	for i := 1; i <= numBots; i++ {
		wg.Add(1)
		go func(botID int) {
			defer wg.Done()
			bot := NewBot(botID, targetURL)
			result := bot.SendOrder(targetURL)
			results <- result
		}(i)
	}

	// close channel when all bots finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// collect all results
	var allResults []OrderResult
	for result := range results {
		allResults = append(allResults, result)
	}

	elapsed := time.Since(start)
	fmt.Printf("✅ All %d bots completed in %v\n", numBots, elapsed)

	return allResults
}