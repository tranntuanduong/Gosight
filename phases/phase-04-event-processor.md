# Phase 4: Event Processor

## Mục Tiêu

Xây dựng Kafka consumer để xử lý events từ Ingestor và lưu vào ClickHouse.

## Prerequisites

- Phase 1 hoàn thành (Infrastructure)
- Phase 2 hoàn thành (Ingestor đang gửi events vào Kafka)

## Tasks

### 4.1 Project Structure

```
processor/
├── cmd/
│   └── event-processor/
│       └── main.go
├── internal/
│   ├── consumer/
│   │   └── kafka.go
│   ├── storage/
│   │   └── clickhouse.go
│   ├── transformer/
│   │   └── event.go
│   └── session/
│       └── aggregator.go
├── go.mod
└── config/
    └── processor.yaml
```

---

### 4.2 Configuration

**`config/processor.yaml`**

```yaml
kafka:
  brokers:
    - kafka:9092
  topics:
    events: gosight.events.raw
  consumer_group: gosight-event-processor
  batch_size: 1000
  batch_timeout: 5s

clickhouse:
  dsn: clickhouse://default:password@clickhouse:9000/gosight
  max_open_conns: 10
  max_idle_conns: 5

redis:
  addr: redis:6379
  password: ${REDIS_PASSWORD}

batch:
  size: 1000
  flush_interval: 5s
```

---

### 4.3 Main Entry Point

**`cmd/event-processor/main.go`**

```go
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
        log.Fatal().Err(err).Msg("Failed to load config")
    }

    // Initialize ClickHouse
    ch, err := storage.NewClickHouse(cfg.ClickHouse)
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to connect to ClickHouse")
    }
    defer ch.Close()

    // Initialize session aggregator
    sessionAgg := session.NewAggregator(ch, cfg.Redis)

    // Create event processor
    processor := NewEventProcessor(ch, sessionAgg, cfg.Batch)

    // Create Kafka consumer
    kafkaConsumer, err := consumer.NewKafkaConsumer(cfg.Kafka, processor)
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
    processor.Flush()
}
```

---

### 4.4 Kafka Consumer

**`internal/consumer/kafka.go`**

```go
package consumer

import (
    "context"
    "encoding/json"

    "github.com/rs/zerolog/log"
    "github.com/segmentio/kafka-go"

    "github.com/gosight/gosight/processor/internal/config"
)

type MessageProcessor interface {
    Process(ctx context.Context, event map[string]interface{}) error
}

type KafkaConsumer struct {
    reader    *kafka.Reader
    processor MessageProcessor
}

func NewKafkaConsumer(cfg config.KafkaConfig, processor MessageProcessor) (*KafkaConsumer, error) {
    reader := kafka.NewReader(kafka.ReaderConfig{
        Brokers:        cfg.Brokers,
        Topic:          cfg.Topics["events"],
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

func (c *KafkaConsumer) Start(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
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
                log.Error().Err(err).Msg("Failed to parse message")
                c.reader.CommitMessages(ctx, msg)
                continue
            }

            // Process event
            if err := c.processor.Process(ctx, event); err != nil {
                log.Error().Err(err).Msg("Failed to process event")
            }

            // Commit
            if err := c.reader.CommitMessages(ctx, msg); err != nil {
                log.Error().Err(err).Msg("Failed to commit message")
            }
        }
    }
}

func (c *KafkaConsumer) Close() error {
    return c.reader.Close()
}
```

---

### 4.5 Event Processor

**`internal/processor.go`**

```go
package main

import (
    "context"
    "sync"
    "time"

    "github.com/rs/zerolog/log"

    "github.com/gosight/gosight/processor/internal/config"
    "github.com/gosight/gosight/processor/internal/session"
    "github.com/gosight/gosight/processor/internal/storage"
    "github.com/gosight/gosight/processor/internal/transformer"
)

type EventProcessor struct {
    ch         *storage.ClickHouse
    sessionAgg *session.Aggregator
    batchCfg   config.BatchConfig

    buffer    []storage.EventRow
    mu        sync.Mutex
    lastFlush time.Time
    ticker    *time.Ticker
}

func NewEventProcessor(ch *storage.ClickHouse, sessionAgg *session.Aggregator, batchCfg config.BatchConfig) *EventProcessor {
    p := &EventProcessor{
        ch:         ch,
        sessionAgg: sessionAgg,
        batchCfg:   batchCfg,
        buffer:     make([]storage.EventRow, 0, batchCfg.Size),
        lastFlush:  time.Now(),
    }

    // Start flush ticker
    p.ticker = time.NewTicker(batchCfg.FlushInterval)
    go p.flushLoop()

    return p
}

func (p *EventProcessor) Process(ctx context.Context, event map[string]interface{}) error {
    // Transform to ClickHouse row
    row, err := transformer.TransformEvent(event)
    if err != nil {
        return err
    }

    // Add to buffer
    p.mu.Lock()
    p.buffer = append(p.buffer, row)
    shouldFlush := len(p.buffer) >= p.batchCfg.Size
    p.mu.Unlock()

    // Update session aggregation
    go p.sessionAgg.UpdateSession(ctx, row)

    // Flush if buffer full
    if shouldFlush {
        p.Flush()
    }

    return nil
}

func (p *EventProcessor) flushLoop() {
    for range p.ticker.C {
        p.Flush()
    }
}

func (p *EventProcessor) Flush() {
    p.mu.Lock()
    if len(p.buffer) == 0 {
        p.mu.Unlock()
        return
    }

    events := p.buffer
    p.buffer = make([]storage.EventRow, 0, p.batchCfg.Size)
    p.lastFlush = time.Now()
    p.mu.Unlock()

    // Insert to ClickHouse
    start := time.Now()
    if err := p.ch.InsertEvents(context.Background(), events); err != nil {
        log.Error().Err(err).Int("count", len(events)).Msg("Failed to insert events")
        return
    }

    log.Info().
        Int("count", len(events)).
        Dur("duration", time.Since(start)).
        Msg("Flushed events to ClickHouse")
}
```

---

### 4.6 ClickHouse Storage

**`internal/storage/clickhouse.go`**

```go
package storage

import (
    "context"
    "database/sql"
    "time"

    "github.com/ClickHouse/clickhouse-go/v2"
    "github.com/google/uuid"

    "github.com/gosight/gosight/processor/internal/config"
)

type ClickHouse struct {
    conn clickhouse.Conn
}

type EventRow struct {
    EventID        uuid.UUID
    ProjectID      string
    SessionID      uuid.UUID
    UserID         string
    EventType      string
    Timestamp      time.Time
    URL            string
    Path           string
    Title          string
    Referrer       string
    ClickX         *int16
    ClickY         *int16
    TargetSelector *string
    TargetTag      *string
    TargetText     *string
    ScrollDepth    *uint8
    ErrorMessage   *string
    ErrorStack     *string
    ErrorType      *string
    LCP            *uint16
    FID            *uint16
    CLS            *float32
    TTFB           *uint16
    Payload        string
    Browser        string
    BrowserVersion string
    OS             string
    OSVersion      string
    DeviceType     string
    ScreenWidth    uint16
    ScreenHeight   uint16
    ViewportWidth  uint16
    ViewportHeight uint16
    IP             string
    Country        string
    City           string
    Custom         string
}

func NewClickHouse(cfg config.ClickHouseConfig) (*ClickHouse, error) {
    conn, err := clickhouse.Open(&clickhouse.Options{
        Addr: []string{cfg.Addr},
        Auth: clickhouse.Auth{
            Database: cfg.Database,
            Username: cfg.Username,
            Password: cfg.Password,
        },
        MaxOpenConns: cfg.MaxOpenConns,
        MaxIdleConns: cfg.MaxIdleConns,
    })
    if err != nil {
        return nil, err
    }

    return &ClickHouse{conn: conn}, nil
}

func (c *ClickHouse) InsertEvents(ctx context.Context, events []EventRow) error {
    batch, err := c.conn.PrepareBatch(ctx, `
        INSERT INTO events (
            event_id, project_id, session_id, user_id, event_type, timestamp,
            url, path, title, referrer,
            click_x, click_y, target_selector, target_tag, target_text,
            scroll_depth,
            error_message, error_stack, error_type,
            lcp, fid, cls, ttfb,
            payload,
            browser, browser_version, os, os_version, device_type,
            screen_width, screen_height, viewport_width, viewport_height,
            ip, country, city,
            custom
        )
    `)
    if err != nil {
        return err
    }

    for _, e := range events {
        err := batch.Append(
            e.EventID, e.ProjectID, e.SessionID, e.UserID, e.EventType, e.Timestamp,
            e.URL, e.Path, e.Title, e.Referrer,
            e.ClickX, e.ClickY, e.TargetSelector, e.TargetTag, e.TargetText,
            e.ScrollDepth,
            e.ErrorMessage, e.ErrorStack, e.ErrorType,
            e.LCP, e.FID, e.CLS, e.TTFB,
            e.Payload,
            e.Browser, e.BrowserVersion, e.OS, e.OSVersion, e.DeviceType,
            e.ScreenWidth, e.ScreenHeight, e.ViewportWidth, e.ViewportHeight,
            e.IP, e.Country, e.City,
            e.Custom,
        )
        if err != nil {
            return err
        }
    }

    return batch.Send()
}

func (c *ClickHouse) Close() error {
    return c.conn.Close()
}
```

---

### 4.7 Event Transformer

**`internal/transformer/event.go`**

```go
package transformer

import (
    "encoding/json"
    "time"

    "github.com/google/uuid"

    "github.com/gosight/gosight/processor/internal/storage"
)

func TransformEvent(raw map[string]interface{}) (storage.EventRow, error) {
    row := storage.EventRow{}

    // Required fields
    row.EventID = parseUUID(raw["event_id"])
    row.ProjectID = getString(raw, "project_id")
    row.SessionID = parseUUID(raw["session_id"])
    row.UserID = getString(raw, "user_id")
    row.EventType = getString(raw, "type")
    row.Timestamp = parseTimestamp(raw["timestamp"])
    row.URL = getString(raw, "url")
    row.Path = getString(raw, "path")
    row.Title = getString(raw, "title")
    row.Referrer = getString(raw, "referrer")

    // Payload based on event type
    if payload, ok := raw["payload"].(map[string]interface{}); ok {
        switch row.EventType {
        case "click":
            row.ClickX = getInt16Ptr(payload, "x")
            row.ClickY = getInt16Ptr(payload, "y")
            if target, ok := payload["target"].(map[string]interface{}); ok {
                row.TargetSelector = getStringPtr(target, "selector")
                row.TargetTag = getStringPtr(target, "tag")
                row.TargetText = getStringPtr(target, "text")
            }

        case "scroll":
            row.ScrollDepth = getUint8Ptr(payload, "depth_percent")

        case "js_error":
            row.ErrorMessage = getStringPtr(payload, "message")
            row.ErrorStack = getStringPtr(payload, "stack")
            row.ErrorType = getStringPtr(payload, "errorType")

        case "web_vitals":
            row.LCP = getUint16Ptr(payload, "lcp")
            row.FID = getUint16Ptr(payload, "fid")
            row.CLS = getFloat32Ptr(payload, "cls")
            row.TTFB = getUint16Ptr(payload, "ttfb")
        }

        // Store full payload as JSON
        payloadBytes, _ := json.Marshal(payload)
        row.Payload = string(payloadBytes)
    }

    // Device info
    if device, ok := raw["device"].(map[string]interface{}); ok {
        row.Browser = getString(device, "browser")
        row.BrowserVersion = getString(device, "browser_version")
        row.OS = getString(device, "os")
        row.OSVersion = getString(device, "os_version")
        row.DeviceType = getString(device, "device_type")
        row.ScreenWidth = getUint16(device, "screen_width")
        row.ScreenHeight = getUint16(device, "screen_height")
        row.ViewportWidth = getUint16(device, "viewport_width")
        row.ViewportHeight = getUint16(device, "viewport_height")
    }

    // Geo info (enriched by ingestor)
    row.IP = getString(raw, "client_ip")
    row.Country = getString(raw, "country")
    row.City = getString(raw, "city")

    // Custom properties
    if custom, ok := raw["custom"].(map[string]interface{}); ok {
        customBytes, _ := json.Marshal(custom)
        row.Custom = string(customBytes)
    }

    return row, nil
}

func parseUUID(v interface{}) uuid.UUID {
    if s, ok := v.(string); ok {
        if id, err := uuid.Parse(s); err == nil {
            return id
        }
    }
    return uuid.New()
}

func parseTimestamp(v interface{}) time.Time {
    switch t := v.(type) {
    case float64:
        return time.UnixMilli(int64(t))
    case int64:
        return time.UnixMilli(t)
    default:
        return time.Now()
    }
}

func getString(m map[string]interface{}, key string) string {
    if v, ok := m[key].(string); ok {
        return v
    }
    return ""
}

func getStringPtr(m map[string]interface{}, key string) *string {
    if v, ok := m[key].(string); ok {
        return &v
    }
    return nil
}

func getInt16Ptr(m map[string]interface{}, key string) *int16 {
    if v, ok := m[key].(float64); ok {
        i := int16(v)
        return &i
    }
    return nil
}

func getUint8Ptr(m map[string]interface{}, key string) *uint8 {
    if v, ok := m[key].(float64); ok {
        i := uint8(v)
        return &i
    }
    return nil
}

func getUint16(m map[string]interface{}, key string) uint16 {
    if v, ok := m[key].(float64); ok {
        return uint16(v)
    }
    return 0
}

func getUint16Ptr(m map[string]interface{}, key string) *uint16 {
    if v, ok := m[key].(float64); ok {
        i := uint16(v)
        return &i
    }
    return nil
}

func getFloat32Ptr(m map[string]interface{}, key string) *float32 {
    if v, ok := m[key].(float64); ok {
        f := float32(v)
        return &f
    }
    return nil
}
```

---

### 4.8 Session Aggregator

**`internal/session/aggregator.go`**

```go
package session

import (
    "context"
    "time"

    "github.com/google/uuid"
    "github.com/redis/go-redis/v9"

    "github.com/gosight/gosight/processor/internal/storage"
)

type Aggregator struct {
    ch    *storage.ClickHouse
    redis *redis.Client
}

type SessionData struct {
    SessionID      uuid.UUID
    ProjectID      string
    UserID         string
    StartedAt      time.Time
    EndedAt        time.Time
    EntryURL       string
    EntryPath      string
    ExitURL        string
    ExitPath       string
    PageCount      uint16
    EventCount     uint32
    ClickCount     uint16
    ErrorCount     uint16
    MaxScrollDepth uint8
    HasError       bool
    HasRageClick   bool
    HasDeadClick   bool
    IsBounce       bool
    Browser        string
    OS             string
    DeviceType     string
    Country        string
}

func NewAggregator(ch *storage.ClickHouse, redisCfg config.RedisConfig) *Aggregator {
    rdb := redis.NewClient(&redis.Options{
        Addr:     redisCfg.Addr,
        Password: redisCfg.Password,
        DB:       redisCfg.DB,
    })

    return &Aggregator{
        ch:    ch,
        redis: rdb,
    }
}

func (a *Aggregator) UpdateSession(ctx context.Context, event storage.EventRow) error {
    key := "session:" + event.SessionID.String()

    // Use Redis to track session state
    pipe := a.redis.Pipeline()

    // Update session end time
    pipe.HSet(ctx, key, "ended_at", event.Timestamp.UnixMilli())

    // Increment event count
    pipe.HIncrBy(ctx, key, "event_count", 1)

    // Track based on event type
    switch event.EventType {
    case "page_view":
        pipe.HIncrBy(ctx, key, "page_count", 1)
        pipe.HSetNX(ctx, key, "entry_url", event.URL)
        pipe.HSetNX(ctx, key, "entry_path", event.Path)
        pipe.HSet(ctx, key, "exit_url", event.URL)
        pipe.HSet(ctx, key, "exit_path", event.Path)

    case "click":
        pipe.HIncrBy(ctx, key, "click_count", 1)

    case "js_error":
        pipe.HIncrBy(ctx, key, "error_count", 1)
        pipe.HSet(ctx, key, "has_error", 1)

    case "scroll":
        if event.ScrollDepth != nil {
            // Update max scroll depth
            pipe.Eval(ctx, `
                local current = redis.call('HGET', KEYS[1], 'max_scroll_depth')
                if not current or tonumber(ARGV[1]) > tonumber(current) then
                    redis.call('HSET', KEYS[1], 'max_scroll_depth', ARGV[1])
                end
            `, []string{key}, *event.ScrollDepth)
        }
    }

    // Set session metadata (only if not exists)
    pipe.HSetNX(ctx, key, "project_id", event.ProjectID)
    pipe.HSetNX(ctx, key, "user_id", event.UserID)
    pipe.HSetNX(ctx, key, "started_at", event.Timestamp.UnixMilli())
    pipe.HSetNX(ctx, key, "browser", event.Browser)
    pipe.HSetNX(ctx, key, "os", event.OS)
    pipe.HSetNX(ctx, key, "device_type", event.DeviceType)
    pipe.HSetNX(ctx, key, "country", event.Country)

    // Set TTL (1 hour)
    pipe.Expire(ctx, key, time.Hour)

    _, err := pipe.Exec(ctx)
    return err
}

// FlushSession writes session data to ClickHouse
// Called periodically or when session ends
func (a *Aggregator) FlushSession(ctx context.Context, sessionID uuid.UUID) error {
    key := "session:" + sessionID.String()

    // Get all session data from Redis
    data, err := a.redis.HGetAll(ctx, key).Result()
    if err != nil {
        return err
    }

    if len(data) == 0 {
        return nil
    }

    // Convert to SessionData and insert to ClickHouse
    session := a.parseSessionData(sessionID, data)

    return a.ch.UpsertSession(ctx, session)
}
```

---

### 4.9 Dockerfile

**`deploy/docker/event-processor.Dockerfile`**

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY processor/go.mod processor/go.sum ./
RUN go mod download

COPY processor/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /event-processor ./cmd/event-processor

FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /event-processor .
COPY config/processor.yaml ./config/

CMD ["./event-processor"]
```

---

## Checklist

- [ ] Kafka consumer setup
- [ ] Event processor với batching
- [ ] ClickHouse storage client
- [ ] Event transformer
- [ ] Session aggregator (Redis)
- [ ] Upsert sessions vào ClickHouse
- [ ] Dockerfile
- [ ] Unit tests
- [ ] Integration tests

## Kết Quả

Sau phase này:
- Events từ Kafka được xử lý
- Data được lưu vào ClickHouse
- Sessions được aggregate real-time
- Batch inserts tối ưu performance

## Test Commands

```bash
# Run processor
cd processor
go run cmd/event-processor/main.go

# Check ClickHouse data
docker-compose exec clickhouse clickhouse-client \
  --query "SELECT count() FROM gosight.events"

# Check sessions
docker-compose exec clickhouse clickhouse-client \
  --query "SELECT * FROM gosight.sessions LIMIT 10"

# Check Kafka consumer lag
docker-compose exec kafka kafka-consumer-groups.sh \
  --bootstrap-server localhost:9092 \
  --group gosight-event-processor \
  --describe
```
