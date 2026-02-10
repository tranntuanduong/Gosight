-- ===========================================
-- GoSight ClickHouse Schema
-- ===========================================

-- Create database
CREATE DATABASE IF NOT EXISTS gosight;

-- ===========================================
-- Events Table
-- Stores all user events (clicks, scrolls, errors...)
-- ===========================================
CREATE TABLE IF NOT EXISTS gosight.events
(
    -- Identifiers
    event_id        UUID DEFAULT generateUUIDv4(),
    project_id      String,
    session_id      String,
    user_id         String,

    -- Event info
    event_type      LowCardinality(String),  -- click, scroll, error, etc.
    timestamp       DateTime64(3),            -- millisecond precision

    -- Page info
    page_url        String,
    page_path       String,
    page_title      String,
    referrer        String,

    -- Device info
    browser         LowCardinality(String),
    browser_version String,
    os              LowCardinality(String),
    os_version      String,
    device_type     LowCardinality(String),  -- desktop, mobile, tablet
    screen_width    UInt16,
    screen_height   UInt16,
    viewport_width  UInt16,
    viewport_height UInt16,

    -- Geo info (enriched by ingestor)
    country         LowCardinality(String),
    city            String,

    -- Event payload (JSON)
    payload         String,

    -- Metadata
    created_at      DateTime DEFAULT now()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (project_id, session_id, timestamp)
TTL timestamp + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- ===========================================
-- Sessions Table
-- Aggregated session data
-- ===========================================
CREATE TABLE IF NOT EXISTS gosight.sessions
(
    session_id      String,
    project_id      String,
    user_id         String,

    -- Timing
    started_at      DateTime64(3),
    ended_at        DateTime64(3),
    duration_ms     UInt64,

    -- Device
    browser         LowCardinality(String),
    os              LowCardinality(String),
    device_type     LowCardinality(String),

    -- Geo
    country         LowCardinality(String),
    city            String,

    -- Metrics
    page_views      UInt32,
    events_count    UInt32,
    errors_count    UInt32,

    -- Entry/Exit
    entry_page      String,
    exit_page       String,

    -- Flags
    has_replay      UInt8,
    is_bounced      UInt8,

    created_at      DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(created_at)
PARTITION BY toYYYYMM(started_at)
ORDER BY (project_id, session_id)
TTL started_at + INTERVAL 90 DAY;

-- ===========================================
-- Page Views Table
-- Optimized for page analytics
-- ===========================================
CREATE TABLE IF NOT EXISTS gosight.page_views
(
    project_id      String,
    session_id      String,
    user_id         String,

    -- Page
    page_url        String,
    page_path       String,
    page_title      String,
    referrer        String,

    -- Timing
    timestamp       DateTime64(3),
    time_on_page_ms UInt64,

    -- Scroll depth
    max_scroll_depth UInt8,  -- 0-100%

    -- Device
    device_type     LowCardinality(String),
    country         LowCardinality(String),

    created_at      DateTime DEFAULT now()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (project_id, page_path, timestamp)
TTL timestamp + INTERVAL 90 DAY;

-- ===========================================
-- Web Vitals Table
-- Core Web Vitals metrics
-- ===========================================
CREATE TABLE IF NOT EXISTS gosight.web_vitals
(
    project_id      String,
    session_id      String,
    page_url        String,
    page_path       String,

    timestamp       DateTime64(3),

    -- Core Web Vitals
    lcp             Nullable(Float64),  -- Largest Contentful Paint (ms)
    fid             Nullable(Float64),  -- First Input Delay (ms)
    cls             Nullable(Float64),  -- Cumulative Layout Shift
    ttfb            Nullable(Float64),  -- Time to First Byte (ms)
    fcp             Nullable(Float64),  -- First Contentful Paint (ms)
    inp             Nullable(Float64),  -- Interaction to Next Paint (ms)

    -- Device context
    device_type     LowCardinality(String),
    country         LowCardinality(String),

    created_at      DateTime DEFAULT now()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (project_id, page_path, timestamp)
TTL timestamp + INTERVAL 90 DAY;

-- ===========================================
-- Replay Chunks Table
-- Session replay data (compressed)
-- ===========================================
CREATE TABLE IF NOT EXISTS gosight.replay_chunks
(
    project_id      String,
    session_id      String,

    chunk_index     UInt32,
    timestamp_start DateTime64(3),
    timestamp_end   DateTime64(3),

    -- Compressed rrweb data
    data            String,  -- Base64 encoded, compressed
    has_full_snapshot UInt8,

    created_at      DateTime DEFAULT now()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp_start)
ORDER BY (project_id, session_id, chunk_index)
TTL timestamp_start + INTERVAL 30 DAY;  -- Replay data expires faster

-- ===========================================
-- Errors Table
-- JavaScript errors
-- ===========================================
CREATE TABLE IF NOT EXISTS gosight.errors
(
    project_id      String,
    session_id      String,

    timestamp       DateTime64(3),

    -- Error details
    error_type      String,
    message         String,
    stack           String,
    source          String,
    line            UInt32,
    col             UInt32,

    -- Page context
    page_url        String,
    page_path       String,

    -- Device
    browser         LowCardinality(String),
    os              LowCardinality(String),

    created_at      DateTime DEFAULT now()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (project_id, message, timestamp)
TTL timestamp + INTERVAL 90 DAY;

-- ===========================================
-- Insights Table
-- Detected issues (rage clicks, dead clicks, etc.)
-- ===========================================
CREATE TABLE IF NOT EXISTS gosight.insights
(
    project_id      String,
    session_id      String,

    insight_type    LowCardinality(String),  -- rage_click, dead_click, error_loop
    severity        LowCardinality(String),  -- low, medium, high

    timestamp       DateTime64(3),

    -- Context
    page_url        String,
    element_selector String,

    -- Details (JSON)
    details         String,

    created_at      DateTime DEFAULT now()
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (project_id, insight_type, timestamp)
TTL timestamp + INTERVAL 90 DAY;
