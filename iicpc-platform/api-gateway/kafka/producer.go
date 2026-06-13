/*
****** WHAT THIS FILE DOES ******
* Kafka producer helper for API Gateway.
* Publishes submission events to "submission-events" topic.
*/

package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/segmentio/kafka-go"
)

var writer *kafka.Writer

func InitProducer() {
	writer = &kafka.Writer{
		Addr:     kafka.TCP("kafka:9092"),
		Topic:    "submission-events",
		Balancer: &kafka.LeastBytes{},
	}
	fmt.Println("Kafka producer initialized")
}

func PublishSubmission(data interface{}) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return writer.WriteMessages(context.Background(),
		kafka.Message{Value: bytes},
	)
}

func CloseProducer() {
	if writer != nil {
		writer.Close()
	}
}