package kafka

import (
	"context"
	"encoding/json"
	"log"

	"github.com/segmentio/kafka-go"
)

// Producer sends events to Kafka
type Producer struct {
	eventsWriter    *kafka.Writer
	processesWriter *kafka.Writer
}

// NewProducer creates a new Kafka producer
func NewProducer(brokers []string, eventsTopic, processesTopic string) *Producer {
	return &Producer{
		eventsWriter: &kafka.Writer{
			Addr:     kafka.TCP(brokers...),
			Topic:    eventsTopic,
			Balancer: &kafka.LeastBytes{},
		},
		processesWriter: &kafka.Writer{
			Addr:     kafka.TCP(brokers...),
			Topic:    processesTopic,
			Balancer: &kafka.LeastBytes{},
		},
	}
}

// SendEvent sends an event to the events topic
func (p *Producer) SendEvent(ctx context.Context, key string, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	msg := kafka.Message{
		Key:   []byte(key),
		Value: data,
	}

	if err := p.eventsWriter.WriteMessages(ctx, msg); err != nil {
		return err
	}

	log.Printf("Sent event to Kafka: %s", key)
	return nil
}

// SendProcess sends a process instance to the processes topic
func (p *Producer) SendProcess(ctx context.Context, key string, process any) error {
	data, err := json.Marshal(process)
	if err != nil {
		return err
	}

	msg := kafka.Message{
		Key:   []byte(key),
		Value: data,
	}

	if err := p.processesWriter.WriteMessages(ctx, msg); err != nil {
		return err
	}

	log.Printf("Sent process to Kafka: %s", key)
	return nil
}

// Close closes the Kafka writers
func (p *Producer) Close() error {
	if err := p.eventsWriter.Close(); err != nil {
		return err
	}
	return p.processesWriter.Close()
}
