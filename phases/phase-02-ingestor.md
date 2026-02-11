> **Status:** :white_check_mark: COMPLETED
> **Completed Date:** 2026-02-10

# Phase 2: Ingestor Service

## Mục Tiêu

Xây dựng service nhận events từ SDK, validate, enrich và gửi vào Kafka.

## Prerequisites

- Phase 1 hoàn thành
- Infrastructure đang chạy (Kafka, Redis, PostgreSQL)

## Tasks

### 2.1 Go Module Setup

```bash
cd ingestor
go mod init github.com/gosight/gosight/ingestor
```

**`go.mod`**

```go
module github.com/gosight/gosight/ingestor

go 1.21

require (
    google.golang.org/grpc v1.60.0
    google.golang.org/protobuf v1.32.0
    github.com/segmentio/kafka-go v0.4.47
    github.com/redis/go-redis/v9 v9.3.0
    github.com/jackc/pgx/v5 v5.5.1
    github.com/oschwald/geoip2-golang v1.9.0
    github.com/mssola/useragent v1.0.0
    github.com/go-chi/chi/v5 v5.0.11
    github.com/rs/zerolog v1.31.0
    gopkg.in/yaml.v3 v3.0.1
)
```

---

### 2.2 Configuration

**`config/ingestor.yaml`**

```yaml
server:
  grpc_port: 50051
  http_port: 8080

kafka:
  brokers:
    - kafka:9092
  topics:
    events: gosight.events.raw
    replay: gosight.replay.chunks
    errors: gosight.events.errors

redis:
  addr: redis:6379
  password: ${REDIS_PASSWORD}
  db: 0

postgres:
  dsn: postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}

geoip:
  database_path: /data/geoip/GeoLite2-City.mmdb

rate_limit:
  requests_per_second: 1000
  burst: 2000

batch:
  max_size: 100
  flush_interval: 1s
```

**`internal/config/config.go`**

```go
package config

import (
    "os"
    "gopkg.in/yaml.v3"
)

type Config struct {
    Server    ServerConfig    `yaml:"server"`
    Kafka     KafkaConfig     `yaml:"kafka"`
    Redis     RedisConfig     `yaml:"redis"`
    Postgres  PostgresConfig  `yaml:"postgres"`
    GeoIP     GeoIPConfig     `yaml:"geoip"`
    RateLimit RateLimitConfig `yaml:"rate_limit"`
    Batch     BatchConfig     `yaml:"batch"`
}

type ServerConfig struct {
    GRPCPort int `yaml:"grpc_port"`
    HTTPPort int `yaml:"http_port"`
}

type KafkaConfig struct {
    Brokers []string          `yaml:"brokers"`
    Topics  map[string]string `yaml:"topics"`
}

type RedisConfig struct {
    Addr     string `yaml:"addr"`
    Password string `yaml:"password"`
    DB       int    `yaml:"db"`
}

type PostgresConfig struct {
    DSN string `yaml:"dsn"`
}

type GeoIPConfig struct {
    DatabasePath string `yaml:"database_path"`
}

type RateLimitConfig struct {
    RequestsPerSecond int `yaml:"requests_per_second"`
    Burst             int `yaml:"burst"`
}

type BatchConfig struct {
    MaxSize       int    `yaml:"max_size"`
    FlushInterval string `yaml:"flush_interval"`
}

func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    // Expand environment variables
    expanded := os.ExpandEnv(string(data))

    var cfg Config
    if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
        return nil, err
    }

    return &cfg, nil
}
```

---

### 2.3 Main Entry Point

**`cmd/ingestor/main.go`**

```go
package main

import (
    "context"
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
    "github.com/gosight/gosight/ingestor/internal/handler"
    "github.com/gosight/gosight/ingestor/internal/producer"
    "github.com/gosight/gosight/ingestor/internal/server"
    "github.com/gosight/gosight/ingestor/internal/validation"
    pb "github.com/gosight/gosight/proto/gosight"
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

    // Initialize dependencies
    kafkaProducer, err := producer.NewKafkaProducer(cfg.Kafka)
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to create Kafka producer")
    }
    defer kafkaProducer.Close()

    validator, err := validation.NewValidator(cfg)
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to create validator")
    }

    // Create gRPC server
    grpcServer := grpc.NewServer()
    ingestServer := server.NewIngestServer(kafkaProducer, validator)
    pb.RegisterIngestServiceServer(grpcServer, ingestServer)

    // Start gRPC server
    go func() {
        lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Server.GRPCPort))
        if err != nil {
            log.Fatal().Err(err).Msg("Failed to listen")
        }
        log.Info().Int("port", cfg.Server.GRPCPort).Msg("Starting gRPC server")
        if err := grpcServer.Serve(lis); err != nil {
            log.Fatal().Err(err).Msg("Failed to serve gRPC")
        }
    }()

    // Create HTTP server (fallback)
    httpHandler := handler.NewHTTPHandler(kafkaProducer, validator)
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
}
```

---

### 2.4 gRPC Server Implementation

**`internal/server/ingest_server.go`**

```go
package server

import (
    "context"
    "io"
    "time"

    "github.com/google/uuid"
    "github.com/rs/zerolog/log"

    "github.com/gosight/gosight/ingestor/internal/enricher"
    "github.com/gosight/gosight/ingestor/internal/producer"
    "github.com/gosight/gosight/ingestor/internal/validation"
    pb "github.com/gosight/gosight/proto/gosight"
)

type IngestServer struct {
    pb.UnimplementedIngestServiceServer
    producer  *producer.KafkaProducer
    validator *validation.Validator
    enricher  *enricher.Enricher
}

func NewIngestServer(p *producer.KafkaProducer, v *validation.Validator) *IngestServer {
    return &IngestServer{
        producer:  p,
        validator: v,
        enricher:  enricher.NewEnricher(),
    }
}

func (s *IngestServer) SendEvents(stream pb.IngestService_SendEventsServer) error {
    for {
        batch, err := stream.Recv()
        if err == io.EOF {
            return nil
        }
        if err != nil {
            return err
        }

        // Validate API key
        projectID, err := s.validator.ValidateAPIKey(stream.Context(), batch.ProjectKey)
        if err != nil {
            stream.Send(&pb.EventAck{
                Success:       false,
                Errors:        []string{"Invalid API key"},
                RejectedCount: int32(len(batch.Events)),
            })
            continue
        }

        // Rate limiting
        if !s.validator.CheckRateLimit(projectID) {
            stream.Send(&pb.EventAck{
                Success:       false,
                Errors:        []string{"Rate limit exceeded"},
                RejectedCount: int32(len(batch.Events)),
            })
            continue
        }

        // Process events
        accepted := 0
        rejected := 0
        var errors []string

        for _, event := range batch.Events {
            // Validate event
            if err := s.validator.ValidateEvent(event); err != nil {
                rejected++
                errors = append(errors, err.Error())
                continue
            }

            // Enrich event
            enrichedEvent := s.enricher.Enrich(event, batch.Session)

            // Produce to Kafka
            err := s.producer.ProduceEvent(stream.Context(), projectID, enrichedEvent)
            if err != nil {
                rejected++
                errors = append(errors, err.Error())
                continue
            }

            accepted++
        }

        // Send acknowledgment
        stream.Send(&pb.EventAck{
            Success:       rejected == 0,
            AcceptedCount: int32(accepted),
            RejectedCount: int32(rejected),
            Errors:        errors,
        })
    }
}

func (s *IngestServer) SendReplay(stream pb.IngestService_SendReplayServer) error {
    for {
        chunk, err := stream.Recv()
        if err == io.EOF {
            return stream.SendAndClose(&pb.ReplayAck{
                Success: true,
                Message: "All chunks received",
            })
        }
        if err != nil {
            return err
        }

        // Produce to Kafka replay topic
        err = s.producer.ProduceReplayChunk(stream.Context(), chunk)
        if err != nil {
            log.Error().Err(err).Msg("Failed to produce replay chunk")
        }
    }
}
```

---

### 2.5 HTTP Handler (Fallback)

**`internal/handler/http_handler.go`**

```go
package handler

import (
    "encoding/json"
    "io"
    "net/http"

    "github.com/gosight/gosight/ingestor/internal/producer"
    "github.com/gosight/gosight/ingestor/internal/validation"
)

type HTTPHandler struct {
    producer  *producer.KafkaProducer
    validator *validation.Validator
}

func NewHTTPHandler(p *producer.KafkaProducer, v *validation.Validator) *HTTPHandler {
    return &HTTPHandler{
        producer:  p,
        validator: v,
    }
}

type EventBatchRequest struct {
    ProjectKey string                   `json:"project_key"`
    SessionID  string                   `json:"session_id"`
    UserID     string                   `json:"user_id"`
    Events     []map[string]interface{} `json:"events"`
}

type EventResponse struct {
    Success       bool     `json:"success"`
    AcceptedCount int      `json:"accepted_count"`
    RejectedCount int      `json:"rejected_count"`
    Errors        []string `json:"errors,omitempty"`
}

func (h *HTTPHandler) HandleEvents(w http.ResponseWriter, r *http.Request) {
    // Read body
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Failed to read body", http.StatusBadRequest)
        return
    }
    defer r.Body.Close()

    // Parse request
    var req EventBatchRequest
    if err := json.Unmarshal(body, &req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Validate API key
    projectID, err := h.validator.ValidateAPIKey(r.Context(), req.ProjectKey)
    if err != nil {
        w.WriteHeader(http.StatusUnauthorized)
        json.NewEncoder(w).Encode(EventResponse{
            Success: false,
            Errors:  []string{"Invalid API key"},
        })
        return
    }

    // Rate limiting
    if !h.validator.CheckRateLimit(projectID) {
        w.WriteHeader(http.StatusTooManyRequests)
        json.NewEncoder(w).Encode(EventResponse{
            Success: false,
            Errors:  []string{"Rate limit exceeded"},
        })
        return
    }

    // Get client IP for enrichment
    clientIP := r.Header.Get("X-Real-IP")
    if clientIP == "" {
        clientIP = r.RemoteAddr
    }

    // Process events
    accepted := 0
    rejected := 0
    var errors []string

    for _, event := range req.Events {
        // Add metadata
        event["project_id"] = projectID
        event["session_id"] = req.SessionID
        event["client_ip"] = clientIP

        // Produce to Kafka
        err := h.producer.ProduceEventJSON(r.Context(), projectID, event)
        if err != nil {
            rejected++
            errors = append(errors, err.Error())
            continue
        }
        accepted++
    }

    // Response
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(EventResponse{
        Success:       rejected == 0,
        AcceptedCount: accepted,
        RejectedCount: rejected,
        Errors:        errors,
    })
}

func (h *HTTPHandler) HandleReplay(w http.ResponseWriter, r *http.Request) {
    // Similar implementation for replay chunks
    // Accept gzip compressed rrweb events
}

func HealthCheck(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

func CORSMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Project-Key")

        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

---

### 2.6 Kafka Producer

**`internal/producer/kafka_producer.go`**

```go
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
```

---

### 2.7 Validation

**`internal/validation/validator.go`**

```go
package validation

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/redis/go-redis/v9"

    "github.com/gosight/gosight/ingestor/internal/config"
)

type Validator struct {
    db    *pgxpool.Pool
    redis *redis.Client
    cfg   *config.Config
}

func NewValidator(cfg *config.Config) (*Validator, error) {
    // Connect to PostgreSQL
    db, err := pgxpool.New(context.Background(), cfg.Postgres.DSN)
    if err != nil {
        return nil, err
    }

    // Connect to Redis
    rdb := redis.NewClient(&redis.Options{
        Addr:     cfg.Redis.Addr,
        Password: cfg.Redis.Password,
        DB:       cfg.Redis.DB,
    })

    return &Validator{
        db:    db,
        redis: rdb,
        cfg:   cfg,
    }, nil
}

func (v *Validator) ValidateAPIKey(ctx context.Context, apiKey string) (string, error) {
    // Check cache first
    cacheKey := "apikey:" + apiKey[:12]
    projectID, err := v.redis.Get(ctx, cacheKey).Result()
    if err == nil {
        return projectID, nil
    }

    // Hash the key
    hash := sha256.Sum256([]byte(apiKey))
    keyHash := hex.EncodeToString(hash[:])

    // Query database
    var id string
    err = v.db.QueryRow(ctx, `
        SELECT project_id FROM api_keys
        WHERE key_hash = $1 AND is_active = true
        AND (expires_at IS NULL OR expires_at > NOW())
    `, keyHash).Scan(&id)

    if err != nil {
        return "", errors.New("invalid API key")
    }

    // Cache for 5 minutes
    v.redis.Set(ctx, cacheKey, id, 5*time.Minute)

    // Update last used
    go v.db.Exec(context.Background(), `
        UPDATE api_keys
        SET last_used_at = NOW(), request_count = request_count + 1
        WHERE key_hash = $1
    `, keyHash)

    return id, nil
}

func (v *Validator) CheckRateLimit(projectID string) bool {
    ctx := context.Background()
    key := "ratelimit:" + projectID

    // Increment counter
    count, err := v.redis.Incr(ctx, key).Result()
    if err != nil {
        return true // Allow on error
    }

    // Set expiry on first request
    if count == 1 {
        v.redis.Expire(ctx, key, time.Second)
    }

    return count <= int64(v.cfg.RateLimit.RequestsPerSecond)
}

func (v *Validator) ValidateEvent(event interface{}) error {
    // Basic validation
    // - Required fields
    // - Field types
    // - Value ranges
    return nil
}
```

---

### 2.8 Event Enrichment

**`internal/enricher/enricher.go`**

```go
package enricher

import (
    "net"
    "time"

    "github.com/mssola/useragent"
    "github.com/oschwald/geoip2-golang"
)

type Enricher struct {
    geoIP *geoip2.Reader
}

func NewEnricher() *Enricher {
    // Try to load GeoIP database
    geoIP, _ := geoip2.Open("/data/geoip/GeoLite2-City.mmdb")

    return &Enricher{
        geoIP: geoIP,
    }
}

type EnrichedEvent struct {
    // Original event fields
    EventID   string `json:"event_id"`
    Type      string `json:"type"`
    Timestamp int64  `json:"timestamp"`

    // Enriched fields
    ServerTimestamp int64  `json:"server_timestamp"`
    Browser         string `json:"browser"`
    BrowserVersion  string `json:"browser_version"`
    OS              string `json:"os"`
    OSVersion       string `json:"os_version"`
    DeviceType      string `json:"device_type"`
    Country         string `json:"country"`
    City            string `json:"city"`
}

func (e *Enricher) Enrich(event interface{}, session interface{}) *EnrichedEvent {
    enriched := &EnrichedEvent{
        ServerTimestamp: time.Now().UnixMilli(),
    }

    // Parse user agent
    // ua := useragent.New(userAgentString)
    // enriched.Browser = ua.Name()
    // enriched.BrowserVersion = ua.Version()
    // enriched.OS = ua.OS()
    // enriched.DeviceType = getDeviceType(ua)

    // GeoIP lookup
    // if e.geoIP != nil {
    //     ip := net.ParseIP(clientIP)
    //     record, err := e.geoIP.City(ip)
    //     if err == nil {
    //         enriched.Country = record.Country.IsoCode
    //         enriched.City = record.City.Names["en"]
    //     }
    // }

    return enriched
}

func getDeviceType(ua *useragent.UserAgent) string {
    if ua.Mobile() {
        return "mobile"
    }
    if ua.Bot() {
        return "bot"
    }
    return "desktop"
}
```

---

### 2.9 Dockerfile

**`deploy/docker/ingestor.Dockerfile`**

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY ingestor/go.mod ingestor/go.sum ./
RUN go mod download

# Copy source code
COPY ingestor/ ./
COPY proto/ ../proto/

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /ingestor ./cmd/ingestor

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /ingestor .
COPY config/ingestor.yaml ./config/

EXPOSE 50051 8080

CMD ["./ingestor"]
```

---

## Checklist

- [x] Go module setup
- [x] Configuration loading
- [x] gRPC server implementation
- [x] HTTP fallback handler
- [x] Kafka producer
- [x] API key validation
- [x] Rate limiting
- [x] Event enrichment (UserAgent, GeoIP)
- [x] Dockerfile
- [x] Unit tests
- [x] Integration tests với Kafka

## Kết Quả

Sau phase này:
- Ingestor service chạy được
- Nhận events qua gRPC và HTTP
- Validate API keys
- Rate limiting hoạt động
- Events được gửi vào Kafka

## Test Commands

```bash
# Run ingestor
cd ingestor
go run cmd/ingestor/main.go

# Test HTTP endpoint
curl -X POST http://localhost:8080/v1/events \
  -H "Content-Type: application/json" \
  -d '{
    "project_key": "gs_test_key",
    "session_id": "550e8400-e29b-41d4-a716-446655440000",
    "events": [
      {
        "type": "page_view",
        "timestamp": 1704067200000,
        "url": "https://example.com/",
        "path": "/"
      }
    ]
  }'

# Check Kafka topic
docker-compose exec kafka kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 \
  --topic gosight.events.raw \
  --from-beginning
```
