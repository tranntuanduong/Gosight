# GoSight - UX Insights Algorithms

## 1. Overview

This document details the algorithms used to detect UX insights from raw event data. These insights help identify user frustration and usability issues.

### Insight Types

| Insight | Description | Severity |
|---------|-------------|----------|
| **Rage Click** | Rapid repeated clicking | High |
| **Dead Click** | Click on non-interactive element | Medium |
| **Error Click** | Click that causes JS error | High |
| **Thrashed Cursor** | Erratic mouse movement | Medium |
| **U-Turn** | Quick return to previous page | Low |
| **Slow Page** | Page with poor performance | Medium |

---

## 2. Rage Click Detection

### Definition

A **rage click** occurs when a user clicks rapidly and repeatedly in a small area, typically indicating frustration with an unresponsive UI element.

### Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `min_clicks` | 5 | Minimum clicks to trigger |
| `time_window_ms` | 2000 | Time window for clicks |
| `radius_px` | 50 | Maximum area radius |

### Algorithm

```go
type RageClickDetector struct {
    minClicks    int
    timeWindowMs int64
    radiusPx     int
    redis        *redis.Client
}

type ClickRecord struct {
    X         int
    Y         int
    Timestamp int64
    EventID   string
}

func (d *RageClickDetector) ProcessClick(sessionID string, click ClickRecord) *RageClickInsight {
    // Grid cell for spatial grouping (50px cells)
    gridX := click.X / d.radiusPx
    gridY := click.Y / d.radiusPx

    key := fmt.Sprintf("clicks:%s:%d:%d", sessionID, gridX, gridY)

    // Add click to Redis list
    d.redis.RPush(ctx, key, serializeClick(click))
    d.redis.Expire(ctx, key, time.Duration(d.timeWindowMs)*time.Millisecond*2)

    // Get all clicks in this cell
    clicks := d.redis.LRange(ctx, key, 0, -1)

    // Filter to time window
    now := time.Now().UnixMilli()
    recentClicks := filterRecent(clicks, now-d.timeWindowMs)

    // Check if rage click threshold met
    if len(recentClicks) >= d.minClicks {
        // Calculate cluster center
        centerX, centerY := calculateCenter(recentClicks)

        // Verify all clicks within radius of center
        if allWithinRadius(recentClicks, centerX, centerY, d.radiusPx) {
            // Clear processed clicks
            d.redis.Del(ctx, key)

            return &RageClickInsight{
                ClickCount:    len(recentClicks),
                TimeWindowMs:  d.timeWindowMs,
                CenterX:       centerX,
                CenterY:       centerY,
                Radius:        d.radiusPx,
                RelatedEvents: extractEventIDs(recentClicks),
            }
        }
    }

    return nil
}

func calculateCenter(clicks []ClickRecord) (int, int) {
    var sumX, sumY int
    for _, c := range clicks {
        sumX += c.X
        sumY += c.Y
    }
    return sumX / len(clicks), sumY / len(clicks)
}

func allWithinRadius(clicks []ClickRecord, centerX, centerY, radius int) bool {
    for _, c := range clicks {
        dx := c.X - centerX
        dy := c.Y - centerY
        distance := math.Sqrt(float64(dx*dx + dy*dy))
        if distance > float64(radius) {
            return false
        }
    }
    return true
}
```

### State Management

```
Redis Key: clicks:{session_id}:{grid_x}:{grid_y}
Type: List
TTL: 10 seconds
Value: Serialized click records
```

### Edge Cases

1. **Multi-element rage**: User rage-clicking across multiple nearby elements
   - Solution: Use grid cells, not exact coordinates

2. **Legitimate rapid clicks**: Gaming, counter buttons
   - Solution: Configurable thresholds per project

3. **Touch devices**: Double-tap zoom can look like rage click
   - Solution: Filter out standard touch gestures

---

## 3. Dead Click Detection

### Definition

A **dead click** occurs when a user clicks on an element expecting interaction, but nothing happens.

### Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `observation_window_ms` | 1000 | Time to wait for response |
| `min_expected_tags` | See list | Tags expected to be interactive |

### Expected Interactive Elements

```go
var expectedInteractiveTags = []string{
    "a", "button", "input", "select", "textarea",
}

var expectedInteractiveSelectors = []string{
    "[role='button']",
    "[role='link']",
    "[role='checkbox']",
    "[role='radio']",
    "[onclick]",
    "[ng-click]",
    "[@click]",
    "[data-action]",
}

var expectedInteractiveClasses = []string{
    "btn", "button", "link", "clickable", "interactive",
}
```

### Algorithm

```go
type DeadClickDetector struct {
    observationWindowMs int64
    pendingClicks       sync.Map // sessionID:clickID -> ClickContext
}

type ClickContext struct {
    Click       ClickEvent
    ExpectedTo  string // "navigate", "mutate", "handle"
    Timestamp   int64
    DOMSnapshot []byte
}

func (d *DeadClickDetector) ProcessClick(session string, click ClickEvent) {
    // Check if target looks interactive
    if !d.looksInteractive(click.Target) {
        return // Not expected to do anything
    }

    // Determine expected behavior
    expected := d.determineExpectedBehavior(click.Target)

    // Store pending click
    key := fmt.Sprintf("%s:%s", session, click.EventID)
    d.pendingClicks.Store(key, ClickContext{
        Click:      click,
        ExpectedTo: expected,
        Timestamp:  click.Timestamp,
    })

    // Schedule check after observation window
    time.AfterFunc(time.Duration(d.observationWindowMs)*time.Millisecond, func() {
        d.checkForResponse(key)
    })
}

func (d *DeadClickDetector) ProcessEvent(session string, event Event) {
    // Check if this event resolves any pending clicks
    d.pendingClicks.Range(func(key, value interface{}) bool {
        ctx := value.(ClickContext)

        // Check if event is a response to the click
        if d.isResponseTo(ctx, event) {
            d.pendingClicks.Delete(key)
        }

        return true
    })
}

func (d *DeadClickDetector) isResponseTo(ctx ClickContext, event Event) bool {
    // Time check: event must be after click
    if event.Timestamp < ctx.Click.Timestamp {
        return false
    }

    // Within observation window
    if event.Timestamp > ctx.Click.Timestamp+d.observationWindowMs {
        return false
    }

    switch ctx.ExpectedTo {
    case "navigate":
        return event.Type == "page_view"

    case "mutate":
        return event.Type == "dom_mutation"

    case "handle":
        // Any non-trivial event could indicate handling
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

    // No response received - this is a dead click
    d.emitInsight(DeadClickInsight{
        X:              ctx.Click.X,
        Y:              ctx.Click.Y,
        TargetSelector: ctx.Click.Target.Selector,
        Reason:         "no_" + ctx.ExpectedTo,
    })
}

func (d *DeadClickDetector) determineExpectedBehavior(target TargetElement) string {
    // Links should navigate
    if target.Tag == "a" && target.Href != "" {
        return "navigate"
    }

    // Buttons/inputs should trigger handlers
    if target.Tag == "button" || target.Tag == "input" {
        return "handle"
    }

    // Elements with click handlers
    if hasClickHandler(target) {
        return "handle"
    }

    // Default: expect DOM mutation
    return "mutate"
}

func (d *DeadClickDetector) looksInteractive(target TargetElement) bool {
    // Check tag
    for _, tag := range expectedInteractiveTags {
        if target.Tag == tag {
            return true
        }
    }

    // Check classes
    for _, class := range target.Classes {
        for _, expected := range expectedInteractiveClasses {
            if strings.Contains(strings.ToLower(class), expected) {
                return true
            }
        }
    }

    // Check role attribute
    if role, ok := target.Attributes["role"]; ok {
        if role == "button" || role == "link" || role == "checkbox" {
            return true
        }
    }

    // Check for click handlers
    if hasClickHandler(target) {
        return true
    }

    // Check cursor style (if available from replay data)
    if target.ComputedStyle != nil {
        if target.ComputedStyle["cursor"] == "pointer" {
            return true
        }
    }

    return false
}
```

### Response Types

| Expected | Response Event | Description |
|----------|---------------|-------------|
| `navigate` | page_view | Link should navigate |
| `mutate` | dom_mutation | Element should update DOM |
| `handle` | Any event | Handler should fire |

---

## 4. Error Click Detection

### Definition

An **error click** is a click that leads to a JavaScript error within a short time window.

### Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `error_window_ms` | 1000 | Max time between click and error |

### Algorithm

```go
type ErrorClickDetector struct {
    errorWindowMs int64
    recentClicks  *ring.Ring // Circular buffer of recent clicks
    mu            sync.Mutex
}

func (d *ErrorClickDetector) ProcessClick(session string, click ClickEvent) {
    d.mu.Lock()
    defer d.mu.Unlock()

    d.recentClicks.Value = ClickWithSession{
        Session: session,
        Click:   click,
    }
    d.recentClicks = d.recentClicks.Next()
}

func (d *ErrorClickDetector) ProcessError(session string, error JsErrorEvent) *ErrorClickInsight {
    d.mu.Lock()
    defer d.mu.Unlock()

    var matchingClick *ClickWithSession

    // Find most recent click in session within error window
    d.recentClicks.Do(func(v interface{}) {
        if v == nil {
            return
        }

        click := v.(ClickWithSession)

        // Same session
        if click.Session != session {
            return
        }

        // Within error window (click before error)
        timeDiff := error.Timestamp - click.Click.Timestamp
        if timeDiff > 0 && timeDiff <= d.errorWindowMs {
            if matchingClick == nil || click.Click.Timestamp > matchingClick.Click.Timestamp {
                matchingClick = &click
            }
        }
    })

    if matchingClick != nil {
        return &ErrorClickInsight{
            X:              matchingClick.Click.X,
            Y:              matchingClick.Click.Y,
            TargetSelector: matchingClick.Click.Target.Selector,
            ErrorEventID:   error.EventID,
            TimeToErrorMs:  error.Timestamp - matchingClick.Click.Timestamp,
        }
    }

    return nil
}
```

---

## 5. Thrashed Cursor Detection

### Definition

**Thrashed cursor** indicates erratic mouse movement, often signifying user confusion or frustration.

### Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `min_duration_ms` | 2000 | Minimum duration of erratic movement |
| `min_direction_changes` | 10 | Minimum direction reversals |
| `min_velocity` | 500 | Minimum pixels per second |

### Algorithm

```go
type ThrashDetector struct {
    minDurationMs       int64
    minDirectionChanges int
    minVelocity         float64

    sessions map[string]*MouseMovementBuffer
    mu       sync.Mutex
}

type MouseMovementBuffer struct {
    positions []MousePosition
    lastCheck int64
}

type Direction int

const (
    DirUp Direction = iota
    DirDown
    DirLeft
    DirRight
)

func (d *ThrashDetector) ProcessMouseMove(session string, move MouseMoveEvent) *ThrashedCursorInsight {
    d.mu.Lock()
    buf, exists := d.sessions[session]
    if !exists {
        buf = &MouseMovementBuffer{}
        d.sessions[session] = buf
    }
    d.mu.Unlock()

    // Add positions
    buf.positions = append(buf.positions, move.Positions...)

    // Clean old positions (keep last 5 seconds)
    buf.positions = filterRecentPositions(buf.positions, 5000)

    // Analyze if enough data
    if len(buf.positions) < 20 {
        return nil
    }

    // Calculate metrics
    metrics := d.calculateMetrics(buf.positions)

    // Check thresholds
    if metrics.Duration >= d.minDurationMs &&
        metrics.DirectionChanges >= d.minDirectionChanges &&
        metrics.AvgVelocity >= d.minVelocity {

        // Clear buffer after detection
        buf.positions = nil

        return &ThrashedCursorInsight{
            DurationMs:       metrics.Duration,
            DistancePx:       metrics.TotalDistance,
            DirectionChanges: metrics.DirectionChanges,
            AreaBounds:       metrics.Bounds,
        }
    }

    return nil
}

type MovementMetrics struct {
    Duration         int64
    TotalDistance    float64
    DirectionChanges int
    AvgVelocity      float64
    Bounds           Rect
}

func (d *ThrashDetector) calculateMetrics(positions []MousePosition) MovementMetrics {
    if len(positions) < 2 {
        return MovementMetrics{}
    }

    var (
        totalDistance    float64
        directionChanges int
        lastDirection    Direction
        minX, maxX       = positions[0].X, positions[0].X
        minY, maxY       = positions[0].Y, positions[0].Y
    )

    for i := 1; i < len(positions); i++ {
        prev := positions[i-1]
        curr := positions[i]

        // Calculate distance
        dx := float64(curr.X - prev.X)
        dy := float64(curr.Y - prev.Y)
        distance := math.Sqrt(dx*dx + dy*dy)
        totalDistance += distance

        // Determine direction
        currentDir := d.getDirection(dx, dy)

        // Count direction changes
        if i > 1 && currentDir != lastDirection {
            directionChanges++
        }
        lastDirection = currentDir

        // Update bounds
        minX = min(minX, curr.X)
        maxX = max(maxX, curr.X)
        minY = min(minY, curr.Y)
        maxY = max(maxY, curr.Y)
    }

    duration := positions[len(positions)-1].T - positions[0].T
    avgVelocity := totalDistance / (float64(duration) / 1000.0)

    return MovementMetrics{
        Duration:         duration,
        TotalDistance:    totalDistance,
        DirectionChanges: directionChanges,
        AvgVelocity:      avgVelocity,
        Bounds: Rect{
            Top:    minY,
            Left:   minX,
            Width:  maxX - minX,
            Height: maxY - minY,
        },
    }
}

func (d *ThrashDetector) getDirection(dx, dy float64) Direction {
    // Determine dominant direction
    if math.Abs(dx) > math.Abs(dy) {
        if dx > 0 {
            return DirRight
        }
        return DirLeft
    }
    if dy > 0 {
        return DirDown
    }
    return DirUp
}
```

---

## 6. U-Turn Detection

### Definition

A **U-turn** occurs when a user navigates to a page and quickly returns to the previous page.

### Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `max_time_away_ms` | 10000 | Max time on intermediate page |

### Algorithm

```go
type UTurnDetector struct {
    maxTimeAwayMs int64
    pageHistory   map[string][]PageVisit // session -> page history
    mu            sync.Mutex
}

type PageVisit struct {
    URL       string
    Path      string
    Timestamp int64
}

func (d *UTurnDetector) ProcessPageView(session string, pageView PageViewEvent) *UTurnInsight {
    d.mu.Lock()
    defer d.mu.Unlock()

    history, exists := d.pageHistory[session]
    if !exists {
        d.pageHistory[session] = []PageVisit{{
            URL:       pageView.URL,
            Path:      pageView.Path,
            Timestamp: pageView.Timestamp,
        }}
        return nil
    }

    // Add current page
    current := PageVisit{
        URL:       pageView.URL,
        Path:      pageView.Path,
        Timestamp: pageView.Timestamp,
    }

    // Check for U-turn: current page matches 2-pages-ago
    if len(history) >= 2 {
        twoPagesAgo := history[len(history)-2]
        lastPage := history[len(history)-1]

        // Same page as 2 pages ago
        if current.Path == twoPagesAgo.Path {
            timeAway := current.Timestamp - lastPage.Timestamp

            // Quick return
            if timeAway <= d.maxTimeAwayMs {
                // Don't store this as new history entry (it's a return)
                return &UTurnInsight{
                    FromURL:    twoPagesAgo.URL,
                    ToURL:      lastPage.URL,
                    ReturnURL:  current.URL,
                    TimeAwayMs: timeAway,
                }
            }
        }
    }

    // Add to history (keep last 10 pages)
    history = append(history, current)
    if len(history) > 10 {
        history = history[1:]
    }
    d.pageHistory[session] = history

    return nil
}
```

---

## 7. Slow Page Detection

### Definition

**Slow page** identifies pages with poor loading performance.

### Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `threshold_ms` | 3000 | LCP threshold |
| `ttfb_threshold_ms` | 800 | TTFB threshold |

### Algorithm

```go
type SlowPageDetector struct {
    lcpThresholdMs  int64
    ttfbThresholdMs int64
}

func (d *SlowPageDetector) ProcessPerformance(session string, perf PerformanceEvent) *SlowPageInsight {
    var reasons []string
    var slowestMetric int64

    // Check LCP
    if perf.LCP != nil && *perf.LCP > d.lcpThresholdMs {
        reasons = append(reasons, "lcp")
        if *perf.LCP > slowestMetric {
            slowestMetric = *perf.LCP
        }
    }

    // Check TTFB
    if perf.TTFB != nil && *perf.TTFB > d.ttfbThresholdMs {
        reasons = append(reasons, "ttfb")
    }

    // Check total load time
    if perf.LoadComplete != nil && *perf.LoadComplete > d.lcpThresholdMs {
        reasons = append(reasons, "load")
        if *perf.LoadComplete > slowestMetric {
            slowestMetric = *perf.LoadComplete
        }
    }

    if len(reasons) > 0 {
        return &SlowPageInsight{
            URL:         perf.URL,
            LoadTimeMs:  slowestMetric,
            ThresholdMs: d.lcpThresholdMs,
            Reasons:     reasons,
        }
    }

    return nil
}
```

---

## 8. Processor Integration

### Combined Insight Processor

```go
type InsightProcessor struct {
    rageClick     *RageClickDetector
    deadClick     *DeadClickDetector
    errorClick    *ErrorClickDetector
    thrashCursor  *ThrashDetector
    uTurn         *UTurnDetector
    slowPage      *SlowPageDetector

    insightsChan  chan<- Insight
    alertsChan    chan<- Alert

    config        InsightConfig
}

func (p *InsightProcessor) ProcessEvent(session string, event Event) {
    var insights []Insight

    switch event.Type {
    case "click":
        // Rage click detection
        if insight := p.rageClick.ProcessClick(session, event); insight != nil {
            insights = append(insights, insight)
        }

        // Dead click detection
        p.deadClick.ProcessClick(session, event)

        // Error click tracking
        p.errorClick.ProcessClick(session, event)

    case "js_error":
        // Error click detection
        if insight := p.errorClick.ProcessError(session, event); insight != nil {
            insights = append(insights, insight)
        }

    case "mouse_move":
        // Thrashed cursor detection
        if insight := p.thrashCursor.ProcessMouseMove(session, event); insight != nil {
            insights = append(insights, insight)
        }

    case "page_view":
        // U-turn detection
        if insight := p.uTurn.ProcessPageView(session, event); insight != nil {
            insights = append(insights, insight)
        }

        // Dead click resolution
        p.deadClick.ProcessEvent(session, event)

    case "dom_mutation":
        // Dead click resolution
        p.deadClick.ProcessEvent(session, event)

    case "page_load", "web_vitals":
        // Slow page detection
        if insight := p.slowPage.ProcessPerformance(session, event); insight != nil {
            insights = append(insights, insight)
        }
    }

    // Emit insights
    for _, insight := range insights {
        p.insightsChan <- insight

        // Check alert thresholds
        p.checkAlertThresholds(session, insight)
    }
}
```

---

## 9. Configuration

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

## 10. References

- [Event Catalog](./06-event-catalog.md)
- [Data Models](./03-data-models.md)
- [Alerting System](./10-alerting-system.md)
