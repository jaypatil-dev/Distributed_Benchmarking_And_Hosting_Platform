/*
****** WHAT THIS FILE DOES ******
* Kafka consumer for Telemetry Ingester.
* Consumes submission events from "submission-events" topic.
* This replaces the Redis BRPOP polling with proper Kafka consumption.
*
* WHY KAFKA OVER REDIS QUEUE?
* - Messages persist even if telemetry ingester crashes
* - Multiple instances can consume in parallel (partitions)
* - Message replay possible for debugging
* - Industry standard for event streaming
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
	// wait for Kafka to be ready
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