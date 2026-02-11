package producer

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/gosight/gosight/ingestor/internal/config"
)

type KafkaProducer struct {
	writers map[string]*kafka.Writer
	topics  map[string]string
}

func NewKafkaProducer(cfg config.KafkaConfig) (*KafkaProducer, error) {
	writers := make(map[string]*kafka.Writer)

	for name, topic := range cfg.Topics {
		writers[name] = &kafka.Writer{
			Addr:         kafka.TCP(cfg.Brokers...),
			Topic:        topic,
			Balancer:     &kafka.LeastBytes{},
			BatchSize:    100,
			BatchTimeout: time.Millisecond * 100,
			Async:        true,
		}
	}

	return &KafkaProducer{
		writers: writers,
		topics:  cfg.Topics,
	}, nil
}

func (p *KafkaProducer) ProduceEvent(ctx context.Context, projectID string, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return p.writers["events"].WriteMessages(ctx, kafka.Message{
		Key:   []byte(projectID),
		Value: data,
	})
}

func (p *KafkaProducer) ProduceEventJSON(ctx context.Context, projectID string, event map[string]interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return p.writers["events"].WriteMessages(ctx, kafka.Message{
		Key:   []byte(projectID),
		Value: data,
	})
}

func (p *KafkaProducer) ProduceReplayChunk(ctx context.Context, chunk interface{}) error {
	data, err := json.Marshal(chunk)
	if err != nil {
		return err
	}

	return p.writers["replay"].WriteMessages(ctx, kafka.Message{
		Value: data,
	})
}

func (p *KafkaProducer) Close() error {
	for _, w := range p.writers {
		w.Close()
	}
	return nil
}
