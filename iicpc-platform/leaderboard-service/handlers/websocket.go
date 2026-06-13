/*
****** WHAT THIS FILE DOES ******
* Handles WebSocket connections and streams live leaderboard
* updates to all connected browsers.
*
* FUNCTIONS:
* - WebSocketHandler()       => upgrades HTTP to WebSocket, sends current
*                               leaderboard on connect, listens for updates
* - sendCurrentLeaderboard() => fetches top 10 from Redis sorted set and
*                               sends as JSON array to newly connected browser
* - BroadcastScore()         => pushes a score update to all connected clients
*                               called by Kafka consumer in main.go
*
* WHY WEBSOCKETS?
* Server pushes updates the moment they arrive — no polling needed.
* Reduces network traffic by 90% vs polling for a live leaderboard.
*
* WHY GORILLA WEBSOCKET?
* Most battle-tested Go WebSocket library — used by Docker and Kubernetes.
*
* SECURITY NOTE:
* CheckOrigin returns true for all origins in development.
* In production restrict to platform domain to prevent CSWSH attacks.
*/

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all origins in development
	},
}

// ─── Connected Clients Hub ────────────────────────────────────
// tracks all active WebSocket connections for broadcasting

var (
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex
)

type LeaderboardEntry struct {
	Contestant  string  `json:"contestant"`
	Score       float64 `json:"score"`
	P50         int64   `json:"p50"`
	P90         int64   `json:"p90"`
	P99         int64   `json:"p99"`
	TPS         float64 `json:"tps"`
	SuccessRate float64 `json:"success_rate"`
}

func WebSocketHandler(rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// upgrade HTTP to WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Printf("WebSocket upgrade error: %v\n", err)
			return
		}
		defer func() {
			// remove from hub on disconnect
			clientsMu.Lock()
			delete(clients, conn)
			clientsMu.Unlock()
			conn.Close()
			fmt.Println("Browser disconnected from leaderboard")
		}()

		// register in hub
		clientsMu.Lock()
		clients[conn] = true
		clientsMu.Unlock()

		fmt.Println("New browser connected to leaderboard!")

		ctx := context.Background()

		// send current leaderboard immediately on connect
		sendCurrentLeaderboard(conn, rdb, ctx)

		// keep connection alive — read loop detects disconnects
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}
}

// BroadcastScore pushes a score update to all connected WebSocket clients
// called by the Kafka consumer in main.go when a new score arrives
func BroadcastScore(data []byte) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	for conn := range clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			fmt.Printf("WebSocket write error: %v\n", err)
			// remove dead connection
			delete(clients, conn)
			conn.Close()
		}
	}
	fmt.Printf("Score broadcast to %d connected browsers\n", len(clients))
}

func sendCurrentLeaderboard(conn *websocket.Conn, rdb *redis.Client, ctx context.Context) {
	// get top 10 from Redis sorted set
	results, err := rdb.ZRevRangeWithScores(ctx, "leaderboard", 0, 9).Result()
	if err != nil {
		fmt.Printf("Error getting leaderboard: %v\n", err)
		return
	}

	var entries []LeaderboardEntry
	for _, result := range results {
		data, err := rdb.Get(ctx, "contestant:"+result.Member.(string)).Result()
		if err != nil {
			continue
		}

		var entry LeaderboardEntry
		if err := json.Unmarshal([]byte(data), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return
	}

	conn.WriteMessage(websocket.TextMessage, data)
	fmt.Printf("Sent current leaderboard (%d entries) to new connection\n", len(entries))
}