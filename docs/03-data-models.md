# GoSight - Data Models

## 1. Overview

This document defines all data schemas used in GoSight:

1. **Protobuf Schemas** - Wire format for SDK ↔ Ingestor communication
2. **ClickHouse Schemas** - Analytics data storage
3. **PostgreSQL Schemas** - Metadata and configuration
4. **Redis Key Schemas** - Caching and real-time state

---

## 2. Protobuf Schemas

### 2.1 Package Structure

```
proto/
└── gosight/
    ├── common.proto      # Shared types
    ├── events.proto      # Event definitions
    ├── replay.proto      # Replay-specific types
    └── ingest.proto      # Service definitions
```

### 2.2 Common Types (`common.proto`)

```protobuf
syntax = "proto3";

package gosight;

option go_package = "github.com/gosight/proto/gosight";

// Timestamp in milliseconds since Unix epoch
message Timestamp {
  int64 millis = 1;
}

// 2D position
message Position {
  int32 x = 1;
  int32 y = 2;
}

// Bounding rectangle
message Rect {
  int32 top = 1;
  int32 left = 2;
  int32 width = 3;
  int32 height = 4;
}

// Target element information
message TargetElement {
  string tag = 1;                         // "button", "a", "div"
  optional string id = 2;                 // Element ID
  repeated string classes = 3;            // CSS classes
  optional string text = 4;               // Inner text (truncated)
  optional string href = 5;               // For links
  string selector = 6;                    // Unique CSS selector
  map<string, string> attributes = 7;     // data-* attributes
  optional Rect rect = 8;                 // Bounding box
}

// Device and browser information
message DeviceInfo {
  string user_agent = 1;
  string browser = 2;
  string browser_version = 3;
  string os = 4;
  string os_version = 5;
  DeviceType device_type = 6;
  int32 screen_width = 7;
  int32 screen_height = 8;
  int32 viewport_width = 9;
  int32 viewport_height = 10;
  float device_pixel_ratio = 11;
  string language = 12;
  string timezone = 13;
}

enum DeviceType {
  DEVICE_TYPE_UNSPECIFIED = 0;
  DEVICE_TYPE_DESKTOP = 1;
  DEVICE_TYPE_TABLET = 2;
  DEVICE_TYPE_MOBILE = 3;
}

// Page context
message PageContext {
  string url = 1;
  string path = 2;
  string title = 3;
  string referrer = 4;
  map<string, string> query_params = 5;
}

// Geo location (enriched server-side)
message GeoLocation {
  string ip = 1;           // May be masked
  string country = 2;
  string country_code = 3;
  string region = 4;
  string city = 5;
}
```

### 2.3 Event Types (`events.proto`)

```protobuf
syntax = "proto3";

package gosight;

import "gosight/common.proto";

option go_package = "github.com/gosight/proto/gosight";

// Base event wrapper
message Event {
  string event_id = 1;          // UUID
  string session_id = 2;        // UUID
  optional string user_id = 3;  // Custom user ID
  string project_id = 4;        // From API key
  EventType event_type = 5;
  Timestamp timestamp = 6;
  PageContext page = 7;
  DeviceInfo device = 8;
  map<string, string> custom = 9;  // Custom dimensions

  // Event-specific payload (oneof)
  oneof payload {
    SessionStartPayload session_start = 100;
    SessionEndPayload session_end = 101;
    PageViewPayload page_view = 102;
    PageExitPayload page_exit = 103;
    ClickPayload click = 104;
    MouseMovePayload mouse_move = 105;
    ScrollPayload scroll = 106;
    InputChangePayload input_change = 107;
    InputFocusPayload input_focus = 108;
    FormSubmitPayload form_submit = 109;
    JsErrorPayload js_error = 110;
    PerformancePayload performance = 111;
    NetworkRequestPayload network_request = 112;
    CustomEventPayload custom_event = 113;
    IdentifyPayload identify = 114;
    ReplayEventPayload replay = 115;
  }
}

enum EventType {
  EVENT_TYPE_UNSPECIFIED = 0;

  // Session
  EVENT_TYPE_SESSION_START = 1;
  EVENT_TYPE_SESSION_END = 2;

  // Page
  EVENT_TYPE_PAGE_VIEW = 10;
  EVENT_TYPE_PAGE_EXIT = 11;
  EVENT_TYPE_PAGE_VISIBLE = 12;
  EVENT_TYPE_PAGE_HIDDEN = 13;

  // Mouse
  EVENT_TYPE_CLICK = 20;
  EVENT_TYPE_DBLCLICK = 21;
  EVENT_TYPE_CONTEXT_MENU = 22;
  EVENT_TYPE_MOUSE_MOVE = 23;

  // Scroll
  EVENT_TYPE_SCROLL = 30;
  EVENT_TYPE_SCROLL_MILESTONE = 31;

  // Input
  EVENT_TYPE_INPUT_CHANGE = 40;
  EVENT_TYPE_INPUT_FOCUS = 41;
  EVENT_TYPE_INPUT_BLUR = 42;

  // Form
  EVENT_TYPE_FORM_START = 50;
  EVENT_TYPE_FORM_SUBMIT = 51;
  EVENT_TYPE_FORM_ABANDON = 52;
  EVENT_TYPE_FORM_ERROR = 53;

  // Error
  EVENT_TYPE_JS_ERROR = 60;
  EVENT_TYPE_UNHANDLED_REJECTION = 61;
  EVENT_TYPE_RESOURCE_ERROR = 62;
  EVENT_TYPE_CONSOLE_ERROR = 63;

  // Performance
  EVENT_TYPE_PAGE_LOAD = 70;
  EVENT_TYPE_WEB_VITALS = 71;
  EVENT_TYPE_LONG_TASK = 72;

  // Network
  EVENT_TYPE_XHR_REQUEST = 80;
  EVENT_TYPE_FETCH_REQUEST = 81;
  EVENT_TYPE_WEBSOCKET = 82;

  // Media
  EVENT_TYPE_MEDIA_PLAY = 90;
  EVENT_TYPE_MEDIA_PAUSE = 91;
  EVENT_TYPE_MEDIA_COMPLETE = 92;

  // Resize
  EVENT_TYPE_WINDOW_RESIZE = 100;
  EVENT_TYPE_ORIENTATION_CHANGE = 101;

  // Custom
  EVENT_TYPE_CUSTOM = 110;
  EVENT_TYPE_IDENTIFY = 111;
  EVENT_TYPE_GROUP = 112;

  // Replay
  EVENT_TYPE_REPLAY = 120;

  // Insights (server-generated)
  EVENT_TYPE_RAGE_CLICK = 200;
  EVENT_TYPE_DEAD_CLICK = 201;
  EVENT_TYPE_ERROR_CLICK = 202;
  EVENT_TYPE_THRASHED_CURSOR = 203;
}

// ─────────────────────────────────────────────
// Session Events
// ─────────────────────────────────────────────

message SessionStartPayload {
  bool is_new_user = 1;
  string entry_url = 2;
  optional string utm_source = 3;
  optional string utm_medium = 4;
  optional string utm_campaign = 5;
  optional string utm_term = 6;
  optional string utm_content = 7;
  optional string referrer_domain = 8;
}

message SessionEndPayload {
  int64 duration_ms = 1;
  int32 page_count = 2;
  int32 event_count = 3;
  SessionEndReason end_reason = 4;
}

enum SessionEndReason {
  SESSION_END_REASON_UNSPECIFIED = 0;
  SESSION_END_REASON_TIMEOUT = 1;
  SESSION_END_REASON_NAVIGATION = 2;
  SESSION_END_REASON_CLOSE = 3;
}

// ─────────────────────────────────────────────
// Page Events
// ─────────────────────────────────────────────

message PageViewPayload {
  string page_title = 1;
  string page_path = 2;
  optional string page_hash = 3;
}

message PageExitPayload {
  int64 time_on_page_ms = 1;
  int32 max_scroll_depth = 2;
  optional string exit_url = 3;
}

// ─────────────────────────────────────────────
// Mouse Events
// ─────────────────────────────────────────────

message ClickPayload {
  Position position = 1;
  TargetElement target = 2;
  MouseButton button = 3;
}

enum MouseButton {
  MOUSE_BUTTON_LEFT = 0;
  MOUSE_BUTTON_MIDDLE = 1;
  MOUSE_BUTTON_RIGHT = 2;
}

message MouseMovePayload {
  repeated MousePosition positions = 1;
}

message MousePosition {
  int32 x = 1;
  int32 y = 2;
  int64 timestamp = 3;  // Relative to event timestamp
}

// ─────────────────────────────────────────────
// Scroll Events
// ─────────────────────────────────────────────

message ScrollPayload {
  int32 scroll_top = 1;
  int32 scroll_depth_px = 2;
  int32 scroll_depth_percent = 3;
  int32 page_height = 4;
  ScrollDirection direction = 5;
  optional float velocity = 6;
}

enum ScrollDirection {
  SCROLL_DIRECTION_DOWN = 0;
  SCROLL_DIRECTION_UP = 1;
}

// ─────────────────────────────────────────────
// Input Events
// ─────────────────────────────────────────────

message InputChangePayload {
  TargetElement target = 1;
  int32 value_length = 2;
  bool is_masked = 3;
}

message InputFocusPayload {
  TargetElement target = 1;
}

// ─────────────────────────────────────────────
// Form Events
// ─────────────────────────────────────────────

message FormSubmitPayload {
  optional string form_id = 1;
  optional string form_name = 2;
  bool success = 3;
  int64 time_to_complete_ms = 4;
  int32 field_count = 5;
}

// ─────────────────────────────────────────────
// Error Events
// ─────────────────────────────────────────────

message JsErrorPayload {
  string message = 1;
  optional string stack = 2;
  optional string filename = 3;
  optional int32 lineno = 4;
  optional int32 colno = 5;
  string error_type = 6;
}

// ─────────────────────────────────────────────
// Performance Events
// ─────────────────────────────────────────────

message PerformancePayload {
  optional int32 ttfb = 1;
  optional int32 dom_content_loaded = 2;
  optional int32 load_complete = 3;
  optional int32 first_paint = 4;
  optional int32 first_contentful_paint = 5;
  optional int32 largest_contentful_paint = 6;
  optional float cumulative_layout_shift = 7;
  optional int32 first_input_delay = 8;
  optional int32 interaction_to_next_paint = 9;
}

// ─────────────────────────────────────────────
// Network Events
// ─────────────────────────────────────────────

message NetworkRequestPayload {
  string method = 1;
  string url = 2;
  int32 status = 3;
  int64 duration_ms = 4;
  optional int64 request_size = 5;
  optional int64 response_size = 6;
  NetworkRequestType request_type = 7;
}

enum NetworkRequestType {
  NETWORK_REQUEST_TYPE_XHR = 0;
  NETWORK_REQUEST_TYPE_FETCH = 1;
  NETWORK_REQUEST_TYPE_WEBSOCKET = 2;
}

// ─────────────────────────────────────────────
// Custom Events
// ─────────────────────────────────────────────

message CustomEventPayload {
  string name = 1;
  map<string, string> properties = 2;
  optional string category = 3;
}

message IdentifyPayload {
  string user_id = 1;
  map<string, string> traits = 2;
}

// ─────────────────────────────────────────────
// Replay Events
// ─────────────────────────────────────────────

message ReplayEventPayload {
  int32 rrweb_type = 1;
  bytes data = 2;  // Compressed rrweb event data
}
```

### 2.4 Ingest Service (`ingest.proto`)

```protobuf
syntax = "proto3";

package gosight;

import "gosight/events.proto";

option go_package = "github.com/gosight/proto/gosight";

service IngestService {
  // Stream events from SDK to server
  rpc SendEvents(stream EventBatch) returns (IngestResponse);

  // Single batch send (for HTTP fallback)
  rpc SendBatch(EventBatch) returns (IngestResponse);
}

message EventBatch {
  string project_key = 1;
  repeated Event events = 2;
  int64 sent_at = 3;
  string sdk_version = 4;
}

message IngestResponse {
  bool success = 1;
  optional string error = 2;
  int32 events_accepted = 3;
  int32 events_rejected = 4;
}
```

---

## 3. ClickHouse Schemas

### 3.1 Events Table

```sql
-- Main events table
CREATE TABLE events (
    -- Identifiers
    event_id          UUID,
    project_id        String,
    session_id        UUID,
    user_id           String DEFAULT '',

    -- Event info
    event_type        LowCardinality(String),
    timestamp         DateTime64(3),

    -- Page context
    url               String,
    path              String,
    title             String DEFAULT '',
    referrer          String DEFAULT '',

    -- Click fields (nullable for non-click events)
    click_x           Nullable(Int16),
    click_y           Nullable(Int16),
    target_selector   Nullable(String),
    target_tag        Nullable(LowCardinality(String)),
    target_text       Nullable(String),

    -- Scroll fields
    scroll_depth      Nullable(UInt8),
    scroll_direction  Nullable(LowCardinality(String)),

    -- Error fields
    error_message     Nullable(String),
    error_stack       Nullable(String),
    error_type        Nullable(LowCardinality(String)),

    -- Performance fields
    lcp               Nullable(UInt16),
    fid               Nullable(UInt16),
    cls               Nullable(Float32),
    ttfb              Nullable(UInt16),

    -- Full payload as JSON (for flexibility)
    payload           String DEFAULT '{}',

    -- Device info
    browser           LowCardinality(String),
    browser_version   String DEFAULT '',
    os                LowCardinality(String),
    os_version        String DEFAULT '',
    device_type       LowCardinality(String),
    screen_width      UInt16 DEFAULT 0,
    screen_height     UInt16 DEFAULT 0,
    viewport_width    UInt16 DEFAULT 0,
    viewport_height   UInt16 DEFAULT 0,

    -- Geo info
    ip                String DEFAULT '',
    country           LowCardinality(String) DEFAULT '',
    country_code      LowCardinality(String) DEFAULT '',
    region            String DEFAULT '',
    city              String DEFAULT '',

    -- Custom dimensions
    custom            String DEFAULT '{}',

    -- Partitioning
    event_date        Date DEFAULT toDate(timestamp),

    -- Ingestion metadata
    ingested_at       DateTime64(3) DEFAULT now64(3)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(event_date)
ORDER BY (project_id, session_id, timestamp, event_id)
TTL event_date + INTERVAL 1 YEAR
SETTINGS index_granularity = 8192;

-- Index for common queries
ALTER TABLE events ADD INDEX idx_event_type (event_type) TYPE set(100) GRANULARITY 4;
ALTER TABLE events ADD INDEX idx_path (path) TYPE bloom_filter(0.01) GRANULARITY 4;
ALTER TABLE events ADD INDEX idx_country (country) TYPE set(200) GRANULARITY 4;
```

### 3.2 Sessions Table

```sql
-- Aggregated session data
CREATE TABLE sessions (
    session_id        UUID,
    project_id        String,
    user_id           String DEFAULT '',

    -- Timing
    started_at        DateTime64(3),
    ended_at          DateTime64(3),
    duration_ms       UInt32 DEFAULT 0,

    -- Navigation
    entry_url         String,
    entry_path        String,
    exit_url          String DEFAULT '',
    exit_path         String DEFAULT '',

    -- Counts
    page_count        UInt16 DEFAULT 0,
    event_count       UInt32 DEFAULT 0,
    click_count       UInt16 DEFAULT 0,
    error_count       UInt16 DEFAULT 0,

    -- Engagement
    max_scroll_depth  UInt8 DEFAULT 0,

    -- Flags
    has_error         UInt8 DEFAULT 0,
    has_rage_click    UInt8 DEFAULT 0,
    has_dead_click    UInt8 DEFAULT 0,
    is_bounce         UInt8 DEFAULT 0,

    -- UTM
    utm_source        String DEFAULT '',
    utm_medium        String DEFAULT '',
    utm_campaign      String DEFAULT '',

    -- Device
    browser           LowCardinality(String),
    os                LowCardinality(String),
    device_type       LowCardinality(String),

    -- Geo
    country           LowCardinality(String) DEFAULT '',
    country_code      LowCardinality(String) DEFAULT '',
    city              String DEFAULT '',

    -- Partitioning
    session_date      Date DEFAULT toDate(started_at)
)
ENGINE = ReplacingMergeTree(ended_at)
PARTITION BY toYYYYMM(session_date)
ORDER BY (project_id, session_id)
TTL session_date + INTERVAL 1 YEAR;
```

### 3.3 Replay Chunks Table

```sql
-- Session replay data
CREATE TABLE replay_chunks (
    session_id        UUID,
    project_id        String,
    chunk_index       UInt16,

    -- Timing
    timestamp_start   DateTime64(3),
    timestamp_end     DateTime64(3),

    -- Data
    data              String,  -- Compressed rrweb JSON
    data_size         UInt32,  -- Uncompressed size
    event_count       UInt16,  -- Number of rrweb events in chunk

    -- Metadata
    has_snapshot      UInt8 DEFAULT 0,  -- Contains full snapshot

    -- Partitioning
    chunk_date        Date DEFAULT toDate(timestamp_start)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(chunk_date)
ORDER BY (project_id, session_id, chunk_index)
TTL chunk_date + INTERVAL 90 DAY;  -- Move to cold storage after 90 days
```

### 3.4 Insights Table

```sql
-- Derived insights
CREATE TABLE insights (
    insight_id        UUID,
    project_id        String,
    session_id        UUID,

    insight_type      LowCardinality(String),  -- rage_click, dead_click, etc.
    timestamp         DateTime64(3),

    -- Location
    url               String,
    path              String,

    -- Position (for click-based insights)
    x                 Nullable(Int16),
    y                 Nullable(Int16),

    -- Details
    target_selector   Nullable(String),
    details           String DEFAULT '{}',  -- JSON with type-specific data

    -- Related events
    related_event_ids Array(UUID),

    -- Partitioning
    insight_date      Date DEFAULT toDate(timestamp)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(insight_date)
ORDER BY (project_id, insight_type, timestamp)
TTL insight_date + INTERVAL 1 YEAR;
```

### 3.5 Materialized Views

```sql
-- Hourly aggregates for dashboard
CREATE MATERIALIZED VIEW events_hourly_mv
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(hour)
ORDER BY (project_id, hour, event_type, path)
AS SELECT
    project_id,
    toStartOfHour(timestamp) AS hour,
    event_type,
    path,
    count() AS event_count,
    uniqExact(session_id) AS session_count,
    uniqExact(user_id) AS user_count
FROM events
GROUP BY project_id, hour, event_type, path;

-- Daily session stats
CREATE MATERIALIZED VIEW sessions_daily_mv
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(day)
ORDER BY (project_id, day, country, device_type)
AS SELECT
    project_id,
    toDate(started_at) AS day,
    country,
    device_type,
    count() AS session_count,
    uniqExact(user_id) AS user_count,
    sum(duration_ms) AS total_duration_ms,
    sum(page_count) AS total_pages,
    sum(has_error) AS error_sessions,
    sum(has_rage_click) AS rage_click_sessions,
    sum(is_bounce) AS bounce_sessions
FROM sessions
GROUP BY project_id, day, country, device_type;

-- Page performance stats
CREATE MATERIALIZED VIEW page_performance_mv
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(day)
ORDER BY (project_id, day, path)
AS SELECT
    project_id,
    toDate(timestamp) AS day,
    path,
    count() AS view_count,
    avgState(lcp) AS avg_lcp,
    avgState(fid) AS avg_fid,
    avgState(cls) AS avg_cls,
    avgState(ttfb) AS avg_ttfb,
    quantileState(0.75)(lcp) AS p75_lcp,
    quantileState(0.95)(lcp) AS p95_lcp
FROM events
WHERE event_type = 'page_load' AND lcp IS NOT NULL
GROUP BY project_id, day, path;
```

### 3.6 Analytics-Only Aggregated Tables

Các bảng này cho phép heatmaps và analytics mà **không cần Session Replay**, giảm 99% storage.

```sql
-- ═══════════════════════════════════════════════════════════════════
-- HEATMAP: Click aggregation cho click heatmaps
-- ═══════════════════════════════════════════════════════════════════
CREATE MATERIALIZED VIEW click_heatmap_mv
ENGINE = SummingMergeTree()
ORDER BY (project_id, path, viewport_width, grid_x, grid_y)
AS SELECT
    project_id,
    path,
    viewport_width,
    intDiv(click_x, 10) AS grid_x,          -- 10px grid cells
    intDiv(click_y, 10) AS grid_y,
    target_selector,
    target_tag,
    count() AS click_count,
    toDate(timestamp) AS date
FROM events
WHERE event_type = 'click' AND click_x IS NOT NULL
GROUP BY project_id, path, viewport_width, grid_x, grid_y,
         target_selector, target_tag, date;

-- ═══════════════════════════════════════════════════════════════════
-- HEATMAP: Element click ranking
-- ═══════════════════════════════════════════════════════════════════
CREATE MATERIALIZED VIEW element_clicks_mv
ENGINE = SummingMergeTree()
ORDER BY (project_id, path, target_selector)
AS SELECT
    project_id,
    path,
    target_selector,
    target_tag,
    target_text,
    count() AS click_count,
    toDate(timestamp) AS date
FROM events
WHERE event_type = 'click' AND target_selector != ''
GROUP BY project_id, path, target_selector, target_tag, target_text, date;

-- ═══════════════════════════════════════════════════════════════════
-- HEATMAP: Scroll depth distribution
-- ═══════════════════════════════════════════════════════════════════
CREATE MATERIALIZED VIEW scroll_depth_mv
ENGINE = SummingMergeTree()
ORDER BY (project_id, path, date)
AS SELECT
    project_id,
    path,
    toDate(timestamp) AS date,
    countIf(scroll_depth >= 25) AS reached_25,
    countIf(scroll_depth >= 50) AS reached_50,
    countIf(scroll_depth >= 75) AS reached_75,
    countIf(scroll_depth >= 90) AS reached_90,
    countIf(scroll_depth >= 100) AS reached_100,
    count() AS total_sessions
FROM events
WHERE event_type = 'page_exit' AND scroll_depth IS NOT NULL
GROUP BY project_id, path, date;

-- ═══════════════════════════════════════════════════════════════════
-- ANALYTICS: Time on page statistics
-- ═══════════════════════════════════════════════════════════════════
CREATE MATERIALIZED VIEW page_time_mv
ENGINE = AggregatingMergeTree()
ORDER BY (project_id, path, date)
AS SELECT
    project_id,
    path,
    toDate(timestamp) AS date,
    count() AS view_count,
    avgState(time_on_page_ms) AS avg_time_on_page,
    maxState(time_on_page_ms) AS max_time_on_page,
    minState(time_on_page_ms) AS min_time_on_page,
    quantileState(0.5)(time_on_page_ms) AS median_time_on_page,
    quantileState(0.75)(time_on_page_ms) AS p75_time_on_page,
    sumState(time_on_page_ms) AS total_time_on_page
FROM events
WHERE event_type = 'page_exit' AND time_on_page_ms > 0
GROUP BY project_id, path, date;
```

**Query examples:**

```sql
-- Top clicked elements trên 1 trang
SELECT
    target_selector,
    target_text,
    sum(click_count) AS total_clicks
FROM element_clicks_mv
WHERE project_id = 'xxx' AND path = '/pricing'
GROUP BY target_selector, target_text
ORDER BY total_clicks DESC
LIMIT 10;

-- Scroll depth analysis
SELECT
    path,
    sum(reached_25) * 100 / sum(total_sessions) AS pct_25,
    sum(reached_50) * 100 / sum(total_sessions) AS pct_50,
    sum(reached_75) * 100 / sum(total_sessions) AS pct_75,
    sum(reached_100) * 100 / sum(total_sessions) AS pct_100
FROM scroll_depth_mv
WHERE project_id = 'xxx'
GROUP BY path
ORDER BY pct_100 DESC;

-- Pages với time on page cao nhất
SELECT
    path,
    avgMerge(avg_time_on_page) / 1000 AS avg_seconds,
    sum(view_count) AS total_views
FROM page_time_mv
WHERE project_id = 'xxx'
GROUP BY path
ORDER BY avg_seconds DESC
LIMIT 10;
```

### 3.7 Data Retention Configuration

```sql
-- Replay: 7 ngày (hoặc tắt hoàn toàn trong Analytics-Only mode)
ALTER TABLE replay_chunks
MODIFY TTL chunk_date + INTERVAL 7 DAY DELETE;

-- Raw events: 30-90 ngày
ALTER TABLE events
MODIFY TTL event_date + INTERVAL 90 DAY DELETE;

-- Sessions: 1 năm (nhỏ, giữ lâu để phân tích trend)
ALTER TABLE sessions
MODIFY TTL session_date + INTERVAL 365 DAY DELETE;

-- Aggregated views: Giữ vĩnh viễn (rất nhỏ)
-- Không cần TTL cho materialized views
```

**Storage comparison:**

| Data Type | 10K sessions/day | 30-day retention |
|-----------|------------------|------------------|
| replay_chunks | 10-50 GB/day | 300-1500 GB |
| events | 100-500 MB/day | 3-15 GB |
| sessions | 5-10 MB/day | 150-300 MB |
| aggregated MVs | 1-5 MB/day | 30-150 MB |

---

## 4. PostgreSQL Schemas

### 4.1 Users

```sql
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           VARCHAR(255) UNIQUE NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,
    name            VARCHAR(255),
    avatar_url      VARCHAR(500),

    -- Status
    is_active       BOOLEAN DEFAULT TRUE,
    email_verified  BOOLEAN DEFAULT FALSE,

    -- Timestamps
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    last_login_at   TIMESTAMPTZ
);

CREATE INDEX idx_users_email ON users(email);
```

### 4.2 Projects

```sql
CREATE TABLE projects (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) NOT NULL,
    slug            VARCHAR(100) UNIQUE NOT NULL,

    -- Domain settings
    allowed_domains TEXT[] DEFAULT '{}',

    -- Settings (JSON)
    settings        JSONB DEFAULT '{
        "events": {
            "session": true,
            "page": true,
            "mouse": true,
            "scroll": true,
            "input": true,
            "form": true,
            "error": true,
            "performance": true,
            "replay": true,
            "network": false,
            "media": false
        },
        "privacy": {
            "maskAllInputs": true,
            "maskSelectors": [],
            "blockSelectors": [],
            "blockUrls": [],
            "anonymizeIp": false
        },
        "sampling": {
            "sessionSampleRate": 100,
            "replaySampleRate": 100
        }
    }',

    -- Timestamps
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_projects_slug ON projects(slug);
```

### 4.3 API Keys

```sql
CREATE TABLE api_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    -- Key info
    key_hash        VARCHAR(64) UNIQUE NOT NULL,  -- SHA-256 hash
    key_prefix      VARCHAR(12) NOT NULL,         -- "gs_abc123..." for display
    name            VARCHAR(255) DEFAULT 'Default',

    -- Permissions
    permissions     TEXT[] DEFAULT '{write}',  -- read, write

    -- Rate limiting
    rate_limit      INTEGER DEFAULT 1000,  -- requests per second

    -- Status
    is_active       BOOLEAN DEFAULT TRUE,

    -- Usage tracking
    last_used_at    TIMESTAMPTZ,
    request_count   BIGINT DEFAULT 0,

    -- Timestamps
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    expires_at      TIMESTAMPTZ  -- NULL = never expires
);

CREATE INDEX idx_api_keys_project ON api_keys(project_id);
CREATE INDEX idx_api_keys_hash ON api_keys(key_hash);
```

### 4.4 Project Members

```sql
CREATE TYPE member_role AS ENUM ('owner', 'admin', 'member', 'viewer');

CREATE TABLE project_members (
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            member_role NOT NULL DEFAULT 'member',

    -- Timestamps
    created_at      TIMESTAMPTZ DEFAULT NOW(),

    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX idx_project_members_user ON project_members(user_id);
```

### 4.5 Alert Rules

```sql
CREATE TYPE alert_condition_type AS ENUM (
    'threshold',      -- metric > value
    'anomaly',        -- statistical anomaly
    'absence'         -- no data
);

CREATE TABLE alert_rules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    -- Rule info
    name            VARCHAR(255) NOT NULL,
    description     TEXT,

    -- Condition
    condition_type  alert_condition_type NOT NULL,
    condition       JSONB NOT NULL,
    /*
    Example conditions:
    {
        "metric": "rage_clicks",
        "operator": ">",
        "threshold": 10,
        "window": "5m",
        "groupBy": ["path"]
    }
    */

    -- Notification
    channels        JSONB NOT NULL DEFAULT '[]',
    /*
    Example:
    [
        {"type": "telegram", "chat_id": "-123456789"},
        {"type": "webhook", "url": "https://..."}
    ]
    */

    -- Behavior
    cooldown_mins   INTEGER DEFAULT 15,
    is_active       BOOLEAN DEFAULT TRUE,

    -- Timestamps
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    last_triggered  TIMESTAMPTZ
);

CREATE INDEX idx_alert_rules_project ON alert_rules(project_id);
```

### 4.6 Alert History

```sql
CREATE TABLE alert_history (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id         UUID NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    project_id      UUID NOT NULL,

    -- Alert details
    triggered_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at     TIMESTAMPTZ,

    -- Context
    metric_value    FLOAT NOT NULL,
    threshold       FLOAT NOT NULL,
    context         JSONB DEFAULT '{}',

    -- Notification status
    notification_sent BOOLEAN DEFAULT FALSE,
    notification_error TEXT
);

CREATE INDEX idx_alert_history_rule ON alert_history(rule_id);
CREATE INDEX idx_alert_history_project ON alert_history(project_id, triggered_at);
```

### 4.7 Saved Filters

```sql
CREATE TABLE saved_filters (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    name            VARCHAR(255) NOT NULL,
    filter_type     VARCHAR(50) NOT NULL,  -- sessions, events, errors
    filter          JSONB NOT NULL,

    is_shared       BOOLEAN DEFAULT FALSE,

    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_saved_filters_project ON saved_filters(project_id);
```

---

## 5. Redis Key Schemas

### 5.1 Session State

```
Key:     session:{session_id}:state
Type:    Hash
TTL:     30 minutes (sliding)
Fields:
  - project_id: string
  - user_id: string
  - started_at: timestamp
  - last_event_at: timestamp
  - page_count: int
  - event_count: int
  - current_url: string
```

### 5.2 Rate Limiting

```
Key:     ratelimit:{project_id}:{minute}
Type:    Counter
TTL:     2 minutes
Value:   Request count for this minute
```

### 5.3 API Key Cache

```
Key:     apikey:{key_hash}
Type:    Hash
TTL:     5 minutes
Fields:
  - project_id: string
  - permissions: string (comma-separated)
  - rate_limit: int
  - is_active: bool
```

### 5.4 Real-time Active Sessions

```
Key:     realtime:{project_id}:sessions
Type:    HyperLogLog
TTL:     1 hour
Value:   Approximate count of unique session_ids
```

### 5.5 Rage Click Detection

```
Key:     clicks:{session_id}:{grid_x}:{grid_y}
Type:    List
TTL:     10 seconds
Value:   [timestamp1, timestamp2, timestamp3, ...]

Grid calculation:
  grid_x = floor(click_x / 50)  # 50px grid cells
  grid_y = floor(click_y / 50)
```

### 5.6 Page View Counter (Real-time)

```
Key:     pageviews:{project_id}:{path_hash}:{minute}
Type:    Counter
TTL:     5 minutes
Value:   Page view count
```

---

## 6. Entity Relationships

```
┌─────────────┐       ┌─────────────┐       ┌─────────────┐
│    User     │───────│   Project   │───────│   API Key   │
│             │  N:M  │   Member    │  1:N  │             │
└─────────────┘       └─────────────┘       └─────────────┘
                             │
                             │ 1:N
                             ▼
                      ┌─────────────┐
                      │   Project   │
                      └─────────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
              ▼              ▼              ▼
       ┌───────────┐  ┌───────────┐  ┌───────────┐
       │  Session  │  │Alert Rule │  │  Filter   │
       └───────────┘  └───────────┘  └───────────┘
              │              │
              │              │ 1:N
              │              ▼
              │       ┌───────────┐
              │       │  Alert    │
              │       │  History  │
              │       └───────────┘
              │
              │ 1:N
              ▼
       ┌───────────┐
       │   Event   │
       └───────────┘
              │
              │ 1:N
              ▼
       ┌───────────┐
       │  Insight  │
       └───────────┘
```

---

## 7. References

- [System Architecture](./02-system-architecture.md)
- [Event Catalog](./06-event-catalog.md)
- [API Specification](./05-api-specification.md)
