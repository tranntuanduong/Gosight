package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/gosight/gosight/processor/internal/config"
	"github.com/gosight/gosight/processor/internal/consumer"
	"github.com/gosight/gosight/processor/internal/insights"
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
		Msg("Configuration loaded")

	// Initialize ClickHouse
	ch, err := storage.NewClickHouse(cfg.ClickHouse)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to ClickHouse")
	}
	defer ch.Close()
	log.Info().Msg("Connected to ClickHouse")

	// Initialize Redis
	var rdb *redis.Client
	if cfg.Redis.Addr != "" {
		rdb = redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})

		// Test connection
		if err := rdb.Ping(context.Background()).Err(); err != nil {
			log.Warn().Err(err).Msg("Failed to connect to Redis, some detectors will be disabled")
			rdb = nil
		} else {
			defer rdb.Close()
			log.Info().Msg("Connected to Redis")
		}
	}

	// Enable all detectors by default if not configured
	if !cfg.Insights.RageClick.Enabled && !cfg.Insights.DeadClick.Enabled &&
		!cfg.Insights.ErrorClick.Enabled && !cfg.Insights.ThrashedCursor.Enabled &&
		!cfg.Insights.UTurn.Enabled && !cfg.Insights.SlowPage.Enabled {
		log.Info().Msg("No insight detectors enabled in config, enabling all by default")
		cfg.Insights.RageClick.Enabled = true
		cfg.Insights.DeadClick.Enabled = true
		cfg.Insights.ErrorClick.Enabled = true
		cfg.Insights.ThrashedCursor.Enabled = true
		cfg.Insights.UTurn.Enabled = true
		cfg.Insights.SlowPage.Enabled = true
	}

	// Create insight processor with Kafka alert publishing
	insightProcessor := insights.NewProcessorWithKafka(ch, rdb, cfg.Insights, cfg.Kafka)

	// Override consumer group for insight processor
	cfg.Kafka.ConsumerGroup = "gosight-insight-processor"

	// Create Kafka consumer
	kafkaConsumer, err := consumer.NewKafkaConsumer(cfg.Kafka, insightProcessor)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Kafka consumer")
	}

	// Start consuming
	ctx, cancel := context.WithCancel(context.Background())
	go kafkaConsumer.Start(ctx)

	log.Info().
		Bool("rage_click", cfg.Insights.RageClick.Enabled).
		Bool("dead_click", cfg.Insights.DeadClick.Enabled).
		Bool("error_click", cfg.Insights.ErrorClick.Enabled).
		Bool("thrashed_cursor", cfg.Insights.ThrashedCursor.Enabled).
		Bool("u_turn", cfg.Insights.UTurn.Enabled).
		Bool("slow_page", cfg.Insights.SlowPage.Enabled).
		Msg("Insight processor started")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down...")
	cancel()
	kafkaConsumer.Close()
	insightProcessor.Stop()

	log.Info().Msg("Shutdown complete")
}
