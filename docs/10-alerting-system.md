# GoSight - Alerting System

## 1. Overview

The Alerting System monitors metrics and insights in real-time, triggering notifications when thresholds are exceeded. Currently supports **Telegram** as the notification channel.

### Features

| Feature | Description |
|---------|-------------|
| **Threshold Alerts** | Trigger when metric exceeds value |
| **Anomaly Detection** | Detect unusual patterns |
| **Cooldown Period** | Prevent alert fatigue |
| **Alert History** | Track all triggered alerts |
| **Telegram Integration** | Real-time notifications |

---

## 2. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Insight Processor                             │
│  (Produces alerts when insights detected)                       │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Kafka: gosight.alerts                       │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Alert Processor                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │    Load     │  │  Evaluate   │  │  Cooldown   │             │
│  │    Rules    │  │  Conditions │  │   Check     │             │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘             │
│         │                │                │                      │
│         └────────────────┴────────────────┘                      │
│                          │                                       │
│                          ▼                                       │
│                 ┌─────────────────┐                             │
│                 │   Send Alert    │                             │
│                 │   (Telegram)    │                             │
│                 └─────────────────┘                             │
└─────────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                      PostgreSQL                                  │
│  (Alert rules, history, cooldown state)                         │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. Alert Rules

### 3.1 Rule Schema

```go
type AlertRule struct {
    ID            string         `json:"id"`
    ProjectID     string         `json:"project_id"`
    Name          string         `json:"name"`
    Description   string         `json:"description"`
    Condition     AlertCondition `json:"condition"`
    Channels      []Channel      `json:"channels"`
    CooldownMins  int            `json:"cooldown_mins"`
    IsActive      bool           `json:"is_active"`
    CreatedAt     time.Time      `json:"created_at"`
    LastTriggered *time.Time     `json:"last_triggered"`
}

type AlertCondition struct {
    Metric    string            `json:"metric"`
    Operator  string            `json:"operator"`  // >, <, >=, <=, ==
    Threshold float64           `json:"threshold"`
    Window    string            `json:"window"`    // 1m, 5m, 15m, 1h
    GroupBy   []string          `json:"group_by"`  // path, browser, country
    Filters   map[string]string `json:"filters"`
}

type Channel struct {
    Type   string            `json:"type"`   // telegram, webhook
    Config map[string]string `json:"config"` // chat_id, url, etc.
}
```

### 3.2 Available Metrics

| Metric | Description | Unit |
|--------|-------------|------|
| `sessions` | Active sessions | count |
| `pageviews` | Page views | count |
| `errors` | JavaScript errors | count |
| `error_rate` | Error rate | percentage |
| `rage_clicks` | Rage click detections | count |
| `dead_clicks` | Dead click detections | count |
| `slow_pages` | Slow page loads | count |
| `avg_lcp` | Average LCP | milliseconds |
| `avg_ttfb` | Average TTFB | milliseconds |
| `bounce_rate` | Bounce rate | percentage |

### 3.3 Example Rules

#### High Error Rate

```json
{
  "name": "High Error Rate",
  "description": "Alert when error rate exceeds 5%",
  "condition": {
    "metric": "error_rate",
    "operator": ">",
    "threshold": 5,
    "window": "5m"
  },
  "channels": [
    {
      "type": "telegram",
      "config": {
        "chat_id": "-123456789"
      }
    }
  ],
  "cooldown_mins": 15
}
```

#### Rage Click Spike

```json
{
  "name": "Rage Click Spike",
  "description": "Alert when rage clicks exceed threshold",
  "condition": {
    "metric": "rage_clicks",
    "operator": ">",
    "threshold": 10,
    "window": "5m",
    "group_by": ["path"]
  },
  "channels": [
    {
      "type": "telegram",
      "config": {
        "chat_id": "-123456789"
      }
    }
  ],
  "cooldown_mins": 30
}
```

#### Slow Page Performance

```json
{
  "name": "Slow LCP",
  "description": "Alert when LCP exceeds 4 seconds",
  "condition": {
    "metric": "avg_lcp",
    "operator": ">",
    "threshold": 4000,
    "window": "15m",
    "filters": {
      "path": "/checkout"
    }
  },
  "channels": [
    {
      "type": "telegram",
      "config": {
        "chat_id": "-123456789"
      }
    }
  ],
  "cooldown_mins": 60
}
```

---

## 4. Alert Processor

### 4.1 Implementation

```go
type AlertProcessor struct {
    rules       *RuleStore
    history     *HistoryStore
    cooldowns   *CooldownManager
    notifiers   map[string]Notifier
    metrics     *MetricsAggregator
}

func NewAlertProcessor(db *sql.DB, redis *redis.Client) *AlertProcessor {
    return &AlertProcessor{
        rules:     NewRuleStore(db),
        history:   NewHistoryStore(db),
        cooldowns: NewCooldownManager(redis),
        notifiers: map[string]Notifier{
            "telegram": NewTelegramNotifier(),
            "webhook":  NewWebhookNotifier(),
        },
        metrics: NewMetricsAggregator(redis),
    }
}

func (p *AlertProcessor) ProcessInsight(ctx context.Context, insight Insight) error {
    // Get all active rules for this project
    rules, err := p.rules.GetActiveRules(ctx, insight.ProjectID)
    if err != nil {
        return err
    }

    for _, rule := range rules {
        // Check if insight matches rule
        if !p.matchesRule(insight, rule) {
            continue
        }

        // Check cooldown
        if p.cooldowns.IsInCooldown(ctx, rule.ID) {
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
        err = p.triggerAlert(ctx, rule, insight, value)
        if err != nil {
            log.Error("Failed to trigger alert", "rule", rule.ID, "error", err)
        }
    }

    return nil
}

func (p *AlertProcessor) matchesRule(insight Insight, rule AlertRule) bool {
    // Map insight types to metrics
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

func (p *AlertProcessor) evaluateCondition(value float64, cond AlertCondition) bool {
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

func (p *AlertProcessor) triggerAlert(ctx context.Context, rule AlertRule, insight Insight, value float64) error {
    // Create alert record
    alert := AlertHistory{
        ID:           uuid.New().String(),
        RuleID:       rule.ID,
        ProjectID:    rule.ProjectID,
        TriggeredAt:  time.Now(),
        MetricValue:  value,
        Threshold:    rule.Condition.Threshold,
        Context:      p.buildContext(insight),
    }

    // Save to history
    err := p.history.Save(ctx, alert)
    if err != nil {
        return err
    }

    // Set cooldown
    p.cooldowns.SetCooldown(ctx, rule.ID, time.Duration(rule.CooldownMins)*time.Minute)

    // Send notifications
    for _, channel := range rule.Channels {
        notifier, ok := p.notifiers[channel.Type]
        if !ok {
            continue
        }

        err = notifier.Send(ctx, channel.Config, p.buildMessage(rule, alert))
        if err != nil {
            log.Error("Failed to send notification", "channel", channel.Type, "error", err)
        }
    }

    // Update rule last triggered
    p.rules.UpdateLastTriggered(ctx, rule.ID, time.Now())

    return nil
}
```

### 4.2 Cooldown Manager

```go
type CooldownManager struct {
    redis *redis.Client
}

func (m *CooldownManager) IsInCooldown(ctx context.Context, ruleID string) bool {
    key := fmt.Sprintf("alert:cooldown:%s", ruleID)
    exists, _ := m.redis.Exists(ctx, key).Result()
    return exists > 0
}

func (m *CooldownManager) SetCooldown(ctx context.Context, ruleID string, duration time.Duration) {
    key := fmt.Sprintf("alert:cooldown:%s", ruleID)
    m.redis.Set(ctx, key, "1", duration)
}

func (m *CooldownManager) ClearCooldown(ctx context.Context, ruleID string) {
    key := fmt.Sprintf("alert:cooldown:%s", ruleID)
    m.redis.Del(ctx, key)
}
```

### 4.3 Metrics Aggregator

```go
type MetricsAggregator struct {
    redis      *redis.Client
    clickhouse *sql.DB
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
    case "avg_lcp":
        return a.avgPerformanceMetric(ctx, projectID, "lcp", window)
    default:
        return 0, fmt.Errorf("unknown metric: %s", cond.Metric)
    }
}

func (a *MetricsAggregator) countInsights(ctx context.Context, projectID, insightType string, window time.Duration) (float64, error) {
    query := `
        SELECT count()
        FROM insights
        WHERE project_id = ?
          AND insight_type = ?
          AND timestamp > now() - interval ? second
    `

    var count uint64
    err := a.clickhouse.QueryRow(ctx, query, projectID, insightType, int(window.Seconds())).Scan(&count)
    return float64(count), err
}

func (a *MetricsAggregator) calculateErrorRate(ctx context.Context, projectID string, window time.Duration) (float64, error) {
    query := `
        SELECT
            countIf(event_type = 'js_error') AS errors,
            uniqExact(session_id) AS sessions
        FROM events
        WHERE project_id = ?
          AND timestamp > now() - interval ? second
    `

    var errors, sessions uint64
    err := a.clickhouse.QueryRow(ctx, query, projectID, int(window.Seconds())).Scan(&errors, &sessions)
    if err != nil || sessions == 0 {
        return 0, err
    }

    return float64(errors) / float64(sessions) * 100, nil
}
```

---

## 5. Telegram Integration

### 5.1 Bot Setup

1. Create bot via [@BotFather](https://t.me/BotFather)
2. Get bot token
3. Add bot to group/channel
4. Get chat ID

### 5.2 Telegram Notifier

```go
type TelegramNotifier struct {
    botToken string
    client   *http.Client
}

func NewTelegramNotifier() *TelegramNotifier {
    return &TelegramNotifier{
        botToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
        client:   &http.Client{Timeout: 10 * time.Second},
    }
}

func (n *TelegramNotifier) Send(ctx context.Context, config map[string]string, message AlertMessage) error {
    chatID := config["chat_id"]
    if chatID == "" {
        return errors.New("chat_id is required")
    }

    // Build message text
    text := n.formatMessage(message)

    // Send request
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
    if msg.Severity == "critical" {
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
        if path, ok := msg.Context["path"]; ok {
            sb.WriteString(fmt.Sprintf("📍 <b>Path:</b> %s\n", path))
        }
        if sessions, ok := msg.Context["affected_sessions"]; ok {
            sb.WriteString(fmt.Sprintf("👥 <b>Affected Sessions:</b> %v\n", sessions))
        }
    }

    // Link
    sb.WriteString(fmt.Sprintf("\n🔗 <a href=\"%s\">View in Dashboard</a>", msg.DashboardURL))

    return sb.String()
}
```

### 5.3 Message Format

```
🚨 High Error Rate Alert

📊 Metric: error_rate
📈 Value: 7.5%
🎯 Threshold: 5%
⏰ Time: 14:32:15 UTC

📍 Path: /checkout
👥 Affected Sessions: 23

🔗 View in Dashboard
```

---

## 6. Webhook Integration (Future)

```go
type WebhookNotifier struct {
    client *http.Client
}

func (n *WebhookNotifier) Send(ctx context.Context, config map[string]string, message AlertMessage) error {
    webhookURL := config["url"]
    if webhookURL == "" {
        return errors.New("url is required")
    }

    payload := WebhookPayload{
        Alert:     message,
        Timestamp: time.Now().Unix(),
        Version:   "1.0",
    }

    body, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    // Add signature for verification
    signature := n.signPayload(body, config["secret"])
    req.Header.Set("X-GoSight-Signature", signature)

    resp, err := n.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        return fmt.Errorf("webhook error: status %d", resp.StatusCode)
    }

    return nil
}
```

---

## 7. API Endpoints

### Create Alert Rule

```http
POST /api/v1/projects/:id/alerts
Content-Type: application/json

{
  "name": "High Error Rate",
  "condition": {
    "metric": "error_rate",
    "operator": ">",
    "threshold": 5,
    "window": "5m"
  },
  "channels": [
    {
      "type": "telegram",
      "config": {
        "chat_id": "-123456789"
      }
    }
  ],
  "cooldown_mins": 15
}
```

### List Alert Rules

```http
GET /api/v1/projects/:id/alerts

Response:
{
  "success": true,
  "data": [
    {
      "id": "alert_123",
      "name": "High Error Rate",
      "condition": {...},
      "channels": [...],
      "is_active": true,
      "last_triggered": "2024-01-20T10:00:00Z"
    }
  ]
}
```

### Get Alert History

```http
GET /api/v1/projects/:id/alerts/history?start_date=2024-01-01&end_date=2024-01-20

Response:
{
  "success": true,
  "data": [
    {
      "id": "hist_123",
      "rule_id": "alert_123",
      "rule_name": "High Error Rate",
      "triggered_at": "2024-01-20T10:00:00Z",
      "metric_value": 7.5,
      "threshold": 5,
      "context": {
        "path": "/checkout",
        "affected_sessions": 23
      }
    }
  ]
}
```

### Test Alert

```http
POST /api/v1/projects/:id/alerts/:alertId/test

Response:
{
  "success": true,
  "message": "Test notification sent"
}
```

---

## 8. Dashboard UI

### Alert Rules List

```typescript
function AlertRulesList({ projectId }) {
  const { data: rules } = useAlertRules(projectId);

  return (
    <div className="alert-rules">
      <header>
        <h2>Alert Rules</h2>
        <Button onClick={() => openCreateModal()}>Create Rule</Button>
      </header>

      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Condition</th>
            <th>Status</th>
            <th>Last Triggered</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {rules.map((rule) => (
            <tr key={rule.id}>
              <td>{rule.name}</td>
              <td>
                {rule.condition.metric} {rule.condition.operator} {rule.condition.threshold}
              </td>
              <td>
                <Toggle
                  checked={rule.is_active}
                  onChange={() => toggleRule(rule.id)}
                />
              </td>
              <td>
                {rule.last_triggered
                  ? formatRelative(rule.last_triggered)
                  : 'Never'}
              </td>
              <td>
                <Button onClick={() => editRule(rule)}>Edit</Button>
                <Button onClick={() => testRule(rule.id)}>Test</Button>
                <Button onClick={() => deleteRule(rule.id)}>Delete</Button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
```

### Create/Edit Rule Modal

```typescript
function AlertRuleModal({ rule, onSave, onClose }) {
  const [form, setForm] = useState(rule || defaultRule);

  return (
    <Modal onClose={onClose}>
      <h3>{rule ? 'Edit Alert Rule' : 'Create Alert Rule'}</h3>

      <FormField label="Name">
        <Input
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
        />
      </FormField>

      <FormField label="Metric">
        <Select
          value={form.condition.metric}
          options={METRIC_OPTIONS}
          onChange={(v) => setForm({
            ...form,
            condition: { ...form.condition, metric: v }
          })}
        />
      </FormField>

      <FormField label="Condition">
        <div className="condition-row">
          <Select
            value={form.condition.operator}
            options={OPERATOR_OPTIONS}
          />
          <Input
            type="number"
            value={form.condition.threshold}
          />
        </div>
      </FormField>

      <FormField label="Time Window">
        <Select
          value={form.condition.window}
          options={WINDOW_OPTIONS}
        />
      </FormField>

      <FormField label="Telegram Chat ID">
        <Input
          value={form.channels[0]?.config?.chat_id || ''}
          placeholder="-123456789"
        />
      </FormField>

      <FormField label="Cooldown (minutes)">
        <Input
          type="number"
          value={form.cooldown_mins}
        />
      </FormField>

      <div className="modal-actions">
        <Button variant="secondary" onClick={onClose}>Cancel</Button>
        <Button onClick={() => onSave(form)}>Save</Button>
      </div>
    </Modal>
  );
}
```

---

## 9. Configuration

```yaml
# config/alerting.yaml
alerting:
  enabled: true

  # Kafka consumer settings
  kafka:
    topic: gosight.alerts
    consumer_group: gosight-alert-processor
    batch_size: 100
    batch_timeout: 1s

  # Telegram settings
  telegram:
    bot_token: ${TELEGRAM_BOT_TOKEN}
    rate_limit: 30  # messages per second

  # Default cooldown
  default_cooldown_mins: 15

  # Metrics aggregation
  aggregation:
    cache_ttl: 30s
    query_timeout: 5s
```

---

## 10. References

- [UX Insights Algorithms](./07-ux-insights-algorithms.md)
- [API Specification](./05-api-specification.md)
- [System Architecture](./02-system-architecture.md)
