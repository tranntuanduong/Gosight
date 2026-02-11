package consumer

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"

	"github.com/gosight/gosight/processor/internal/config"
)

// MessageProcessor interface for processing messages
type MessageProcessor interface {
	Process(ctx context.Context, event map[string]interface{}) error
	Flush()
}

// KafkaConsumer consumes messages from Kafka
type KafkaConsumer struct {
	reader    *kafka.Reader
	processor MessageProcessor
}

// NewKafkaConsumer creates a new Kafka consumer
func NewKafkaConsumer(cfg config.KafkaConfig, processor MessageProcessor) (*KafkaConsumer, error) {
	topic := cfg.Topics["events"]
	if topic == "" {
		topic = "gosight.events.raw"
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		Topic:          topic,
		GroupID:        cfg.ConsumerGroup,
		MinBytes:       1e3,  // 1KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: 1000,
		StartOffset:    kafka.LastOffset,
	})

	return &KafkaConsumer{
		reader:    reader,
		processor: processor,
	}, nil
}

// Start begins consuming messages
func (c *KafkaConsumer) Start(ctx context.Context) {
	log.Info().
		Str("topic", c.reader.Config().Topic).
		Str("group", c.reader.Config().GroupID).
		Msg("Starting Kafka consumer")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Kafka consumer stopped")
			return
		default:
			msg, err := c.reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Error().Err(err).Msg("Failed to fetch message")
				continue
			}

			// Parse message
			var event map[string]interface{}
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				log.Error().
					Err(err).
					Str("value", string(msg.Value)).
					Msg("Failed to parse message")
				// Still commit to avoid getting stuck
				if err := c.reader.CommitMessages(ctx, msg); err != nil {
					log.Error().Err(err).Msg("Failed to commit message")
				}
				continue
			}

			// Process event
			if err := c.processor.Process(ctx, event); err != nil {
				log.Error().
					Err(err).
					Interface("event", event).
					Msg("Failed to process event")
			}

			// Commit
			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				log.Error().Err(err).Msg("Failed to commit message")
			}
		}
	}
}

// Close closes the consumer
func (c *KafkaConsumer) Close() error {
	log.Info().Msg("Closing Kafka consumer")
	// Flush remaining events before closing
	c.processor.Flush()
	return c.reader.Close()
}
