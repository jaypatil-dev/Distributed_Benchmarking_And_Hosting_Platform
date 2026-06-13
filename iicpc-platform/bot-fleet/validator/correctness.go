/*
****** WHAT THIS FILE DOES ******
* This file validates the correctness of contestant trading engines.
* It sends carefully crafted orders with known expected outcomes
* and verifies the engine responds correctly.
*
* VALIDATION RULES:
* 1. Price Priority — best price gets filled first
*    => Send buy@105 and sell@100 → MUST be filled (buy > sell)
*    => Send buy@95 and sell@100  → MUST be queued (buy < sell)
*
* 2. Time Priority — earlier order at same price wins
*    => Send two buys at same price → first one must fill first
*
* 3. Fill Accuracy — no phantom or missed fills
*    => Track all expected fills vs actual fills
*    => Report accuracy percentage
*
* WHY CORRECTNESS MATTERS?
* A fast but incorrect trading engine is worse than useless.
* In real markets, incorrect order matching means:
* - Traders get wrong prices
* - Market manipulation becomes possible
* - Exchange faces legal liability
*
* SCORING IMPACT:
* Correctness score contributes 20% to final composite score
* A completely incorrect engine gets 0 correctness points
* regardless of how fast it is
*/

package validator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ValidationResult holds the result of one correctness test
type ValidationResult struct {
	TestName    string
	Expected    string
	Actual      string
	Passed      bool
	Description string
}

// CorrectnessReport holds all validation results
type CorrectnessReport struct {
	TotalTests  int
	PassedTests int
	FailedTests int
	Accuracy    float64
	Results     []ValidationResult
	Score       float64
}

// Order represents a trading order
type Order struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

// OrderResponse represents engine response
type OrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

var client = &http.Client{Timeout: 5 * time.Second}

// sendOrder sends a single order and returns the response
func sendOrder(endpoint string, order Order) (string, error) {
	body, err := json.Marshal(order)
	if err != nil {
		return "", err
	}

	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var orderResp OrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&orderResp); err != nil {
		return "", err
	}

	return orderResp.Status, nil
}

// ValidateEngine runs all correctness tests against contestant engine
func ValidateEngine(endpoint string) CorrectnessReport {
	fmt.Println("🔍 Starting correctness validation...")
	var results []ValidationResult

	// ── TEST 1: Basic fill — buy price > sell price ──
	// send sell first then buy at higher price → must fill
	sellOrder := Order{
		ID:       fmt.Sprintf("val-sell-%d", time.Now().UnixNano()),
		Type:     "sell",
		Price:    100.0,
		Quantity: 10,
	}
	sendOrder(endpoint, sellOrder)
	time.Sleep(10 * time.Millisecond)

	buyOrder := Order{
		ID:       fmt.Sprintf("val-buy-%d", time.Now().UnixNano()),
		Type:     "buy",
		Price:    105.0,
		Quantity: 10,
	}
	status, err := sendOrder(endpoint, buyOrder)
	results = append(results, ValidationResult{
		TestName:    "Basic Fill Test",
		Expected:    "filled",
		Actual:      status,
		Passed:      err == nil && status == "filled",
		Description: "buy@105 with sell@100 already queued → must fill",
	})

	// ── TEST 2: No fill — buy price < sell price ──
	sellOrder2 := Order{
		ID:       fmt.Sprintf("val-sell2-%d", time.Now().UnixNano()),
		Type:     "sell",
		Price:    110.0,
		Quantity: 10,
	}
	sendOrder(endpoint, sellOrder2)
	time.Sleep(10 * time.Millisecond)

	buyOrder2 := Order{
		ID:       fmt.Sprintf("val-buy2-%d", time.Now().UnixNano()),
		Type:     "buy",
		Price:    95.0,
		Quantity: 10,
	}
	status2, err2 := sendOrder(endpoint, buyOrder2)
	results = append(results, ValidationResult{
		TestName:    "No Fill Test",
		Expected:    "queued",
		Actual:      status2,
		Passed:      err2 == nil && status2 == "queued",
		Description: "buy@95 with sell@110 already queued → must NOT fill",
	})

	// ── TEST 3: Exact price match ──
	sellOrder3 := Order{
		ID:       fmt.Sprintf("val-sell3-%d", time.Now().UnixNano()),
		Type:     "sell",
		Price:    100.0,
		Quantity: 5,
	}
	sendOrder(endpoint, sellOrder3)
	time.Sleep(10 * time.Millisecond)

	buyOrder3 := Order{
		ID:       fmt.Sprintf("val-buy3-%d", time.Now().UnixNano()),
		Type:     "buy",
		Price:    100.0,
		Quantity: 5,
	}
	status3, err3 := sendOrder(endpoint, buyOrder3)
	results = append(results, ValidationResult{
		TestName:    "Exact Price Match Test",
		Expected:    "filled",
		Actual:      status3,
		Passed:      err3 == nil && status3 == "filled",
		Description: "buy@100 with sell@100 → must fill (equal prices match)",
	})

	// ── TEST 4: Multiple fills ──
	// send 3 sell orders then a large buy
	for i := 0; i < 3; i++ {
		sellOrderM := Order{
			ID:       fmt.Sprintf("val-sellm-%d-%d", i, time.Now().UnixNano()),
			Type:     "sell",
			Price:    99.0,
			Quantity: 1,
		}
		sendOrder(endpoint, sellOrderM)
		time.Sleep(5 * time.Millisecond)
	}

	buyOrderM := Order{
		ID:       fmt.Sprintf("val-buym-%d", time.Now().UnixNano()),
		Type:     "buy",
		Price:    102.0,
		Quantity: 1,
	}
	status4, err4 := sendOrder(endpoint, buyOrderM)
	results = append(results, ValidationResult{
		TestName:    "Multiple Sellers Test",
		Expected:    "filled",
		Actual:      status4,
		Passed:      err4 == nil && status4 == "filled",
		Description: "buy@102 with multiple sells@99 queued → must fill",
	})

	// ── TEST 5: Price priority ──
	// send two sells at different prices, buy should match cheapest
	sellHigh := Order{
		ID:       fmt.Sprintf("val-sellhigh-%d", time.Now().UnixNano()),
		Type:     "sell",
		Price:    108.0,
		Quantity: 10,
	}
	sendOrder(endpoint, sellHigh)
	time.Sleep(5 * time.Millisecond)

	sellLow := Order{
		ID:       fmt.Sprintf("val-selllow-%d", time.Now().UnixNano()),
		Type:     "sell",
		Price:    101.0,
		Quantity: 10,
	}
	sendOrder(endpoint, sellLow)
	time.Sleep(5 * time.Millisecond)

	buyPriority := Order{
		ID:       fmt.Sprintf("val-buypriority-%d", time.Now().UnixNano()),
		Type:     "buy",
		Price:    103.0,
		Quantity: 10,
	}
	status5, err5 := sendOrder(endpoint, buyPriority)
	results = append(results, ValidationResult{
		TestName:    "Price Priority Test",
		Expected:    "filled",
		Actual:      status5,
		Passed:      err5 == nil && status5 == "filled",
		Description: "buy@103 with sell@101 and sell@108 queued → must fill with cheapest sell",
	})

	// calculate report
	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}

	accuracy := float64(passed) / float64(len(results)) * 100
	score := accuracy / 100.0 * 20.0 // max 20 points for correctness

	report := CorrectnessReport{
		TotalTests:  len(results),
		PassedTests: passed,
		FailedTests: len(results) - passed,
		Accuracy:    accuracy,
		Results:     results,
		Score:       score,
	}

	printReport(report)
	return report
}

func printReport(r CorrectnessReport) {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("   CORRECTNESS VALIDATION REPORT")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	for _, result := range r.Results {
		status := "✅ PASS"
		if !result.Passed {
			status = "❌ FAIL"
		}
		fmt.Printf("%s %s\n", status, result.TestName)
		fmt.Printf("     Expected: %s | Got: %s\n", result.Expected, result.Actual)
		fmt.Printf("     %s\n", result.Description)
	}
	fmt.Println("───────────────────────────────")
	fmt.Printf("Tests Passed  : %d/%d\n", r.PassedTests, r.TotalTests)
	fmt.Printf("Accuracy      : %.1f%%\n", r.Accuracy)
	fmt.Printf("Correctness   : %.1f/20 points\n", r.Score)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}