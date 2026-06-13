/*
****** WHAT THIS FILE DOES ******
* Kafka consumer for Leaderboard Service.
* Consumes scored metrics from "scored-metrics" topic.
* Replaces Redis pub/sub with proper Kafka consumption.
*
* WHY KAFKA OVER REDIS PUB/SUB?
* - Redis pub/sub drops messages if leaderboard service is down
* - Kafka retains messages — no score updates lost
* - Multiple leaderboard instances can consume independently
*/

package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

type Consumer struct {
	reader *kafka.Reader
}

func NewConsumer(topic string, groupID string) *Consumer {
	fmt.Println("Waiting for Kafka to be ready...")
	time.Sleep(15 * time.Second)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{"kafka:9092"},
		Topic:       topic,
		GroupID:     groupID,
		MinBytes:    10e3,
		MaxBytes:    10e6,
		StartOffset: kafka.LastOffset,
		MaxWait:     3 * time.Second,
	})
	fmt.Printf("Kafka consumer initialized for topic: %s\n", topic)
	return &Consumer{reader: reader}
}

func (c *Consumer) ReadMessage(ctx context.Context) ([]byte, error) {
	msg, err := c.reader.ReadMessage(ctx)
	if err != nil {
		return nil, err
	}
	return msg.Value, nil
}

func (c *Consumer) Close() {
	c.reader.Close()
}