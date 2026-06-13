/*
****** WHAT THIS FILE DOES ******
* Kafka producer for Telemetry Ingester.
* Publishes to two topics:
* - "bot-commands"   => triggers bot fleet to attack contestant engine
* - "scored-metrics" => sends final scores to leaderboard service
*
* WHY TWO TOPICS?
* Clean separation of concerns — bot fleet only cares about attack
* commands, leaderboard only cares about final scores.
* Each service consumes only what it needs.
*/

package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/segmentio/kafka-go"
)

var (
	botCommandsWriter  *kafka.Writer
	scoredMetricsWriter *kafka.Writer
)

func InitProducer() {
	// producer for bot fleet attack commands
	botCommandsWriter = &kafka.Writer{
		Addr:     kafka.TCP("kafka:9092"),
		Topic:    "bot-commands",
		Balancer: &kafka.LeastBytes{},
	}

	// producer for scored metrics to leaderboard
	scoredMetricsWriter = &kafka.Writer{
		Addr:     kafka.TCP("kafka:9092"),
		Topic:    "scored-metrics",
		Balancer: &kafka.LeastBytes{},
	}

	fmt.Println("Kafka producers initialized (bot-commands, scored-metrics)")
}

// PublishBotCommand sends attack command to bot fleet
func PublishBotCommand(data interface{}) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return botCommandsWriter.WriteMessages(context.Background(),
		kafka.Message{Value: bytes},
	)
}

// PublishScoredMetrics sends final score to leaderboard service
func PublishScoredMetrics(data interface{}) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return scoredMetricsWriter.WriteMessages(context.Background(),
		kafka.Message{Value: bytes},
	)
}

func CloseProducer() {
	if botCommandsWriter != nil {
		botCommandsWriter.Close()
	}
	if scoredMetricsWriter != nil {
		scoredMetricsWriter.Close()
	}
}