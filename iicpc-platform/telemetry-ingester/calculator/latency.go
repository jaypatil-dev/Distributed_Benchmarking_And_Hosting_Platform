/*
****** WHAT THIS FILE DOES ******
* Core math engine of the telemetry system.
* Takes raw order results from the bot fleet and computes
* meaningful performance metrics including composite score.
*
* STRUCTURES:
* - OrderResult => one order sent by a bot.
*   Contains bot ID, latency in ms, success flag, price, order type
*   and timestamp.
*
* - Metrics => final computed output.
*   Contains p50, p90, p99 latencies, TPS, total orders, success rate
*   and composite score.
*
* FUNCTIONS:
* - Calculate()     => sorts latencies, extracts percentiles, calculates
*                      TPS, success rate and composite score
* - percentile()    => finds value at position (p/100 * length) in sorted array
* - PrintMetrics()  => prints formatted telemetry report to terminal
*
* COMPOSITE SCORE FORMULA:
* Score = (1000/p99) × 40  => rewards low latency       (40% weight)
*       + (TPS/100)  × 40  => rewards high throughput   (40% weight)
*       + successRate × 20 => rewards reliability        (20% weight)
*
* WHY PERCENTILES INSTEAD OF AVERAGE?
* Average latency is misleading — one 900ms outlier raises the average
* but 999 users had great experience. Percentiles tell the truth:
* - p50 = typical experience (50% of users)
* - p90 = degraded experience (10% of users)
* - p99 = worst experience (1% of users — critical in trading systems)
*/

package calculator

import (
	"fmt"
	"math"
	"sort"
	"time"
)

type OrderResult struct {
	BotID     int
	Latency   int64
	Success   bool
	Price     float64
	OrderType string
	Timestamp time.Time
}

type Metrics struct {
	P50         int64
	P90         int64
	P99         int64
	TPS         float64
	TotalOrders int
	SuccessRate float64
	Score       float64 // composite score out of ~100+
}

func Calculate(results []OrderResult) Metrics {
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

	p50 := percentile(latencies, 50)
	p90 := percentile(latencies, 90)
	p99 := percentile(latencies, 99)

	// calculate TPS from time window
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

	successRate := float64(successCount) / float64(len(results)) * 100

	// composite score — correctness (20pts) added separately by bot fleet
	score := math.Round(((1000.0/float64(p99+1))*40+
		(tps/100.0)*40+
		(successRate/100.0)*20)*10) / 10

	return Metrics{
		P50:         p50,
		P90:         p90,
		P99:         p99,
		TPS:         tps,
		TotalOrders: len(results),
		SuccessRate: successRate,
		Score:       score,
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

func PrintMetrics(m Metrics) {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("       TELEMETRY REPORT")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Total Orders  : %d\n", m.TotalOrders)
	fmt.Printf("Success Rate  : %.1f%%\n", m.SuccessRate)
	fmt.Printf("TPS           : %.0f orders/sec\n", m.TPS)
	fmt.Println("───────────────────────────────")
	fmt.Printf("p50 Latency   : %dms\n", m.P50)
	fmt.Printf("p90 Latency   : %dms\n", m.P90)
	fmt.Printf("p99 Latency   : %dms\n", m.P99)
	fmt.Println("───────────────────────────────")
	fmt.Printf("Score         : %.1f\n", m.Score)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}