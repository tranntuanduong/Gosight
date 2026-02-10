# Phase 10: Testing & Optimization

## Mục Tiêu

Đảm bảo chất lượng và performance của hệ thống.

## Prerequisites

- Phase 1-9 hoàn thành

## Tasks

### 10.1 Unit Tests

#### Go Services

**Ingestor Tests:**

```go
// ingestor/internal/validation/validator_test.go
package validation

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

type MockRedis struct {
    mock.Mock
}

func TestValidateAPIKey_Valid(t *testing.T) {
    // Setup
    mockRedis := new(MockRedis)
    mockDB := new(MockDB)

    validator := NewValidator(mockRedis, mockDB)

    mockDB.On("QueryRow", mock.Anything, mock.Anything).Return(
        &Row{ProjectID: "project-123"},
        nil,
    )
    mockRedis.On("Set", mock.Anything, mock.Anything, mock.Anything).Return(nil)

    // Test
    projectID, err := validator.ValidateAPIKey(context.Background(), "gs_valid_key_12345")

    // Assert
    assert.NoError(t, err)
    assert.Equal(t, "project-123", projectID)
}

func TestValidateAPIKey_Invalid(t *testing.T) {
    validator := NewValidator(nil, nil)

    _, err := validator.ValidateAPIKey(context.Background(), "invalid")

    assert.Error(t, err)
}

func TestCheckRateLimit_WithinLimit(t *testing.T) {
    mockRedis := new(MockRedis)
    mockRedis.On("Incr", mock.Anything, mock.Anything).Return(int64(50), nil)
    mockRedis.On("Expire", mock.Anything, mock.Anything, mock.Anything).Return(nil)

    validator := NewValidator(mockRedis, nil)
    validator.cfg = &Config{RateLimit: RateLimitConfig{RequestsPerSecond: 1000}}

    allowed := validator.CheckRateLimit("project-123")

    assert.True(t, allowed)
}

func TestCheckRateLimit_ExceedsLimit(t *testing.T) {
    mockRedis := new(MockRedis)
    mockRedis.On("Incr", mock.Anything, mock.Anything).Return(int64(1001), nil)

    validator := NewValidator(mockRedis, nil)
    validator.cfg = &Config{RateLimit: RateLimitConfig{RequestsPerSecond: 1000}}

    allowed := validator.CheckRateLimit("project-123")

    assert.False(t, allowed)
}
```

**Insight Detector Tests:**

```go
// processor/internal/insights/rage_click_test.go
package insights

import (
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/assert"
)

func TestRageClickDetector_DetectsRageClick(t *testing.T) {
    detector := NewRageClickDetector(nil, RageClickConfig{
        MinClicks:    5,
        TimeWindowMs: 2000,
        RadiusPx:     50,
    })

    sessionID := uuid.New()
    baseTime := time.Now().UnixMilli()

    // Simulate 5 rapid clicks in same area
    for i := 0; i < 5; i++ {
        event := Event{
            EventID:   uuid.New().String(),
            Type:      "click",
            Timestamp: baseTime + int64(i*100), // 100ms apart
            ClickX:    100 + i*5,                // Within 50px radius
            ClickY:    200 + i*5,
        }

        insight := detector.ProcessClick(sessionID, event)

        // Should detect on 5th click
        if i == 4 {
            assert.NotNil(t, insight)
            assert.Equal(t, "rage_click", insight.Type)
            assert.Equal(t, 5, insight.Details["click_count"])
        }
    }
}

func TestRageClickDetector_NoDetectionWhenTooSlow(t *testing.T) {
    detector := NewRageClickDetector(nil, RageClickConfig{
        MinClicks:    5,
        TimeWindowMs: 2000,
        RadiusPx:     50,
    })

    sessionID := uuid.New()
    baseTime := time.Now().UnixMilli()

    // Clicks too slow (500ms apart = 2.5 seconds total)
    for i := 0; i < 5; i++ {
        event := Event{
            EventID:   uuid.New().String(),
            Timestamp: baseTime + int64(i*500),
            ClickX:    100,
            ClickY:    200,
        }

        insight := detector.ProcessClick(sessionID, event)
        assert.Nil(t, insight)
    }
}

func TestRageClickDetector_NoDetectionWhenTooFarApart(t *testing.T) {
    detector := NewRageClickDetector(nil, RageClickConfig{
        MinClicks:    5,
        TimeWindowMs: 2000,
        RadiusPx:     50,
    })

    sessionID := uuid.New()
    baseTime := time.Now().UnixMilli()

    // Clicks too far apart spatially
    for i := 0; i < 5; i++ {
        event := Event{
            EventID:   uuid.New().String(),
            Timestamp: baseTime + int64(i*100),
            ClickX:    100 + i*100, // 100px apart
            ClickY:    200,
        }

        insight := detector.ProcessClick(sessionID, event)
        assert.Nil(t, insight)
    }
}
```

#### SDK Tests

**`sdk/src/__tests__/session.test.ts`**

```typescript
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { SessionManager } from '../core/session';

describe('SessionManager', () => {
  let sessionManager: SessionManager;

  beforeEach(() => {
    sessionManager = new SessionManager(30 * 60 * 1000);
    sessionStorage.clear();
  });

  it('should create new session on start', () => {
    sessionManager.start();

    expect(sessionManager.getId()).toBeDefined();
    expect(sessionManager.getId()).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
    );
  });

  it('should resume session within timeout', () => {
    sessionManager.start();
    const sessionId = sessionManager.getId();

    // Create new manager (simulating page reload)
    const newManager = new SessionManager(30 * 60 * 1000);
    newManager.start();

    expect(newManager.getId()).toBe(sessionId);
  });

  it('should create new session after timeout', () => {
    sessionManager.start();
    const sessionId = sessionManager.getId();

    // Simulate timeout
    vi.advanceTimersByTime(31 * 60 * 1000);

    const newManager = new SessionManager(30 * 60 * 1000);
    newManager.start();

    expect(newManager.getId()).not.toBe(sessionId);
  });

  it('should track user ID', () => {
    sessionManager.start();
    sessionManager.setUserId('user-123');

    expect(sessionManager.getUserId()).toBe('user-123');
  });
});
```

**`sdk/src/__tests__/transport.test.ts`**

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { Transport } from '../transport/transport';

describe('Transport', () => {
  let transport: Transport;

  beforeEach(() => {
    transport = new Transport('https://ingest.gosight.io', 'gs_test_key');
    vi.stubGlobal('navigator', { sendBeacon: vi.fn(() => true) });
  });

  it('should send events via sendBeacon', async () => {
    const events = [{ eventId: '1', type: 'click', timestamp: Date.now() }];
    const session = { sessionId: 'sess-1', startedAt: Date.now(), device: {} };

    await transport.sendEvents(events, session);

    expect(navigator.sendBeacon).toHaveBeenCalled();
  });

  it('should fallback to fetch when sendBeacon fails', async () => {
    vi.stubGlobal('navigator', { sendBeacon: vi.fn(() => false) });
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({ ok: true })));

    const events = [{ eventId: '1', type: 'click', timestamp: Date.now() }];
    const session = { sessionId: 'sess-1', startedAt: Date.now(), device: {} };

    await transport.sendEvents(events, session);

    expect(fetch).toHaveBeenCalled();
  });
});
```

---

### 10.2 Integration Tests

**`tests/integration/ingest_test.go`**

```go
package integration

import (
    "bytes"
    "encoding/json"
    "net/http"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/suite"
)

type IngestIntegrationSuite struct {
    suite.Suite
    ingestURL string
    apiKey    string
}

func (s *IngestIntegrationSuite) SetupSuite() {
    s.ingestURL = "http://localhost:8080"
    s.apiKey = "gs_test_key_12345"

    // Wait for services
    for i := 0; i < 30; i++ {
        resp, err := http.Get(s.ingestURL + "/health")
        if err == nil && resp.StatusCode == 200 {
            return
        }
        time.Sleep(time.Second)
    }
    s.T().Fatal("Ingestor not ready")
}

func (s *IngestIntegrationSuite) TestSendEvents_Success() {
    payload := map[string]interface{}{
        "project_key": s.apiKey,
        "session_id":  "550e8400-e29b-41d4-a716-446655440000",
        "events": []map[string]interface{}{
            {
                "type":      "page_view",
                "timestamp": time.Now().UnixMilli(),
                "url":       "https://example.com/",
                "path":      "/",
            },
        },
    }

    body, _ := json.Marshal(payload)
    resp, err := http.Post(s.ingestURL+"/v1/events", "application/json", bytes.NewReader(body))

    assert.NoError(s.T(), err)
    assert.Equal(s.T(), http.StatusOK, resp.StatusCode)

    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    assert.True(s.T(), result["success"].(bool))
    assert.Equal(s.T(), float64(1), result["accepted_count"])
}

func (s *IngestIntegrationSuite) TestSendEvents_InvalidAPIKey() {
    payload := map[string]interface{}{
        "project_key": "invalid_key",
        "session_id":  "550e8400-e29b-41d4-a716-446655440000",
        "events":      []map[string]interface{}{},
    }

    body, _ := json.Marshal(payload)
    resp, err := http.Post(s.ingestURL+"/v1/events", "application/json", bytes.NewReader(body))

    assert.NoError(s.T(), err)
    assert.Equal(s.T(), http.StatusUnauthorized, resp.StatusCode)
}

func TestIngestIntegration(t *testing.T) {
    suite.Run(t, new(IngestIntegrationSuite))
}
```

---

### 10.3 E2E Tests

**`tests/e2e/dashboard.spec.ts`**

```typescript
import { test, expect } from '@playwright/test';

test.describe('Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    // Login
    await page.goto('/login');
    await page.fill('input[name="email"]', 'test@example.com');
    await page.fill('input[name="password"]', 'password123');
    await page.click('button[type="submit"]');

    // Wait for redirect
    await expect(page).toHaveURL(/\/projects/);
  });

  test('should display overview metrics', async ({ page }) => {
    await page.goto('/project-123');

    // Check stats are displayed
    await expect(page.locator('text=Sessions')).toBeVisible();
    await expect(page.locator('text=Page Views')).toBeVisible();
    await expect(page.locator('text=Bounce Rate')).toBeVisible();
  });

  test('should filter sessions', async ({ page }) => {
    await page.goto('/project-123/sessions');

    // Apply error filter
    await page.click('button:has-text("Has Error")');

    // Check URL updated
    await expect(page).toHaveURL(/has_error=true/);

    // Check table shows filtered results
    await expect(page.locator('tbody tr').first().locator('text=Error')).toBeVisible();
  });

  test('should play session replay', async ({ page }) => {
    await page.goto('/project-123/sessions/session-123');

    // Wait for replay to load
    await expect(page.locator('.replay-player')).toBeVisible();

    // Click play
    await page.click('button[aria-label="Play"]');

    // Check timeline is progressing
    const initialTime = await page.locator('.timeline-time').textContent();
    await page.waitForTimeout(2000);
    const newTime = await page.locator('.timeline-time').textContent();

    expect(newTime).not.toBe(initialTime);
  });
});
```

---

### 10.4 Load Testing

**`tests/load/k6/ingest.js`**

```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const latency = new Trend('latency');

export const options = {
  stages: [
    { duration: '1m', target: 100 },   // Ramp up to 100 VUs
    { duration: '3m', target: 100 },   // Stay at 100 VUs
    { duration: '1m', target: 500 },   // Ramp up to 500 VUs
    { duration: '3m', target: 500 },   // Stay at 500 VUs
    { duration: '1m', target: 1000 },  // Ramp up to 1000 VUs
    { duration: '5m', target: 1000 },  // Stay at 1000 VUs
    { duration: '2m', target: 0 },     // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'],  // 95% requests under 500ms
    errors: ['rate<0.01'],              // Error rate under 1%
  },
};

const BASE_URL = __ENV.INGEST_URL || 'http://localhost:8080';
const API_KEY = __ENV.API_KEY || 'gs_test_key';

export default function () {
  const payload = JSON.stringify({
    project_key: API_KEY,
    session_id: `${__VU}-${__ITER}`,
    events: generateEvents(10),
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
    },
  };

  const start = Date.now();
  const res = http.post(`${BASE_URL}/v1/events`, payload, params);
  latency.add(Date.now() - start);

  const success = check(res, {
    'status is 200': (r) => r.status === 200,
    'success is true': (r) => r.json('success') === true,
  });

  errorRate.add(!success);

  sleep(0.1);
}

function generateEvents(count) {
  const events = [];
  const now = Date.now();

  for (let i = 0; i < count; i++) {
    events.push({
      type: 'click',
      timestamp: now + i * 100,
      url: 'https://example.com/page',
      path: '/page',
      payload: {
        x: Math.floor(Math.random() * 1920),
        y: Math.floor(Math.random() * 1080),
        target: {
          tag: 'button',
          selector: '.btn-primary',
        },
      },
    });
  }

  return events;
}
```

**Run Load Test:**

```bash
# Install k6
brew install k6

# Run test
k6 run tests/load/k6/ingest.js

# Run with environment
k6 run -e INGEST_URL=https://ingest.gosight.io -e API_KEY=gs_xxx tests/load/k6/ingest.js
```

---

### 10.5 Performance Optimization

#### ClickHouse Query Optimization

```sql
-- Add materialized views for common queries

-- Sessions per day
CREATE MATERIALIZED VIEW gosight.sessions_daily_mv
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(date)
ORDER BY (project_id, date)
AS SELECT
    project_id,
    toDate(started_at) AS date,
    count() AS session_count,
    countIf(is_bounce = 1) AS bounce_count,
    sum(duration_ms) AS total_duration,
    countIf(has_error = 1) AS error_sessions
FROM gosight.sessions
GROUP BY project_id, date;

-- Page views aggregation
CREATE MATERIALIZED VIEW gosight.pageviews_hourly_mv
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(date)
ORDER BY (project_id, date, hour, path)
AS SELECT
    project_id,
    toDate(timestamp) AS date,
    toHour(timestamp) AS hour,
    path,
    count() AS view_count,
    uniqExact(session_id) AS unique_sessions
FROM gosight.events
WHERE event_type = 'page_view'
GROUP BY project_id, date, hour, path;

-- Explain query for debugging
EXPLAIN PIPELINE
SELECT count()
FROM gosight.events
WHERE project_id = 'xxx' AND timestamp > now() - interval 7 day;
```

#### Caching Strategy

```go
// api/internal/service/analytics_cached.go
type CachedAnalyticsService struct {
    service *AnalyticsService
    redis   *redis.Client
}

func (s *CachedAnalyticsService) GetOverview(ctx context.Context, projectID string, start, end time.Time) (*OverviewData, error) {
    // Build cache key
    cacheKey := fmt.Sprintf("overview:%s:%s:%s",
        projectID,
        start.Format("2006-01-02"),
        end.Format("2006-01-02"),
    )

    // Try cache
    cached, err := s.redis.Get(ctx, cacheKey).Result()
    if err == nil {
        var data OverviewData
        json.Unmarshal([]byte(cached), &data)
        return &data, nil
    }

    // Query database
    data, err := s.service.GetOverview(ctx, projectID, start, end)
    if err != nil {
        return nil, err
    }

    // Cache for 5 minutes (recent data) or 1 hour (historical)
    ttl := 5 * time.Minute
    if end.Before(time.Now().AddDate(0, 0, -1)) {
        ttl = time.Hour
    }

    cacheData, _ := json.Marshal(data)
    s.redis.Set(ctx, cacheKey, cacheData, ttl)

    return data, nil
}
```

---

### 10.6 Security Audit

**Checklist:**

- [ ] Input validation (SQL injection, XSS)
- [ ] API key hashing (SHA-256)
- [ ] JWT token expiry
- [ ] Rate limiting per project
- [ ] CORS configuration
- [ ] HTTPS enforcement
- [ ] Sensitive data masking
- [ ] No secrets in logs

---

## Checklist

- [ ] Unit tests cho Ingestor
- [ ] Unit tests cho Processors
- [ ] Unit tests cho API
- [ ] Unit tests cho SDK
- [ ] Integration tests
- [ ] E2E tests cho Dashboard
- [ ] Load testing (k6)
- [ ] Performance optimization
- [ ] Security audit
- [ ] CI/CD pipeline

## Test Commands

```bash
# Go tests
go test ./... -v -cover

# SDK tests
cd sdk && npm test

# Dashboard tests
cd dashboard && npm test

# E2E tests
cd dashboard && npx playwright test

# Load test
k6 run tests/load/k6/ingest.js
```
