package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/gosight/gosight/processor/internal/config"
	"github.com/gosight/gosight/processor/internal/consumer"
	"github.com/gosight/gosight/processor/internal/processor"
	"github.com/gosight/gosight/processor/internal/session"
	"github.com/gosight/gosight/processor/internal/storage"
)

func main() {
	// Setup logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Load config
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config/processor.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatal().Err(err).Str("path", configPath).Msg("Failed to load config")
	}

	log.Info().
		Strs("kafka_brokers", cfg.Kafka.Brokers).
		Str("clickhouse_addr", cfg.ClickHouse.Addr).
		Str("redis_addr", cfg.Redis.Addr).
		Int("batch_size", cfg.Batch.Size).
		Dur("flush_interval", cfg.Batch.FlushInterval).
		Msg("Configuration loaded")

	// Initialize ClickHouse
	ch, err := storage.NewClickHouse(cfg.ClickHouse)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to ClickHouse")
	}
	defer ch.Close()
	log.Info().Msg("Connected to ClickHouse")

	// Initialize session aggregator
	var sessionAgg *session.Aggregator
	if cfg.Redis.Addr != "" {
		sessionAgg = session.NewAggregator(ch, cfg.Redis)
		defer sessionAgg.Close()
		log.Info().Msg("Session aggregator initialized")
	}

	// Create event processor
	eventProcessor := processor.NewEventProcessor(ch, sessionAgg, cfg.Batch)

	// Create Kafka consumer
	kafkaConsumer, err := consumer.NewKafkaConsumer(cfg.Kafka, eventProcessor)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Kafka consumer")
	}

	// Start consuming
	ctx, cancel := context.WithCancel(context.Background())
	go kafkaConsumer.Start(ctx)

	log.Info().Msg("Event processor started")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down...")
	cancel()
	kafkaConsumer.Close()
	eventProcessor.Stop()

	// Flush remaining sessions
	if sessionAgg != nil {
		if err := sessionAgg.FlushAllSessions(context.Background()); err != nil {
			log.Error().Err(err).Msg("Failed to flush sessions")
		}
	}

	log.Info().Msg("Shutdown complete")
}
