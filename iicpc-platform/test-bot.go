package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type BotCommand struct {
	SubmissionID string `json:"submission_id"`
	Contestant   string `json:"contestant"`
	TargetURL    string `json:"target_url"`
	NumBots      int    `json:"num_bots"`
}

func main() {
	ctx := context.Background()

	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	cmd := BotCommand{
		SubmissionID: "test-001",
		Contestant:   "Yash",
		TargetURL:    "http://host.docker.internal:9000/order",
		NumBots:      100,
	}

	data, _ := json.Marshal(cmd)
	rdb.LPush(ctx, "bot-commands", string(data))
	fmt.Println("Bot command sent successfully!")
}