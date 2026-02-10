# Phase 9: Alert Processor

## Mục Tiêu

Xây dựng service xử lý và gửi alerts qua Telegram.

## Prerequisites

- Phase 5 hoàn thành (Insights đang publish vào Kafka)

## Tasks

### 9.1 Alert Processor Main

**`cmd/alert-processor/main.go`**

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/rs/zerolog/log"

    "github.com/gosight/gosight/processor/internal/alerts"
    "github.com/gosight/gosight/processor/internal/config"
    "github.com/gosight/gosight/processor/internal/consumer"
)

func main() {
    cfg, err := config.Load(os.Getenv("CONFIG_PATH"))
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to load config")
    }

    // Initialize alert processor
    alertProcessor := alerts.NewProcessor(cfg)

    // Create Kafka consumer for alerts topic
    kafkaConsumer, _ := consumer.NewKafkaConsumer(consumer.Config{
        Brokers:       cfg.Kafka.Brokers,
        Topic:         "gosight.alerts",
        ConsumerGroup: "gosight-alert-processor",
    }, alertProcessor)

    ctx, cancel := context.WithCancel(context.Background())
    go kafkaConsumer.Start(ctx)

    log.Info().Msg("Alert processor started")

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    cancel()
    kafkaConsumer.Close()
}
```

---

### 9.2 Alert Processor

**`internal/alerts/processor.go`**

```go
package alerts

import (
    "context"
    "encoding/json"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/redis/go-redis/v9"
    "github.com/rs/zerolog/log"

    "github.com/gosight/gosight/processor/internal/config"
)

type Processor struct {
    db        *pgxpool.Pool
    redis     *redis.Client
    notifiers map[string]Notifier
    metrics   *MetricsAggregator
}

type Notifier interface {
    Send(ctx context.Context, config map[string]string, message AlertMessage) error
}

func NewProcessor(cfg *config.Config) *Processor {
    db, _ := pgxpool.New(context.Background(), cfg.Postgres.DSN)
    rdb := redis.NewClient(&redis.Options{
        Addr:     cfg.Redis.Addr,
        Password: cfg.Redis.Password,
    })

    return &Processor{
        db:    db,
        redis: rdb,
        notifiers: map[string]Notifier{
            "telegram": NewTelegramNotifier(cfg.Telegram.BotToken),
        },
        metrics: NewMetricsAggregator(rdb, cfg.ClickHouse),
    }
}

func (p *Processor) Process(ctx context.Context, raw map[string]interface{}) error {
    insight := p.parseInsight(raw)

    // Get all active rules for this project
    rules, err := p.getActiveRules(ctx, insight.ProjectID)
    if err != nil {
        return err
    }

    for _, rule := range rules {
        // Check if insight matches rule
        if !p.matchesRule(insight, rule) {
            continue
        }

        // Check cooldown
        if p.isInCooldown(ctx, rule.ID) {
            continue
        }

        // Get current metric value
        value, err := p.metrics.GetMetricValue(ctx, insight.ProjectID, rule.Condition)
        if err != nil {
            continue
        }

        // Evaluate condition
        if !p.evaluateCondition(value, rule.Condition) {
            continue
        }

        // Trigger alert
        if err := p.triggerAlert(ctx, rule, insight, value); err != nil {
            log.Error().Err(err).Str("rule_id", rule.ID).Msg("Failed to trigger alert")
        }
    }

    return nil
}

func (p *Processor) getActiveRules(ctx context.Context, projectID string) ([]AlertRule, error) {
    rows, err := p.db.Query(ctx, `
        SELECT id, name, condition, channels, cooldown_mins
        FROM alert_rules
        WHERE project_id = $1 AND is_active = true
    `, projectID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var rules []AlertRule
    for rows.Next() {
        var rule AlertRule
        var conditionJSON, channelsJSON []byte

        err := rows.Scan(&rule.ID, &rule.Name, &conditionJSON, &channelsJSON, &rule.CooldownMins)
        if err != nil {
            continue
        }

        json.Unmarshal(conditionJSON, &rule.Condition)
        json.Unmarshal(channelsJSON, &rule.Channels)

        rules = append(rules, rule)
    }

    return rules, nil
}

func (p *Processor) matchesRule(insight Insight, rule AlertRule) bool {
    metricMap := map[string]string{
        "rage_click":      "rage_clicks",
        "dead_click":      "dead_clicks",
        "error_click":     "errors",
        "slow_page":       "slow_pages",
        "thrashed_cursor": "thrashed_cursors",
    }

    expectedMetric, ok := metricMap[insight.Type]
    if !ok {
        return false
    }

    return rule.Condition.Metric == expectedMetric
}

func (p *Processor) evaluateCondition(value float64, cond AlertCondition) bool {
    switch cond.Operator {
    case ">":
        return value > cond.Threshold
    case ">=":
        return value >= cond.Threshold
    case "<":
        return value < cond.Threshold
    case "<=":
        return value <= cond.Threshold
    case "==":
        return value == cond.Threshold
    default:
        return false
    }
}

func (p *Processor) isInCooldown(ctx context.Context, ruleID string) bool {
    key := "alert:cooldown:" + ruleID
    exists, _ := p.redis.Exists(ctx, key).Result()
    return exists > 0
}

func (p *Processor) setCooldown(ctx context.Context, ruleID string, mins int) {
    key := "alert:cooldown:" + ruleID
    p.redis.Set(ctx, key, "1", time.Duration(mins)*time.Minute)
}

func (p *Processor) triggerAlert(ctx context.Context, rule AlertRule, insight Insight, value float64) error {
    // Save to history
    _, err := p.db.Exec(ctx, `
        INSERT INTO alert_history (rule_id, project_id, metric_value, threshold, context)
        VALUES ($1, $2, $3, $4, $5)
    `, rule.ID, insight.ProjectID, value, rule.Condition.Threshold, insight.Details)
    if err != nil {
        return err
    }

    // Set cooldown
    p.setCooldown(ctx, rule.ID, rule.CooldownMins)

    // Build message
    message := AlertMessage{
        Title:       rule.Name,
        Metric:      rule.Condition.Metric,
        Value:       value,
        Threshold:   rule.Condition.Threshold,
        Timestamp:   time.Now(),
        Context:     insight.Details,
        DashboardURL: p.buildDashboardURL(insight),
    }

    // Send to all channels
    for _, channel := range rule.Channels {
        notifier, ok := p.notifiers[channel.Type]
        if !ok {
            continue
        }

        if err := notifier.Send(ctx, channel.Config, message); err != nil {
            log.Error().Err(err).
                Str("channel", channel.Type).
                Msg("Failed to send notification")
        }
    }

    // Update last triggered
    p.db.Exec(ctx, `
        UPDATE alert_rules SET last_triggered = NOW() WHERE id = $1
    `, rule.ID)

    return nil
}
```

---

### 9.3 Telegram Notifier

**`internal/alerts/telegram.go`**

```go
package alerts

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"
)

type TelegramNotifier struct {
    botToken string
    client   *http.Client
}

func NewTelegramNotifier(botToken string) *TelegramNotifier {
    return &TelegramNotifier{
        botToken: botToken,
        client:   &http.Client{Timeout: 10 * time.Second},
    }
}

func (n *TelegramNotifier) Send(ctx context.Context, config map[string]string, message AlertMessage) error {
    chatID := config["chat_id"]
    if chatID == "" {
        return fmt.Errorf("chat_id is required")
    }

    text := n.formatMessage(message)

    url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.botToken)

    payload := map[string]interface{}{
        "chat_id":    chatID,
        "text":       text,
        "parse_mode": "HTML",
    }

    body, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    resp, err := n.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("telegram error: %s", string(body))
    }

    return nil
}

func (n *TelegramNotifier) formatMessage(msg AlertMessage) string {
    var sb strings.Builder

    // Header with emoji based on severity
    emoji := "⚠️"
    if msg.Value > msg.Threshold*2 {
        emoji = "🚨"
    }

    sb.WriteString(fmt.Sprintf("%s <b>%s</b>\n\n", emoji, msg.Title))

    // Details
    sb.WriteString(fmt.Sprintf("📊 <b>Metric:</b> %s\n", msg.Metric))
    sb.WriteString(fmt.Sprintf("📈 <b>Value:</b> %.2f\n", msg.Value))
    sb.WriteString(fmt.Sprintf("🎯 <b>Threshold:</b> %.2f\n", msg.Threshold))
    sb.WriteString(fmt.Sprintf("⏰ <b>Time:</b> %s\n", msg.Timestamp.Format("15:04:05 MST")))

    // Context
    if msg.Context != nil {
        if path, ok := msg.Context["path"].(string); ok && path != "" {
            sb.WriteString(fmt.Sprintf("📍 <b>Path:</b> %s\n", path))
        }
    }

    // Link
    sb.WriteString(fmt.Sprintf("\n🔗 <a href=\"%s\">View in Dashboard</a>", msg.DashboardURL))

    return sb.String()
}
```

---

### 9.4 Metrics Aggregator

**`internal/alerts/metrics.go`**

```go
package alerts

import (
    "context"
    "time"

    "github.com/ClickHouse/clickhouse-go/v2"
    "github.com/redis/go-redis/v9"
)

type MetricsAggregator struct {
    redis      *redis.Client
    clickhouse clickhouse.Conn
}

func NewMetricsAggregator(redis *redis.Client, chCfg config.ClickHouseConfig) *MetricsAggregator {
    conn, _ := clickhouse.Open(&clickhouse.Options{
        Addr: []string{chCfg.Addr},
        Auth: clickhouse.Auth{
            Database: chCfg.Database,
            Username: chCfg.Username,
            Password: chCfg.Password,
        },
    })

    return &MetricsAggregator{
        redis:      redis,
        clickhouse: conn,
    }
}

func (a *MetricsAggregator) GetMetricValue(ctx context.Context, projectID string, cond AlertCondition) (float64, error) {
    window := a.parseWindow(cond.Window)

    switch cond.Metric {
    case "sessions":
        return a.countSessions(ctx, projectID, window)
    case "errors":
        return a.countErrors(ctx, projectID, window)
    case "error_rate":
        return a.calculateErrorRate(ctx, projectID, window)
    case "rage_clicks":
        return a.countInsights(ctx, projectID, "rage_click", window)
    case "dead_clicks":
        return a.countInsights(ctx, projectID, "dead_click", window)
    case "slow_pages":
        return a.countInsights(ctx, projectID, "slow_page", window)
    case "avg_lcp":
        return a.avgPerformanceMetric(ctx, projectID, "lcp", window)
    default:
        return 0, fmt.Errorf("unknown metric: %s", cond.Metric)
    }
}

func (a *MetricsAggregator) parseWindow(window string) time.Duration {
    switch window {
    case "1m":
        return time.Minute
    case "5m":
        return 5 * time.Minute
    case "15m":
        return 15 * time.Minute
    case "1h":
        return time.Hour
    default:
        return 5 * time.Minute
    }
}

func (a *MetricsAggregator) countInsights(ctx context.Context, projectID, insightType string, window time.Duration) (float64, error) {
    var count uint64
    err := a.clickhouse.QueryRow(ctx, `
        SELECT count()
        FROM insights
        WHERE project_id = ?
          AND insight_type = ?
          AND timestamp > now() - interval ? second
    `, projectID, insightType, int(window.Seconds())).Scan(&count)

    return float64(count), err
}

func (a *MetricsAggregator) calculateErrorRate(ctx context.Context, projectID string, window time.Duration) (float64, error) {
    var errors, sessions uint64
    err := a.clickhouse.QueryRow(ctx, `
        SELECT
            countIf(event_type = 'js_error') AS errors,
            uniqExact(session_id) AS sessions
        FROM events
        WHERE project_id = ?
          AND timestamp > now() - interval ? second
    `, projectID, int(window.Seconds())).Scan(&errors, &sessions)

    if err != nil || sessions == 0 {
        return 0, err
    }

    return float64(errors) / float64(sessions) * 100, nil
}

func (a *MetricsAggregator) avgPerformanceMetric(ctx context.Context, projectID, metric string, window time.Duration) (float64, error) {
    column := metric
    var avg float64
    err := a.clickhouse.QueryRow(ctx, fmt.Sprintf(`
        SELECT avg(%s)
        FROM events
        WHERE project_id = ?
          AND event_type = 'web_vitals'
          AND %s IS NOT NULL
          AND timestamp > now() - interval ? second
    `, column, column), projectID, int(window.Seconds())).Scan(&avg)

    return avg, err
}
```

---

### 9.5 Types

**`internal/alerts/types.go`**

```go
package alerts

import "time"

type AlertRule struct {
    ID           string         `json:"id"`
    ProjectID    string         `json:"project_id"`
    Name         string         `json:"name"`
    Condition    AlertCondition `json:"condition"`
    Channels     []Channel      `json:"channels"`
    CooldownMins int            `json:"cooldown_mins"`
}

type AlertCondition struct {
    Metric    string  `json:"metric"`
    Operator  string  `json:"operator"`
    Threshold float64 `json:"threshold"`
    Window    string  `json:"window"`
}

type Channel struct {
    Type   string            `json:"type"`
    Config map[string]string `json:"config"`
}

type AlertMessage struct {
    Title        string                 `json:"title"`
    Metric       string                 `json:"metric"`
    Value        float64                `json:"value"`
    Threshold    float64                `json:"threshold"`
    Timestamp    time.Time              `json:"timestamp"`
    Severity     string                 `json:"severity"`
    Context      map[string]interface{} `json:"context"`
    DashboardURL string                 `json:"dashboard_url"`
}

type Insight struct {
    Type      string                 `json:"type"`
    ProjectID string                 `json:"project_id"`
    SessionID string                 `json:"session_id"`
    Timestamp time.Time              `json:"timestamp"`
    URL       string                 `json:"url"`
    Path      string                 `json:"path"`
    Details   map[string]interface{} `json:"details"`
}
```

---

### 9.6 Configuration

**`config/alerting.yaml`**

```yaml
alerting:
  enabled: true

  kafka:
    topic: gosight.alerts
    consumer_group: gosight-alert-processor
    batch_size: 100

  telegram:
    bot_token: ${TELEGRAM_BOT_TOKEN}
    rate_limit: 30  # messages per second

  default_cooldown_mins: 15

  aggregation:
    cache_ttl: 30s
    query_timeout: 5s
```

---

## Telegram Bot Setup

1. **Create Bot:**
   - Chat with [@BotFather](https://t.me/BotFather)
   - Send `/newbot`
   - Follow instructions to get bot token

2. **Get Chat ID:**
   - Add bot to group/channel
   - Send a message in the group
   - Visit: `https://api.telegram.org/bot<TOKEN>/getUpdates`
   - Find `chat.id` in response

3. **Configure:**
   ```bash
   export TELEGRAM_BOT_TOKEN=your_bot_token
   ```

---

## Checklist

- [ ] Alert processor main
- [ ] Load rules từ PostgreSQL
- [ ] Evaluate conditions
- [ ] Cooldown management (Redis)
- [ ] Metrics aggregator
- [ ] Telegram notifier
- [ ] Save alert history
- [ ] Dockerfile
- [ ] Unit tests

## Kết Quả

Sau phase này:
- Alerts được trigger dựa trên rules
- Notifications gửi qua Telegram
- Cooldown tránh spam
- Alert history lưu trong PostgreSQL
