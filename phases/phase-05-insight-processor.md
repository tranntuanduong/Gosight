# Phase 5: Insight Processor

## Mục Tiêu

Xây dựng service phát hiện UX insights từ event stream.

## Prerequisites

- Phase 4 hoàn thành (Events đang được xử lý)

## Tasks

### 5.1 Project Structure

```
processor/
├── cmd/
│   └── insight-processor/
│       └── main.go
└── internal/
    └── insights/
        ├── processor.go
        ├── rage_click.go
        ├── dead_click.go
        ├── error_click.go
        ├── thrashed_cursor.go
        ├── u_turn.go
        └── slow_page.go
```

---

### 5.2 Insight Processor Main

**`cmd/insight-processor/main.go`**

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/rs/zerolog/log"

    "github.com/gosight/gosight/processor/internal/config"
    "github.com/gosight/gosight/processor/internal/consumer"
    "github.com/gosight/gosight/processor/internal/insights"
    "github.com/gosight/gosight/processor/internal/storage"
)

func main() {
    cfg, err := config.Load(os.Getenv("CONFIG_PATH"))
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to load config")
    }

    // Initialize storage
    ch, _ := storage.NewClickHouse(cfg.ClickHouse)
    defer ch.Close()

    redis := storage.NewRedis(cfg.Redis)
    defer redis.Close()

    // Initialize insight detectors
    insightProcessor := insights.NewProcessor(ch, redis, cfg.Insights)

    // Create Kafka consumer
    kafkaConsumer, _ := consumer.NewKafkaConsumer(consumer.Config{
        Brokers:       cfg.Kafka.Brokers,
        Topic:         cfg.Kafka.Topics["events"],
        ConsumerGroup: "gosight-insight-processor",
    }, insightProcessor)

    // Start
    ctx, cancel := context.WithCancel(context.Background())
    go kafkaConsumer.Start(ctx)

    log.Info().Msg("Insight processor started")

    // Shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    cancel()
    kafkaConsumer.Close()
}
```

---

### 5.3 Insight Processor

**`internal/insights/processor.go`**

```go
package insights

import (
    "context"
    "encoding/json"

    "github.com/google/uuid"

    "github.com/gosight/gosight/processor/internal/storage"
)

type Processor struct {
    rageClick      *RageClickDetector
    deadClick      *DeadClickDetector
    errorClick     *ErrorClickDetector
    thrashedCursor *ThrashedCursorDetector
    uTurn          *UTurnDetector
    slowPage       *SlowPageDetector

    ch    *storage.ClickHouse
    redis *storage.Redis
}

func NewProcessor(ch *storage.ClickHouse, redis *storage.Redis, cfg Config) *Processor {
    return &Processor{
        rageClick:      NewRageClickDetector(redis, cfg.RageClick),
        deadClick:      NewDeadClickDetector(cfg.DeadClick),
        errorClick:     NewErrorClickDetector(cfg.ErrorClick),
        thrashedCursor: NewThrashedCursorDetector(cfg.ThrashedCursor),
        uTurn:          NewUTurnDetector(cfg.UTurn),
        slowPage:       NewSlowPageDetector(cfg.SlowPage),
        ch:             ch,
        redis:          redis,
    }
}

func (p *Processor) Process(ctx context.Context, raw map[string]interface{}) error {
    event := p.parseEvent(raw)

    var insights []Insight

    switch event.Type {
    case "click":
        // Rage click detection
        if insight := p.rageClick.ProcessClick(event.SessionID, event); insight != nil {
            insights = append(insights, insight)
        }

        // Dead click detection
        p.deadClick.ProcessClick(event.SessionID, event)

        // Error click tracking
        p.errorClick.ProcessClick(event.SessionID, event)

    case "js_error":
        // Error click detection
        if insight := p.errorClick.ProcessError(event.SessionID, event); insight != nil {
            insights = append(insights, insight)
        }

    case "mouse_move":
        // Thrashed cursor detection
        if insight := p.thrashedCursor.ProcessMouseMove(event.SessionID, event); insight != nil {
            insights = append(insights, insight)
        }

    case "page_view":
        // U-turn detection
        if insight := p.uTurn.ProcessPageView(event.SessionID, event); insight != nil {
            insights = append(insights, insight)
        }

        // Resolve pending dead clicks
        p.deadClick.ProcessEvent(event.SessionID, event)

    case "dom_mutation":
        // Resolve pending dead clicks
        p.deadClick.ProcessEvent(event.SessionID, event)

    case "web_vitals", "page_load":
        // Slow page detection
        if insight := p.slowPage.ProcessPerformance(event); insight != nil {
            insights = append(insights, insight)
        }
    }

    // Store insights
    for _, insight := range insights {
        if err := p.storeInsight(ctx, insight); err != nil {
            return err
        }

        // Publish to alerts topic
        p.publishAlert(ctx, insight)
    }

    return nil
}

func (p *Processor) storeInsight(ctx context.Context, insight Insight) error {
    return p.ch.InsertInsight(ctx, storage.InsightRow{
        InsightID:       uuid.New(),
        ProjectID:       insight.ProjectID,
        SessionID:       insight.SessionID,
        InsightType:     insight.Type,
        Timestamp:       insight.Timestamp,
        URL:             insight.URL,
        Path:            insight.Path,
        X:               insight.X,
        Y:               insight.Y,
        TargetSelector:  insight.TargetSelector,
        Details:         insight.Details,
        RelatedEventIDs: insight.RelatedEventIDs,
    })
}

func (p *Processor) publishAlert(ctx context.Context, insight Insight) {
    // Publish to Kafka alerts topic for alert processor
}
```

---

### 5.4 Rage Click Detector

**`internal/insights/rage_click.go`**

```go
package insights

import (
    "context"
    "fmt"
    "math"
    "time"

    "github.com/google/uuid"
    "github.com/redis/go-redis/v9"
)

type RageClickDetector struct {
    redis        *redis.Client
    minClicks    int
    timeWindowMs int64
    radiusPx     int
}

type ClickRecord struct {
    X         int
    Y         int
    Timestamp int64
    EventID   string
}

func NewRageClickDetector(rdb *redis.Client, cfg RageClickConfig) *RageClickDetector {
    return &RageClickDetector{
        redis:        rdb,
        minClicks:    cfg.MinClicks,     // Default: 5
        timeWindowMs: cfg.TimeWindowMs,  // Default: 2000
        radiusPx:     cfg.RadiusPx,      // Default: 50
    }
}

func (d *RageClickDetector) ProcessClick(sessionID uuid.UUID, event Event) *Insight {
    ctx := context.Background()

    // Get click coordinates
    x := event.ClickX
    y := event.ClickY
    if x == 0 && y == 0 {
        return nil
    }

    // Grid cell for spatial grouping
    gridX := x / d.radiusPx
    gridY := y / d.radiusPx

    key := fmt.Sprintf("clicks:%s:%d:%d", sessionID.String(), gridX, gridY)

    // Add click to Redis sorted set (score = timestamp)
    d.redis.ZAdd(ctx, key, redis.Z{
        Score:  float64(event.Timestamp),
        Member: fmt.Sprintf("%d:%d:%s", x, y, event.EventID),
    })

    // Set expiry
    d.redis.Expire(ctx, key, time.Duration(d.timeWindowMs*2)*time.Millisecond)

    // Remove old clicks outside time window
    cutoff := event.Timestamp - d.timeWindowMs
    d.redis.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", cutoff))

    // Get remaining clicks
    clicks, err := d.redis.ZRange(ctx, key, 0, -1).Result()
    if err != nil || len(clicks) < d.minClicks {
        return nil
    }

    // Parse clicks
    var records []ClickRecord
    for _, c := range clicks {
        var cx, cy int
        var eid string
        fmt.Sscanf(c, "%d:%d:%s", &cx, &cy, &eid)
        records = append(records, ClickRecord{X: cx, Y: cy, EventID: eid})
    }

    // Calculate center
    centerX, centerY := d.calculateCenter(records)

    // Verify all within radius
    if !d.allWithinRadius(records, centerX, centerY) {
        return nil
    }

    // Clear processed clicks
    d.redis.Del(ctx, key)

    // Create insight
    return &Insight{
        Type:       "rage_click",
        ProjectID:  event.ProjectID,
        SessionID:  sessionID,
        Timestamp:  time.Now(),
        URL:        event.URL,
        Path:       event.Path,
        X:          &centerX,
        Y:          &centerY,
        TargetSelector: event.TargetSelector,
        Details: map[string]interface{}{
            "click_count":    len(records),
            "time_window_ms": d.timeWindowMs,
            "radius_px":      d.radiusPx,
        },
        RelatedEventIDs: d.extractEventIDs(records),
    }
}

func (d *RageClickDetector) calculateCenter(clicks []ClickRecord) (int, int) {
    var sumX, sumY int
    for _, c := range clicks {
        sumX += c.X
        sumY += c.Y
    }
    return sumX / len(clicks), sumY / len(clicks)
}

func (d *RageClickDetector) allWithinRadius(clicks []ClickRecord, centerX, centerY int) bool {
    for _, c := range clicks {
        dx := c.X - centerX
        dy := c.Y - centerY
        distance := math.Sqrt(float64(dx*dx + dy*dy))
        if distance > float64(d.radiusPx) {
            return false
        }
    }
    return true
}

func (d *RageClickDetector) extractEventIDs(clicks []ClickRecord) []uuid.UUID {
    var ids []uuid.UUID
    for _, c := range clicks {
        if id, err := uuid.Parse(c.EventID); err == nil {
            ids = append(ids, id)
        }
    }
    return ids
}
```

---

### 5.5 Dead Click Detector

**`internal/insights/dead_click.go`**

```go
package insights

import (
    "sync"
    "time"

    "github.com/google/uuid"
)

type DeadClickDetector struct {
    observationWindowMs int64
    pendingClicks       sync.Map // key -> ClickContext
}

type ClickContext struct {
    Event       Event
    ExpectedTo  string // "navigate", "mutate", "handle"
    Timestamp   int64
}

var expectedInteractiveTags = []string{
    "a", "button", "input", "select", "textarea",
}

var expectedInteractiveClasses = []string{
    "btn", "button", "link", "clickable", "interactive",
}

func NewDeadClickDetector(cfg DeadClickConfig) *DeadClickDetector {
    d := &DeadClickDetector{
        observationWindowMs: cfg.ObservationWindowMs, // Default: 1000
    }

    // Start checker goroutine
    go d.checkPendingClicks()

    return d
}

func (d *DeadClickDetector) ProcessClick(sessionID uuid.UUID, event Event) {
    // Check if target looks interactive
    if !d.looksInteractive(event) {
        return
    }

    // Determine expected behavior
    expected := d.determineExpectedBehavior(event)

    // Store pending click
    key := fmt.Sprintf("%s:%s", sessionID.String(), event.EventID)
    d.pendingClicks.Store(key, ClickContext{
        Event:      event,
        ExpectedTo: expected,
        Timestamp:  event.Timestamp,
    })

    // Schedule check
    time.AfterFunc(time.Duration(d.observationWindowMs)*time.Millisecond, func() {
        d.checkForResponse(key)
    })
}

func (d *DeadClickDetector) ProcessEvent(sessionID uuid.UUID, event Event) {
    // Check if this event resolves any pending clicks
    d.pendingClicks.Range(func(key, value interface{}) bool {
        ctx := value.(ClickContext)

        if d.isResponseTo(ctx, event) {
            d.pendingClicks.Delete(key)
        }

        return true
    })
}

func (d *DeadClickDetector) isResponseTo(ctx ClickContext, event Event) bool {
    // Time check
    if event.Timestamp < ctx.Event.Timestamp {
        return false
    }

    // Within observation window
    if event.Timestamp > ctx.Event.Timestamp+d.observationWindowMs {
        return false
    }

    switch ctx.ExpectedTo {
    case "navigate":
        return event.Type == "page_view"
    case "mutate":
        return event.Type == "dom_mutation"
    case "handle":
        return event.Type != "mouse_move" && event.Type != "scroll"
    }

    return false
}

func (d *DeadClickDetector) checkForResponse(key string) {
    value, exists := d.pendingClicks.LoadAndDelete(key)
    if !exists {
        return // Already resolved
    }

    ctx := value.(ClickContext)

    // No response - this is a dead click
    // Emit insight (handled by processor)
}

func (d *DeadClickDetector) looksInteractive(event Event) bool {
    // Check tag
    for _, tag := range expectedInteractiveTags {
        if event.TargetTag == tag {
            return true
        }
    }

    // Check classes
    for _, class := range event.TargetClasses {
        for _, expected := range expectedInteractiveClasses {
            if strings.Contains(strings.ToLower(class), expected) {
                return true
            }
        }
    }

    // Check role attribute
    if event.TargetRole == "button" || event.TargetRole == "link" {
        return true
    }

    return false
}

func (d *DeadClickDetector) determineExpectedBehavior(event Event) string {
    if event.TargetTag == "a" && event.TargetHref != "" {
        return "navigate"
    }

    if event.TargetTag == "button" || event.TargetTag == "input" {
        return "handle"
    }

    return "mutate"
}
```

---

### 5.6 Error Click Detector

**`internal/insights/error_click.go`**

```go
package insights

import (
    "container/ring"
    "sync"
    "time"

    "github.com/google/uuid"
)

type ErrorClickDetector struct {
    errorWindowMs int64
    recentClicks  *ring.Ring
    mu            sync.Mutex
}

type ClickWithSession struct {
    SessionID uuid.UUID
    Event     Event
}

func NewErrorClickDetector(cfg ErrorClickConfig) *ErrorClickDetector {
    return &ErrorClickDetector{
        errorWindowMs: cfg.ErrorWindowMs, // Default: 1000
        recentClicks:  ring.New(100),     // Keep last 100 clicks
    }
}

func (d *ErrorClickDetector) ProcessClick(sessionID uuid.UUID, event Event) {
    d.mu.Lock()
    defer d.mu.Unlock()

    d.recentClicks.Value = ClickWithSession{
        SessionID: sessionID,
        Event:     event,
    }
    d.recentClicks = d.recentClicks.Next()
}

func (d *ErrorClickDetector) ProcessError(sessionID uuid.UUID, errorEvent Event) *Insight {
    d.mu.Lock()
    defer d.mu.Unlock()

    var matchingClick *ClickWithSession

    // Find most recent click in same session within error window
    d.recentClicks.Do(func(v interface{}) {
        if v == nil {
            return
        }

        click := v.(ClickWithSession)

        // Same session
        if click.SessionID != sessionID {
            return
        }

        // Within error window (click before error)
        timeDiff := errorEvent.Timestamp - click.Event.Timestamp
        if timeDiff > 0 && timeDiff <= d.errorWindowMs {
            if matchingClick == nil || click.Event.Timestamp > matchingClick.Event.Timestamp {
                matchingClick = &click
            }
        }
    })

    if matchingClick == nil {
        return nil
    }

    return &Insight{
        Type:           "error_click",
        ProjectID:      errorEvent.ProjectID,
        SessionID:      sessionID,
        Timestamp:      time.Now(),
        URL:            matchingClick.Event.URL,
        Path:           matchingClick.Event.Path,
        X:              &matchingClick.Event.ClickX,
        Y:              &matchingClick.Event.ClickY,
        TargetSelector: matchingClick.Event.TargetSelector,
        Details: map[string]interface{}{
            "error_message":  errorEvent.ErrorMessage,
            "error_type":     errorEvent.ErrorType,
            "time_to_error":  errorEvent.Timestamp - matchingClick.Event.Timestamp,
        },
        RelatedEventIDs: []uuid.UUID{
            uuid.MustParse(matchingClick.Event.EventID),
            uuid.MustParse(errorEvent.EventID),
        },
    }
}
```

---

### 5.7 Slow Page Detector

**`internal/insights/slow_page.go`**

```go
package insights

import (
    "time"

    "github.com/google/uuid"
)

type SlowPageDetector struct {
    lcpThresholdMs  int64
    ttfbThresholdMs int64
}

func NewSlowPageDetector(cfg SlowPageConfig) *SlowPageDetector {
    return &SlowPageDetector{
        lcpThresholdMs:  cfg.LCPThresholdMs,  // Default: 3000
        ttfbThresholdMs: cfg.TTFBThresholdMs, // Default: 800
    }
}

func (d *SlowPageDetector) ProcessPerformance(event Event) *Insight {
    var reasons []string
    var slowestMetric int64

    // Check LCP
    if event.LCP != nil && *event.LCP > int(d.lcpThresholdMs) {
        reasons = append(reasons, "lcp")
        if int64(*event.LCP) > slowestMetric {
            slowestMetric = int64(*event.LCP)
        }
    }

    // Check TTFB
    if event.TTFB != nil && *event.TTFB > int(d.ttfbThresholdMs) {
        reasons = append(reasons, "ttfb")
    }

    if len(reasons) == 0 {
        return nil
    }

    return &Insight{
        Type:      "slow_page",
        ProjectID: event.ProjectID,
        SessionID: event.SessionID,
        Timestamp: time.Now(),
        URL:       event.URL,
        Path:      event.Path,
        Details: map[string]interface{}{
            "load_time_ms": slowestMetric,
            "lcp":          event.LCP,
            "ttfb":         event.TTFB,
            "reasons":      reasons,
        },
    }
}
```

---

## Configuration

**`config/insights.yaml`**

```yaml
insights:
  rage_click:
    enabled: true
    min_clicks: 5
    time_window_ms: 2000
    radius_px: 50

  dead_click:
    enabled: true
    observation_window_ms: 1000

  error_click:
    enabled: true
    error_window_ms: 1000

  thrashed_cursor:
    enabled: true
    min_duration_ms: 2000
    min_direction_changes: 10
    min_velocity: 500

  u_turn:
    enabled: true
    max_time_away_ms: 10000

  slow_page:
    enabled: true
    lcp_threshold_ms: 3000
    ttfb_threshold_ms: 800
```

---

## Checklist

- [ ] Insight processor main
- [ ] Rage click detector
- [ ] Dead click detector
- [ ] Error click detector
- [ ] Thrashed cursor detector
- [ ] U-turn detector
- [ ] Slow page detector
- [ ] Store insights vào ClickHouse
- [ ] Publish alerts vào Kafka
- [ ] Unit tests cho mỗi detector

## Kết Quả

Sau phase này:
- 6 loại insights được phát hiện real-time
- Data lưu vào ClickHouse
- Alerts được publish cho Phase 9
