package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Order represents a trading order
type Order struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"` // "buy" or "sell"
	Price     float64 `json:"price"`
	Quantity  int     `json:"quantity"`
	Timestamp int64   `json:"timestamp"`
}

// OrderBook maintains buy and sell orders with price-time priority
type OrderBook struct {
	mu         sync.Mutex
	buyOrders  []Order
	sellOrders []Order
	trades     []string
}

// AddOrder adds a new order to the order book
func (ob *OrderBook) AddOrder(order Order) string {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	order.Timestamp = time.Now().UnixNano()

	if order.Type == "buy" {
		ob.buyOrders = append(ob.buyOrders, order)
		// sort buy orders by price descending (highest price first)
		sort.Slice(ob.buyOrders, func(i, j int) bool {
			if ob.buyOrders[i].Price != ob.buyOrders[j].Price {
				return ob.buyOrders[i].Price > ob.buyOrders[j].Price
			}
			return ob.buyOrders[i].Timestamp < ob.buyOrders[j].Timestamp
		})
	} else {
		ob.sellOrders = append(ob.sellOrders, order)
		// sort sell orders by price ascending (lowest price first)
		sort.Slice(ob.sellOrders, func(i, j int) bool {
			if ob.sellOrders[i].Price != ob.sellOrders[j].Price {
				return ob.sellOrders[i].Price < ob.sellOrders[j].Price
			}
			return ob.sellOrders[i].Timestamp < ob.sellOrders[j].Timestamp
		})
	}

	// try to match orders
	return ob.matchOrders()
}

// matchOrders matches buy and sell orders by price-time priority
func (ob *OrderBook) matchOrders() string {
	if len(ob.buyOrders) == 0 || len(ob.sellOrders) == 0 {
		return "queued"
	}

	bestBuy := ob.buyOrders[0]
	bestSell := ob.sellOrders[0]

	if bestBuy.Price >= bestSell.Price {
		trade := fmt.Sprintf("MATCH: buy@%.2f x sell@%.2f",
			bestBuy.Price, bestSell.Price)
		ob.trades = append(ob.trades, trade)
		ob.buyOrders = ob.buyOrders[1:]
		ob.sellOrders = ob.sellOrders[1:]
		return "filled"
	}

	return "queued"
}

var orderBook = &OrderBook{}

func handleOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var order Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, "invalid order", http.StatusBadRequest)
		return
	}

	status := orderBook.AddOrder(order)

	// add this line
	fmt.Printf("Order received → type: %s price: %.2f status: %s\n", order.Type, order.Price, status)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"order_id": order.ID,
		"status":   status,
	})
}


func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"engine": "order-book-v1",
	})
}

func main() {
	http.HandleFunc("/order", handleOrder)
	http.HandleFunc("/health", handleHealth)

	fmt.Println("Trading Engine starting on :9000")
	http.ListenAndServe(":9000", nil)
}