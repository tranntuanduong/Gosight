# GoSight - API Specification

## 1. Overview

The GoSight API provides REST endpoints for the dashboard application to access analytics data, manage projects, and configure settings.

### Base URL

```
Production: https://api.gosight.io/v1
Self-hosted: https://your-domain.com/api/v1
```

### Authentication

All API requests require authentication via JWT token or API key.

**JWT Token (Dashboard):**
```http
Authorization: Bearer <jwt_token>
```

**API Key (Programmatic):**
```http
X-GoSight-Key: gs_api_xxxxx
```

### Response Format

All responses follow this structure:

```json
{
  "success": true,
  "data": { ... },
  "meta": {
    "page": 1,
    "per_page": 50,
    "total": 1000,
    "total_pages": 20
  }
}
```

Error responses:

```json
{
  "success": false,
  "error": {
    "code": "INVALID_REQUEST",
    "message": "Human readable message",
    "details": { ... }
  }
}
```

---

## 2. Authentication

### POST /auth/login

Login with email and password.

**Request:**
```json
{
  "email": "user@example.com",
  "password": "secret123"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_in": 3600,
    "token_type": "Bearer",
    "user": {
      "id": "uuid",
      "email": "user@example.com",
      "name": "John Doe"
    }
  }
}
```

---

### POST /auth/refresh

Refresh access token.

**Request:**
```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_in": 3600
  }
}
```

---

### POST /auth/logout

Invalidate tokens.

**Response:**
```json
{
  "success": true
}
```

---

## 3. Projects

### GET /projects

List all projects for the authenticated user.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "proj_abc123",
      "name": "My Website",
      "slug": "my-website",
      "allowed_domains": ["example.com", "*.example.com"],
      "created_at": "2024-01-15T10:30:00Z",
      "role": "owner",
      "stats": {
        "sessions_today": 1234,
        "events_today": 56789
      }
    }
  ]
}
```

---

### POST /projects

Create a new project.

**Request:**
```json
{
  "name": "My New Project",
  "allowed_domains": ["newsite.com"]
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "proj_xyz789",
    "name": "My New Project",
    "slug": "my-new-project",
    "api_key": "gs_live_abc123xyz789...",
    "allowed_domains": ["newsite.com"],
    "created_at": "2024-01-20T15:00:00Z"
  }
}
```

---

### GET /projects/:id

Get project details.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "proj_abc123",
    "name": "My Website",
    "slug": "my-website",
    "allowed_domains": ["example.com"],
    "settings": {
      "events": {
        "session": true,
        "page": true,
        "mouse": true,
        "replay": true
      },
      "privacy": {
        "maskAllInputs": true,
        "blockSelectors": [".sensitive"],
        "anonymizeIp": false
      },
      "sampling": {
        "sessionSampleRate": 100,
        "replaySampleRate": 50
      }
    },
    "created_at": "2024-01-15T10:30:00Z"
  }
}
```

---

### PATCH /projects/:id

Update project settings.

**Request:**
```json
{
  "name": "Updated Name",
  "settings": {
    "privacy": {
      "anonymizeIp": true
    }
  }
}
```

---

### DELETE /projects/:id

Delete a project.

**Response:**
```json
{
  "success": true
}
```

---

## 4. API Keys

### GET /projects/:id/api-keys

List API keys for a project.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "key_abc123",
      "name": "Production",
      "key_prefix": "gs_live_abc1...",
      "permissions": ["write"],
      "is_active": true,
      "last_used_at": "2024-01-20T12:00:00Z",
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

---

### POST /projects/:id/api-keys

Create a new API key.

**Request:**
```json
{
  "name": "Staging Environment",
  "permissions": ["write"]
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "key_xyz789",
    "name": "Staging Environment",
    "key": "gs_live_xyz789abc...",
    "key_prefix": "gs_live_xyz7...",
    "permissions": ["write"]
  }
}
```

> **Note:** The full key is only returned once at creation time.

---

### DELETE /projects/:id/api-keys/:keyId

Revoke an API key.

---

## 5. Analytics - Overview

### GET /projects/:id/overview

Get dashboard overview metrics.

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `start_date` | string | 7 days ago | ISO date |
| `end_date` | string | today | ISO date |
| `timezone` | string | UTC | IANA timezone |

**Response:**
```json
{
  "success": true,
  "data": {
    "period": {
      "start": "2024-01-13",
      "end": "2024-01-20"
    },
    "summary": {
      "sessions": 12500,
      "sessions_change": 12.5,
      "users": 8500,
      "users_change": 8.2,
      "pageviews": 45000,
      "pageviews_change": 15.3,
      "avg_session_duration": 245,
      "bounce_rate": 42.5
    },
    "charts": {
      "sessions_over_time": [
        { "date": "2024-01-13", "value": 1500 },
        { "date": "2024-01-14", "value": 1800 }
      ],
      "pageviews_over_time": [...],
      "top_pages": [
        { "path": "/", "views": 15000, "avg_time": 45 },
        { "path": "/pricing", "views": 8000, "avg_time": 120 }
      ],
      "top_referrers": [
        { "source": "google", "sessions": 5000 },
        { "source": "direct", "sessions": 3000 }
      ],
      "devices": {
        "desktop": 65,
        "mobile": 30,
        "tablet": 5
      },
      "countries": [
        { "country": "US", "sessions": 4000 },
        { "country": "VN", "sessions": 2500 }
      ]
    },
    "insights": {
      "rage_clicks": 45,
      "dead_clicks": 120,
      "error_sessions": 89,
      "frustrated_sessions": 156
    }
  }
}
```

---

## 6. Sessions

### GET /projects/:id/sessions

List sessions with filtering.

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `start_date` | string | Filter start date |
| `end_date` | string | Filter end date |
| `page` | number | Page number |
| `per_page` | number | Items per page (max 100) |
| `sort` | string | Sort field (started_at, duration, events) |
| `order` | string | asc or desc |
| `has_error` | boolean | Filter by error presence |
| `has_rage_click` | boolean | Filter by rage clicks |
| `has_replay` | boolean | Filter by replay availability |
| `country` | string | Filter by country code |
| `device_type` | string | desktop, mobile, tablet |
| `browser` | string | Browser name |
| `user_id` | string | Filter by user ID |
| `path` | string | Filter by visited path |
| `min_duration` | number | Minimum duration (seconds) |
| `max_duration` | number | Maximum duration (seconds) |

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "sess_abc123",
      "user_id": "user_456",
      "started_at": "2024-01-20T10:30:00Z",
      "ended_at": "2024-01-20T10:35:00Z",
      "duration_ms": 300000,
      "entry_url": "https://example.com/",
      "exit_url": "https://example.com/checkout",
      "page_count": 5,
      "event_count": 127,
      "has_error": false,
      "has_rage_click": true,
      "has_dead_click": false,
      "has_replay": true,
      "device": {
        "browser": "Chrome",
        "os": "Windows",
        "device_type": "desktop"
      },
      "location": {
        "country": "US",
        "city": "New York"
      }
    }
  ],
  "meta": {
    "page": 1,
    "per_page": 50,
    "total": 12500
  }
}
```

---

### GET /projects/:id/sessions/:sessionId

Get session details.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "sess_abc123",
    "user_id": "user_456",
    "started_at": "2024-01-20T10:30:00Z",
    "ended_at": "2024-01-20T10:35:00Z",
    "duration_ms": 300000,
    "pages": [
      {
        "url": "https://example.com/",
        "path": "/",
        "title": "Home",
        "entered_at": "2024-01-20T10:30:00Z",
        "time_on_page_ms": 45000,
        "scroll_depth": 75
      }
    ],
    "device": {
      "browser": "Chrome",
      "browser_version": "120.0",
      "os": "Windows",
      "os_version": "11",
      "device_type": "desktop",
      "screen_resolution": "1920x1080",
      "viewport": "1200x800"
    },
    "location": {
      "country": "US",
      "country_code": "US",
      "region": "New York",
      "city": "New York"
    },
    "utm": {
      "source": "google",
      "medium": "cpc",
      "campaign": "winter_sale"
    },
    "stats": {
      "event_count": 127,
      "click_count": 45,
      "error_count": 0,
      "rage_click_count": 1,
      "dead_click_count": 3
    },
    "insights": [
      {
        "type": "rage_click",
        "timestamp": "2024-01-20T10:32:15Z",
        "url": "/checkout",
        "target": "#submit-btn"
      }
    ],
    "user": {
      "id": "user_456",
      "traits": {
        "email": "user@example.com",
        "name": "John Doe",
        "plan": "premium"
      }
    }
  }
}
```

---

### GET /projects/:id/sessions/:sessionId/events

Get events for a session.

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `event_types` | string | Comma-separated types |
| `page` | number | Page number |
| `per_page` | number | Items per page |

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "evt_123",
      "type": "click",
      "timestamp": "2024-01-20T10:30:15Z",
      "url": "https://example.com/",
      "payload": {
        "x": 450,
        "y": 320,
        "target": {
          "tag": "button",
          "text": "Sign Up",
          "selector": "#signup-btn"
        }
      }
    },
    {
      "id": "evt_124",
      "type": "page_view",
      "timestamp": "2024-01-20T10:30:45Z",
      "url": "https://example.com/pricing",
      "payload": {
        "title": "Pricing",
        "referrer": "https://example.com/"
      }
    }
  ]
}
```

---

### GET /projects/:id/sessions/:sessionId/replay

Get session replay data.

**Response:**
```json
{
  "success": true,
  "data": {
    "session_id": "sess_abc123",
    "duration_ms": 300000,
    "started_at": "2024-01-20T10:30:00Z",
    "chunks": [
      {
        "index": 0,
        "timestamp_start": "2024-01-20T10:30:00Z",
        "timestamp_end": "2024-01-20T10:31:00Z",
        "url": "/api/v1/projects/proj_abc/sessions/sess_abc/replay/chunks/0"
      },
      {
        "index": 1,
        "timestamp_start": "2024-01-20T10:31:00Z",
        "timestamp_end": "2024-01-20T10:32:00Z",
        "url": "/api/v1/projects/proj_abc/sessions/sess_abc/replay/chunks/1"
      }
    ],
    "events_timeline": [
      {
        "timestamp": "2024-01-20T10:30:15Z",
        "type": "click",
        "label": "Click on 'Sign Up'"
      },
      {
        "timestamp": "2024-01-20T10:32:15Z",
        "type": "rage_click",
        "label": "Rage click on '#submit-btn'"
      }
    ]
  }
}
```

---

### GET /projects/:id/sessions/:sessionId/replay/chunks/:index

Get a specific replay chunk (binary data).

**Response Headers:**
```http
Content-Type: application/json
Content-Encoding: gzip
```

**Response Body:** Compressed rrweb events JSON

---

## 7. Errors

### GET /projects/:id/errors

List JavaScript errors.

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `start_date` | string | Filter start date |
| `end_date` | string | Filter end date |
| `grouped` | boolean | Group similar errors |
| `status` | string | open, resolved, ignored |
| `page` | number | Page number |

**Response (grouped=true):**
```json
{
  "success": true,
  "data": [
    {
      "id": "err_group_123",
      "message": "Cannot read property 'map' of undefined",
      "error_type": "TypeError",
      "filename": "https://example.com/js/app.js",
      "lineno": 142,
      "colno": 5,
      "count": 156,
      "session_count": 89,
      "first_seen": "2024-01-15T10:00:00Z",
      "last_seen": "2024-01-20T12:30:00Z",
      "status": "open",
      "browsers": {
        "Chrome": 120,
        "Safari": 30,
        "Firefox": 6
      }
    }
  ]
}
```

---

### GET /projects/:id/errors/:errorId

Get error details.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "err_group_123",
    "message": "Cannot read property 'map' of undefined",
    "error_type": "TypeError",
    "filename": "https://example.com/js/app.js",
    "lineno": 142,
    "colno": 5,
    "stack": "TypeError: Cannot read property 'map' of undefined\n    at render (app.js:142:5)\n    at ...",
    "count": 156,
    "session_count": 89,
    "status": "open",
    "stats": {
      "occurrences_over_time": [
        { "date": "2024-01-19", "count": 45 },
        { "date": "2024-01-20", "count": 32 }
      ],
      "browsers": {
        "Chrome": 120,
        "Safari": 30,
        "Firefox": 6
      },
      "pages": [
        { "path": "/checkout", "count": 100 },
        { "path": "/cart", "count": 56 }
      ]
    },
    "sample_sessions": [
      {
        "session_id": "sess_abc123",
        "timestamp": "2024-01-20T10:30:15Z",
        "url": "https://example.com/checkout"
      }
    ]
  }
}
```

---

### PATCH /projects/:id/errors/:errorId

Update error status.

**Request:**
```json
{
  "status": "resolved"
}
```

---

## 8. Insights

### GET /projects/:id/insights

Get UX insights (rage clicks, dead clicks, etc.).

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `start_date` | string | Filter start date |
| `end_date` | string | Filter end date |
| `type` | string | rage_click, dead_click, error_click, thrashed_cursor |
| `path` | string | Filter by page path |
| `page` | number | Page number |

**Response:**
```json
{
  "success": true,
  "data": {
    "summary": {
      "rage_clicks": 145,
      "dead_clicks": 320,
      "error_clicks": 45,
      "thrashed_cursors": 28
    },
    "insights": [
      {
        "id": "insight_123",
        "type": "rage_click",
        "timestamp": "2024-01-20T10:32:15Z",
        "session_id": "sess_abc123",
        "url": "https://example.com/checkout",
        "path": "/checkout",
        "target_selector": "#submit-btn",
        "details": {
          "click_count": 7,
          "time_window_ms": 2000
        }
      }
    ],
    "hotspots": [
      {
        "path": "/checkout",
        "selector": "#submit-btn",
        "rage_click_count": 45,
        "dead_click_count": 12
      }
    ]
  }
}
```

---

## 9. Heatmaps

### GET /projects/:id/heatmaps

Get heatmap data for a page.

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `path` | string | Page path (required) |
| `start_date` | string | Filter start date |
| `end_date` | string | Filter end date |
| `device_type` | string | desktop, mobile, tablet |
| `type` | string | click, scroll, move |

**Response:**
```json
{
  "success": true,
  "data": {
    "path": "/pricing",
    "total_sessions": 5000,
    "viewport": {
      "width": 1200,
      "height": 800
    },
    "page_height": 3500,
    "click_map": [
      { "x": 450, "y": 320, "count": 1500, "selector": "#cta-btn" },
      { "x": 600, "y": 450, "count": 800, "selector": ".plan-card" }
    ],
    "scroll_map": [
      { "depth_percent": 25, "reached_count": 4500 },
      { "depth_percent": 50, "reached_count": 3200 },
      { "depth_percent": 75, "reached_count": 1800 },
      { "depth_percent": 100, "reached_count": 500 }
    ],
    "attention_map": [
      { "y_start": 0, "y_end": 800, "avg_time_ms": 5000 },
      { "y_start": 800, "y_end": 1600, "avg_time_ms": 8000 }
    ]
  }
}
```

---

## 10. Alerts

### GET /projects/:id/alerts

List alert rules.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "alert_123",
      "name": "High Rage Clicks",
      "description": "Alert when rage clicks exceed threshold",
      "condition": {
        "metric": "rage_clicks",
        "operator": ">",
        "threshold": 10,
        "window": "5m"
      },
      "channels": [
        { "type": "telegram", "chat_id": "-123456789" }
      ],
      "is_active": true,
      "cooldown_mins": 15,
      "last_triggered": "2024-01-20T10:00:00Z"
    }
  ]
}
```

---

### POST /projects/:id/alerts

Create an alert rule.

**Request:**
```json
{
  "name": "Error Spike Alert",
  "description": "Alert when JS errors spike",
  "condition": {
    "metric": "js_errors",
    "operator": ">",
    "threshold": 50,
    "window": "10m",
    "group_by": ["path"]
  },
  "channels": [
    { "type": "telegram", "chat_id": "-123456789" }
  ],
  "cooldown_mins": 30
}
```

---

### GET /projects/:id/alerts/history

Get alert history.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "hist_123",
      "rule_id": "alert_123",
      "rule_name": "High Rage Clicks",
      "triggered_at": "2024-01-20T10:00:00Z",
      "resolved_at": "2024-01-20T10:15:00Z",
      "metric_value": 15,
      "threshold": 10,
      "context": {
        "path": "/checkout",
        "affected_sessions": 12
      }
    }
  ]
}
```

---

## 11. Team Members

### GET /projects/:id/members

List project members.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "user_id": "user_123",
      "email": "owner@example.com",
      "name": "John Doe",
      "role": "owner",
      "joined_at": "2024-01-15T10:00:00Z"
    },
    {
      "user_id": "user_456",
      "email": "member@example.com",
      "name": "Jane Smith",
      "role": "member",
      "joined_at": "2024-01-18T14:00:00Z"
    }
  ]
}
```

---

### POST /projects/:id/members

Invite a member.

**Request:**
```json
{
  "email": "newmember@example.com",
  "role": "member"
}
```

---

### PATCH /projects/:id/members/:userId

Update member role.

**Request:**
```json
{
  "role": "admin"
}
```

---

### DELETE /projects/:id/members/:userId

Remove a member.

---

## 12. WebSocket API

### Connection

```javascript
const ws = new WebSocket('wss://api.gosight.io/v1/projects/:id/realtime');
ws.send(JSON.stringify({ type: 'auth', token: 'jwt_token' }));
```

### Messages

**Subscribe to channels:**
```json
{
  "type": "subscribe",
  "channels": ["sessions", "events", "alerts"]
}
```

**Incoming messages:**

```json
// New session
{
  "type": "session.start",
  "data": {
    "session_id": "sess_abc123",
    "user_id": "user_456",
    "url": "https://example.com/",
    "device": "desktop",
    "country": "US"
  }
}

// Real-time event
{
  "type": "event",
  "data": {
    "session_id": "sess_abc123",
    "event_type": "click",
    "timestamp": "2024-01-20T10:30:15Z"
  }
}

// Alert triggered
{
  "type": "alert.triggered",
  "data": {
    "rule_id": "alert_123",
    "rule_name": "High Rage Clicks",
    "metric_value": 15
  }
}

// Live metrics (every 5 seconds)
{
  "type": "metrics",
  "data": {
    "active_sessions": 125,
    "events_per_minute": 450
  }
}
```

---

## 13. Rate Limiting

| Endpoint | Limit |
|----------|-------|
| Auth endpoints | 10 req/min |
| Analytics read | 100 req/min |
| Analytics write | 1000 req/min |
| WebSocket | 1 connection/project |

**Rate limit headers:**
```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1705751400
```

---

## 14. Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `UNAUTHORIZED` | 401 | Invalid or missing token |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource not found |
| `VALIDATION_ERROR` | 400 | Invalid request data |
| `RATE_LIMITED` | 429 | Too many requests |
| `INTERNAL_ERROR` | 500 | Server error |

---

## 15. References

- [Data Models](./03-data-models.md)
- [SDK Specification](./04-sdk-specification.md)
- [System Architecture](./02-system-architecture.md)
