package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/gosight/gosight/ingestor/internal/config"
	"github.com/gosight/gosight/ingestor/internal/enricher"
	"github.com/gosight/gosight/ingestor/internal/handler"
	"github.com/gosight/gosight/ingestor/internal/producer"
	"github.com/gosight/gosight/ingestor/internal/server"
	"github.com/gosight/gosight/ingestor/internal/validation"
	pb "github.com/gosight/gosight/ingestor/proto/gosight"
)

func main() {
	// Setup logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Load config
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config/ingestor.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	log.Info().Msg("Starting GoSight Ingestor...")

	// Initialize dependencies
	kafkaProducer, err := producer.NewKafkaProducer(cfg.Kafka)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Kafka producer")
	}
	defer kafkaProducer.Close()
	log.Info().Msg("Kafka producer initialized")

	validator, err := validation.NewValidator(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create validator")
	}
	defer validator.Close()
	log.Info().Msg("Validator initialized")

	eventEnricher := enricher.NewEnricher(cfg.GeoIP.DatabasePath)
	defer eventEnricher.Close()
	log.Info().Msg("Enricher initialized")

	// Create gRPC server
	grpcServer := grpc.NewServer()
	ingestServer := server.NewIngestServer(kafkaProducer, validator, eventEnricher)
	pb.RegisterIngestServiceServer(grpcServer, ingestServer)

	// Start gRPC server
	go func() {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Server.GRPCPort))
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to listen for gRPC")
		}
		log.Info().Int("port", cfg.Server.GRPCPort).Msg("Starting gRPC server")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatal().Err(err).Msg("Failed to serve gRPC")
		}
	}()

	// Create HTTP server (fallback)
	httpHandler := handler.NewHTTPHandler(kafkaProducer, validator, eventEnricher)
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(handler.CORSMiddleware)

	r.Get("/health", handler.HealthCheck)
	r.Post("/v1/events", httpHandler.HandleEvents)
	r.Post("/v1/replay", httpHandler.HandleReplay)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler: r,
	}

	go func() {
		log.Info().Int("port", cfg.Server.HTTPPort).Msg("Starting HTTP server")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to serve HTTP")
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down servers...")
	grpcServer.GracefulStop()
	httpServer.Shutdown(context.Background())
	log.Info().Msg("Servers stopped")
}
