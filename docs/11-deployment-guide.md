# GoSight - Deployment Guide

## 1. Overview

This guide covers deploying GoSight in a self-hosted environment using Docker Compose (single server) or Kubernetes (production scale).

### Minimum Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| **CPU** | 8 cores | 16 cores |
| **RAM** | 16 GB | 32 GB |
| **Storage** | 200 GB SSD | 500 GB NVMe |
| **Network** | 100 Mbps | 1 Gbps |

---

## 2. Docker Compose Deployment

### 2.1 Directory Structure

```
gosight/
├── docker-compose.yml
├── .env
├── config/
│   ├── ingestor.yaml
│   ├── processor.yaml
│   ├── api.yaml
│   └── traefik/
│       └── traefik.yml
├── data/
│   ├── clickhouse/
│   ├── kafka/
│   ├── postgres/
│   ├── redis/
│   └── minio/
└── scripts/
    ├── init-clickhouse.sql
    └── init-postgres.sql
```

### 2.2 Environment Variables

```bash
# .env

# General
DOMAIN=gosight.example.com
ADMIN_EMAIL=admin@example.com

# PostgreSQL
POSTGRES_USER=gosight
POSTGRES_PASSWORD=your_secure_password
POSTGRES_DB=gosight

# ClickHouse
CLICKHOUSE_USER=default
CLICKHOUSE_PASSWORD=your_secure_password

# Redis
REDIS_PASSWORD=your_secure_password

# Kafka
KAFKA_CLUSTER_ID=your_cluster_id

# MinIO
MINIO_ROOT_USER=gosight
MINIO_ROOT_PASSWORD=your_secure_password

# JWT
JWT_SECRET=your_jwt_secret_min_32_chars

# Telegram (optional)
TELEGRAM_BOT_TOKEN=your_bot_token

# GeoIP (optional)
MAXMIND_LICENSE_KEY=your_license_key
```

### 2.3 Docker Compose File

```yaml
# docker-compose.yml
version: '3.8'

services:
  # ─────────────────────────────────────────────
  # Reverse Proxy
  # ─────────────────────────────────────────────
  traefik:
    image: traefik:v2.10
    container_name: gosight-traefik
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./config/traefik:/etc/traefik
      - ./data/traefik/acme:/acme
    environment:
      - CF_API_EMAIL=${ADMIN_EMAIL}
    labels:
      - "traefik.enable=true"

  # ─────────────────────────────────────────────
  # Message Queue
  # ─────────────────────────────────────────────
  kafka:
    image: bitnami/kafka:3.6
    container_name: gosight-kafka
    restart: unless-stopped
    environment:
      - KAFKA_CFG_NODE_ID=0
      - KAFKA_CFG_PROCESS_ROLES=controller,broker
      - KAFKA_CFG_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093
      - KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
      - KAFKA_CFG_CONTROLLER_QUORUM_VOTERS=0@kafka:9093
      - KAFKA_CFG_CONTROLLER_LISTENER_NAMES=CONTROLLER
      - KAFKA_KRAFT_CLUSTER_ID=${KAFKA_CLUSTER_ID}
      - KAFKA_CFG_AUTO_CREATE_TOPICS_ENABLE=true
    volumes:
      - ./data/kafka:/bitnami/kafka
    healthcheck:
      test: ["CMD", "kafka-topics.sh", "--bootstrap-server", "localhost:9092", "--list"]
      interval: 30s
      timeout: 10s
      retries: 5

  # ─────────────────────────────────────────────
  # Analytics Database
  # ─────────────────────────────────────────────
  clickhouse:
    image: clickhouse/clickhouse-server:23.12
    container_name: gosight-clickhouse
    restart: unless-stopped
    environment:
      - CLICKHOUSE_USER=${CLICKHOUSE_USER}
      - CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD}
      - CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1
    volumes:
      - ./data/clickhouse:/var/lib/clickhouse
      - ./scripts/init-clickhouse.sql:/docker-entrypoint-initdb.d/init.sql
    ulimits:
      nofile:
        soft: 262144
        hard: 262144
    healthcheck:
      test: ["CMD", "clickhouse-client", "--query", "SELECT 1"]
      interval: 30s
      timeout: 10s
      retries: 5

  # ─────────────────────────────────────────────
  # Metadata Database
  # ─────────────────────────────────────────────
  postgres:
    image: postgres:16-alpine
    container_name: gosight-postgres
    restart: unless-stopped
    environment:
      - POSTGRES_USER=${POSTGRES_USER}
      - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
      - POSTGRES_DB=${POSTGRES_DB}
    volumes:
      - ./data/postgres:/var/lib/postgresql/data
      - ./scripts/init-postgres.sql:/docker-entrypoint-initdb.d/init.sql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER}"]
      interval: 30s
      timeout: 10s
      retries: 5

  # ─────────────────────────────────────────────
  # Cache
  # ─────────────────────────────────────────────
  redis:
    image: redis:7-alpine
    container_name: gosight-redis
    restart: unless-stopped
    command: redis-server --requirepass ${REDIS_PASSWORD}
    volumes:
      - ./data/redis:/data
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD}", "ping"]
      interval: 30s
      timeout: 10s
      retries: 5

  # ─────────────────────────────────────────────
  # Object Storage
  # ─────────────────────────────────────────────
  minio:
    image: minio/minio:latest
    container_name: gosight-minio
    restart: unless-stopped
    command: server /data --console-address ":9001"
    environment:
      - MINIO_ROOT_USER=${MINIO_ROOT_USER}
      - MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD}
    volumes:
      - ./data/minio:/data
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 30s
      timeout: 10s
      retries: 5

  # ─────────────────────────────────────────────
  # GoSight Services
  # ─────────────────────────────────────────────
  ingestor:
    image: gosight/ingestor:latest
    container_name: gosight-ingestor
    restart: unless-stopped
    depends_on:
      kafka:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      - CONFIG_PATH=/etc/gosight/ingestor.yaml
      - KAFKA_BROKERS=kafka:9092
      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379
    volumes:
      - ./config/ingestor.yaml:/etc/gosight/ingestor.yaml:ro
      - ./data/geoip:/data/geoip:ro
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.ingestor.rule=Host(`ingest.${DOMAIN}`)"
      - "traefik.http.routers.ingestor.tls=true"
      - "traefik.http.routers.ingestor.tls.certresolver=letsencrypt"
      - "traefik.http.services.ingestor.loadbalancer.server.port=8080"
    deploy:
      replicas: 2

  processor:
    image: gosight/processor:latest
    container_name: gosight-processor
    restart: unless-stopped
    depends_on:
      kafka:
        condition: service_healthy
      clickhouse:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      - CONFIG_PATH=/etc/gosight/processor.yaml
      - KAFKA_BROKERS=kafka:9092
      - CLICKHOUSE_DSN=clickhouse://${CLICKHOUSE_USER}:${CLICKHOUSE_PASSWORD}@clickhouse:9000/gosight
      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379
    volumes:
      - ./config/processor.yaml:/etc/gosight/processor.yaml:ro
    deploy:
      replicas: 2

  api:
    image: gosight/api:latest
    container_name: gosight-api
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
      clickhouse:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      - CONFIG_PATH=/etc/gosight/api.yaml
      - DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}
      - CLICKHOUSE_DSN=clickhouse://${CLICKHOUSE_USER}:${CLICKHOUSE_PASSWORD}@clickhouse:9000/gosight
      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379
      - JWT_SECRET=${JWT_SECRET}
      - TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}
    volumes:
      - ./config/api.yaml:/etc/gosight/api.yaml:ro
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.api.rule=Host(`api.${DOMAIN}`)"
      - "traefik.http.routers.api.tls=true"
      - "traefik.http.routers.api.tls.certresolver=letsencrypt"
      - "traefik.http.services.api.loadbalancer.server.port=8080"
    deploy:
      replicas: 2

  dashboard:
    image: gosight/dashboard:latest
    container_name: gosight-dashboard
    restart: unless-stopped
    environment:
      - NEXT_PUBLIC_API_URL=https://api.${DOMAIN}
      - NEXT_PUBLIC_INGEST_URL=https://ingest.${DOMAIN}
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.dashboard.rule=Host(`${DOMAIN}`)"
      - "traefik.http.routers.dashboard.tls=true"
      - "traefik.http.routers.dashboard.tls.certresolver=letsencrypt"
      - "traefik.http.services.dashboard.loadbalancer.server.port=3000"
    deploy:
      replicas: 2

networks:
  default:
    name: gosight-network

volumes:
  kafka_data:
  clickhouse_data:
  postgres_data:
  redis_data:
  minio_data:
```

### 2.4 Traefik Configuration

```yaml
# config/traefik/traefik.yml
api:
  dashboard: true
  insecure: true

entryPoints:
  web:
    address: ":80"
    http:
      redirections:
        entryPoint:
          to: websecure
          scheme: https
  websecure:
    address: ":443"

providers:
  docker:
    endpoint: "unix:///var/run/docker.sock"
    exposedByDefault: false

certificatesResolvers:
  letsencrypt:
    acme:
      email: ${ADMIN_EMAIL}
      storage: /acme/acme.json
      httpChallenge:
        entryPoint: web
```

### 2.5 ClickHouse Initialization

```sql
-- scripts/init-clickhouse.sql

CREATE DATABASE IF NOT EXISTS gosight;

-- Events table
CREATE TABLE IF NOT EXISTS gosight.events (
    event_id UUID,
    project_id String,
    session_id UUID,
    user_id String DEFAULT '',
    event_type LowCardinality(String),
    timestamp DateTime64(3),
    url String,
    path String,
    title String DEFAULT '',
    referrer String DEFAULT '',
    click_x Nullable(Int16),
    click_y Nullable(Int16),
    target_selector Nullable(String),
    target_tag Nullable(LowCardinality(String)),
    target_text Nullable(String),
    scroll_depth Nullable(UInt8),
    error_message Nullable(String),
    error_stack Nullable(String),
    error_type Nullable(LowCardinality(String)),
    lcp Nullable(UInt16),
    fid Nullable(UInt16),
    cls Nullable(Float32),
    ttfb Nullable(UInt16),
    payload String DEFAULT '{}',
    browser LowCardinality(String),
    browser_version String DEFAULT '',
    os LowCardinality(String),
    os_version String DEFAULT '',
    device_type LowCardinality(String),
    screen_width UInt16 DEFAULT 0,
    screen_height UInt16 DEFAULT 0,
    viewport_width UInt16 DEFAULT 0,
    viewport_height UInt16 DEFAULT 0,
    ip String DEFAULT '',
    country LowCardinality(String) DEFAULT '',
    city String DEFAULT '',
    custom String DEFAULT '{}',
    event_date Date DEFAULT toDate(timestamp),
    ingested_at DateTime64(3) DEFAULT now64(3)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(event_date)
ORDER BY (project_id, session_id, timestamp, event_id)
TTL event_date + INTERVAL 1 YEAR;

-- Sessions table
CREATE TABLE IF NOT EXISTS gosight.sessions (
    session_id UUID,
    project_id String,
    user_id String DEFAULT '',
    started_at DateTime64(3),
    ended_at DateTime64(3),
    duration_ms UInt32 DEFAULT 0,
    entry_url String,
    entry_path String,
    exit_url String DEFAULT '',
    exit_path String DEFAULT '',
    page_count UInt16 DEFAULT 0,
    event_count UInt32 DEFAULT 0,
    click_count UInt16 DEFAULT 0,
    error_count UInt16 DEFAULT 0,
    max_scroll_depth UInt8 DEFAULT 0,
    has_error UInt8 DEFAULT 0,
    has_rage_click UInt8 DEFAULT 0,
    has_dead_click UInt8 DEFAULT 0,
    is_bounce UInt8 DEFAULT 0,
    utm_source String DEFAULT '',
    utm_medium String DEFAULT '',
    utm_campaign String DEFAULT '',
    browser LowCardinality(String),
    os LowCardinality(String),
    device_type LowCardinality(String),
    country LowCardinality(String) DEFAULT '',
    city String DEFAULT '',
    session_date Date DEFAULT toDate(started_at)
)
ENGINE = ReplacingMergeTree(ended_at)
PARTITION BY toYYYYMM(session_date)
ORDER BY (project_id, session_id);

-- Replay chunks table
CREATE TABLE IF NOT EXISTS gosight.replay_chunks (
    session_id UUID,
    project_id String,
    chunk_index UInt16,
    timestamp_start DateTime64(3),
    timestamp_end DateTime64(3),
    data String,
    data_size UInt32,
    event_count UInt16,
    has_full_snapshot UInt8 DEFAULT 0,
    chunk_date Date DEFAULT toDate(timestamp_start)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(chunk_date)
ORDER BY (project_id, session_id, chunk_index)
TTL chunk_date + INTERVAL 90 DAY;

-- Insights table
CREATE TABLE IF NOT EXISTS gosight.insights (
    insight_id UUID,
    project_id String,
    session_id UUID,
    insight_type LowCardinality(String),
    timestamp DateTime64(3),
    url String,
    path String,
    x Nullable(Int16),
    y Nullable(Int16),
    target_selector Nullable(String),
    details String DEFAULT '{}',
    related_event_ids Array(UUID),
    insight_date Date DEFAULT toDate(timestamp)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(insight_date)
ORDER BY (project_id, insight_type, timestamp);
```

### 2.6 PostgreSQL Initialization

```sql
-- scripts/init-postgres.sql

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    avatar_url VARCHAR(500),
    is_active BOOLEAN DEFAULT TRUE,
    email_verified BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    last_login_at TIMESTAMPTZ
);

-- Projects table
CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    allowed_domains TEXT[] DEFAULT '{}',
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- API Keys table
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    key_hash VARCHAR(64) UNIQUE NOT NULL,
    key_prefix VARCHAR(12) NOT NULL,
    name VARCHAR(255) DEFAULT 'Default',
    permissions TEXT[] DEFAULT '{write}',
    rate_limit INTEGER DEFAULT 1000,
    is_active BOOLEAN DEFAULT TRUE,
    last_used_at TIMESTAMPTZ,
    request_count BIGINT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

-- Project members table
CREATE TYPE member_role AS ENUM ('owner', 'admin', 'member', 'viewer');

CREATE TABLE IF NOT EXISTS project_members (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role member_role NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (project_id, user_id)
);

-- Alert rules table
CREATE TYPE alert_condition_type AS ENUM ('threshold', 'anomaly', 'absence');

CREATE TABLE IF NOT EXISTS alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    condition_type alert_condition_type NOT NULL DEFAULT 'threshold',
    condition JSONB NOT NULL,
    channels JSONB NOT NULL DEFAULT '[]',
    cooldown_mins INTEGER DEFAULT 15,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    last_triggered TIMESTAMPTZ
);

-- Alert history table
CREATE TABLE IF NOT EXISTS alert_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    project_id UUID NOT NULL,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    metric_value FLOAT NOT NULL,
    threshold FLOAT NOT NULL,
    context JSONB DEFAULT '{}',
    notification_sent BOOLEAN DEFAULT FALSE,
    notification_error TEXT
);

-- Indexes
CREATE INDEX idx_api_keys_project ON api_keys(project_id);
CREATE INDEX idx_api_keys_hash ON api_keys(key_hash);
CREATE INDEX idx_project_members_user ON project_members(user_id);
CREATE INDEX idx_alert_rules_project ON alert_rules(project_id);
CREATE INDEX idx_alert_history_rule ON alert_history(rule_id);
```

---

## 2.7 Deployment Modes

GoSight hỗ trợ nhiều chế độ triển khai tùy theo nhu cầu:

### Mode 1: Analytics-Only (Lite)

**Đặc điểm:**
- Không lưu Session Replay
- Storage giảm ~99% (chỉ còn ~1-2 GB/ngày cho 100K sessions)
- Vẫn có đầy đủ: Heatmaps, Click analytics, Time on page, UX Insights
- Phù hợp: Budget thấp, chỉ cần analytics cơ bản

**Minimum Requirements (Lite):**

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| **CPU** | 2 cores | 4 cores |
| **RAM** | 4 GB | 8 GB |
| **Storage** | 20 GB SSD | 50 GB SSD |

```yaml
# docker-compose.lite.yml
version: '3.8'

services:
  # ─────────────────────────────────────────────
  # Reverse Proxy
  # ─────────────────────────────────────────────
  traefik:
    image: traefik:v2.10
    container_name: gosight-traefik
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./config/traefik:/etc/traefik
      - ./data/traefik/acme:/acme
    labels:
      - "traefik.enable=true"

  # ─────────────────────────────────────────────
  # Message Queue (Lightweight)
  # ─────────────────────────────────────────────
  kafka:
    image: bitnami/kafka:3.6
    container_name: gosight-kafka
    restart: unless-stopped
    environment:
      - KAFKA_CFG_NODE_ID=0
      - KAFKA_CFG_PROCESS_ROLES=controller,broker
      - KAFKA_CFG_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093
      - KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
      - KAFKA_CFG_CONTROLLER_QUORUM_VOTERS=0@kafka:9093
      - KAFKA_CFG_CONTROLLER_LISTENER_NAMES=CONTROLLER
      - KAFKA_KRAFT_CLUSTER_ID=${KAFKA_CLUSTER_ID}
      - KAFKA_CFG_AUTO_CREATE_TOPICS_ENABLE=true
      # Memory optimization
      - KAFKA_HEAP_OPTS=-Xmx512M -Xms512M
    volumes:
      - ./data/kafka:/bitnami/kafka
    healthcheck:
      test: ["CMD", "kafka-topics.sh", "--bootstrap-server", "localhost:9092", "--list"]
      interval: 30s
      timeout: 10s
      retries: 5

  # ─────────────────────────────────────────────
  # Analytics Database
  # ─────────────────────────────────────────────
  clickhouse:
    image: clickhouse/clickhouse-server:23.12
    container_name: gosight-clickhouse
    restart: unless-stopped
    environment:
      - CLICKHOUSE_USER=${CLICKHOUSE_USER}
      - CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD}
    volumes:
      - ./data/clickhouse:/var/lib/clickhouse
      - ./scripts/init-clickhouse-lite.sql:/docker-entrypoint-initdb.d/init.sql
    ulimits:
      nofile:
        soft: 262144
        hard: 262144
    healthcheck:
      test: ["CMD", "clickhouse-client", "--query", "SELECT 1"]
      interval: 30s
      timeout: 10s
      retries: 5

  # ─────────────────────────────────────────────
  # Metadata Database
  # ─────────────────────────────────────────────
  postgres:
    image: postgres:16-alpine
    container_name: gosight-postgres
    restart: unless-stopped
    environment:
      - POSTGRES_USER=${POSTGRES_USER}
      - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
      - POSTGRES_DB=${POSTGRES_DB}
    volumes:
      - ./data/postgres:/var/lib/postgresql/data
      - ./scripts/init-postgres.sql:/docker-entrypoint-initdb.d/init.sql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER}"]
      interval: 30s
      timeout: 10s
      retries: 5

  # ─────────────────────────────────────────────
  # Cache
  # ─────────────────────────────────────────────
  redis:
    image: redis:7-alpine
    container_name: gosight-redis
    restart: unless-stopped
    command: redis-server --requirepass ${REDIS_PASSWORD} --maxmemory 256mb --maxmemory-policy allkeys-lru
    volumes:
      - ./data/redis:/data
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD}", "ping"]
      interval: 30s
      timeout: 10s
      retries: 5

  # ─────────────────────────────────────────────
  # GoSight Services (Single replica each)
  # ─────────────────────────────────────────────
  ingestor:
    image: gosight/ingestor:latest
    container_name: gosight-ingestor
    restart: unless-stopped
    depends_on:
      kafka:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      - CONFIG_PATH=/etc/gosight/ingestor.yaml
      - KAFKA_BROKERS=kafka:9092
      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379
      - REPLAY_ENABLED=false  # Disable replay processing
    volumes:
      - ./config/ingestor-lite.yaml:/etc/gosight/ingestor.yaml:ro
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.ingestor.rule=Host(`ingest.${DOMAIN}`)"
      - "traefik.http.routers.ingestor.tls=true"
      - "traefik.http.routers.ingestor.tls.certresolver=letsencrypt"
      - "traefik.http.services.ingestor.loadbalancer.server.port=8080"

  processor:
    image: gosight/processor:latest
    container_name: gosight-processor
    restart: unless-stopped
    depends_on:
      kafka:
        condition: service_healthy
      clickhouse:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      - CONFIG_PATH=/etc/gosight/processor.yaml
      - KAFKA_BROKERS=kafka:9092
      - CLICKHOUSE_DSN=clickhouse://${CLICKHOUSE_USER}:${CLICKHOUSE_PASSWORD}@clickhouse:9000/gosight
      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379
      - REPLAY_ENABLED=false  # Skip replay chunk processing
    volumes:
      - ./config/processor-lite.yaml:/etc/gosight/processor.yaml:ro

  api:
    image: gosight/api:latest
    container_name: gosight-api
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
      clickhouse:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      - CONFIG_PATH=/etc/gosight/api.yaml
      - DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}
      - CLICKHOUSE_DSN=clickhouse://${CLICKHOUSE_USER}:${CLICKHOUSE_PASSWORD}@clickhouse:9000/gosight
      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379
      - JWT_SECRET=${JWT_SECRET}
      - TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}
      - REPLAY_ENABLED=false
    volumes:
      - ./config/api-lite.yaml:/etc/gosight/api.yaml:ro
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.api.rule=Host(`api.${DOMAIN}`)"
      - "traefik.http.routers.api.tls=true"
      - "traefik.http.routers.api.tls.certresolver=letsencrypt"
      - "traefik.http.services.api.loadbalancer.server.port=8080"

  dashboard:
    image: gosight/dashboard:latest
    container_name: gosight-dashboard
    restart: unless-stopped
    environment:
      - NEXT_PUBLIC_API_URL=https://api.${DOMAIN}
      - NEXT_PUBLIC_INGEST_URL=https://ingest.${DOMAIN}
      - NEXT_PUBLIC_REPLAY_ENABLED=false
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.dashboard.rule=Host(`${DOMAIN}`)"
      - "traefik.http.routers.dashboard.tls=true"
      - "traefik.http.routers.dashboard.tls.certresolver=letsencrypt"
      - "traefik.http.services.dashboard.loadbalancer.server.port=3000"

networks:
  default:
    name: gosight-network
```

**ClickHouse Initialization for Lite Mode:**

```sql
-- scripts/init-clickhouse-lite.sql
-- Không có replay_chunks table, chỉ có events và aggregated views

CREATE DATABASE IF NOT EXISTS gosight;

-- Events table (shorter TTL)
CREATE TABLE IF NOT EXISTS gosight.events (
    event_id UUID,
    project_id String,
    session_id UUID,
    user_id String DEFAULT '',
    event_type LowCardinality(String),
    timestamp DateTime64(3),
    url String,
    path String,
    title String DEFAULT '',
    referrer String DEFAULT '',
    click_x Nullable(Int16),
    click_y Nullable(Int16),
    target_selector Nullable(String),
    target_tag Nullable(LowCardinality(String)),
    target_text Nullable(String),
    scroll_depth Nullable(UInt8),
    viewport_width UInt16 DEFAULT 0,
    viewport_height UInt16 DEFAULT 0,
    browser LowCardinality(String),
    os LowCardinality(String),
    device_type LowCardinality(String),
    country LowCardinality(String) DEFAULT '',
    event_date Date DEFAULT toDate(timestamp)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(event_date)
ORDER BY (project_id, session_id, timestamp, event_id)
TTL event_date + INTERVAL 90 DAY;

-- Sessions table
CREATE TABLE IF NOT EXISTS gosight.sessions (
    session_id UUID,
    project_id String,
    user_id String DEFAULT '',
    started_at DateTime64(3),
    ended_at DateTime64(3),
    duration_ms UInt32 DEFAULT 0,
    entry_path String,
    exit_path String DEFAULT '',
    page_count UInt16 DEFAULT 0,
    event_count UInt32 DEFAULT 0,
    click_count UInt16 DEFAULT 0,
    max_scroll_depth UInt8 DEFAULT 0,
    has_rage_click UInt8 DEFAULT 0,
    has_dead_click UInt8 DEFAULT 0,
    is_bounce UInt8 DEFAULT 0,
    browser LowCardinality(String),
    os LowCardinality(String),
    device_type LowCardinality(String),
    country LowCardinality(String) DEFAULT '',
    session_date Date DEFAULT toDate(started_at)
)
ENGINE = ReplacingMergeTree(ended_at)
PARTITION BY toYYYYMM(session_date)
ORDER BY (project_id, session_id)
TTL session_date + INTERVAL 90 DAY;

-- =========================================
-- MATERIALIZED VIEWS FOR ANALYTICS
-- =========================================

-- Click Heatmap Aggregation (10x10 pixel grid)
CREATE MATERIALIZED VIEW IF NOT EXISTS gosight.click_heatmap_mv
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(date)
ORDER BY (project_id, path, viewport_width, grid_x, grid_y, target_selector)
TTL date + INTERVAL 90 DAY
AS SELECT
    project_id,
    path,
    viewport_width,
    intDiv(click_x, 10) AS grid_x,
    intDiv(click_y, 10) AS grid_y,
    target_selector,
    target_tag,
    count() AS click_count,
    toDate(timestamp) AS date
FROM gosight.events
WHERE event_type = 'click' AND click_x IS NOT NULL
GROUP BY project_id, path, viewport_width, grid_x, grid_y, target_selector, target_tag, date;

-- Scroll Depth Aggregation
CREATE MATERIALIZED VIEW IF NOT EXISTS gosight.scroll_depth_mv
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(date)
ORDER BY (project_id, path, depth_bucket)
TTL date + INTERVAL 90 DAY
AS SELECT
    project_id,
    path,
    intDiv(scroll_depth, 10) * 10 AS depth_bucket,
    count() AS session_count,
    toDate(timestamp) AS date
FROM gosight.events
WHERE event_type = 'scroll' AND scroll_depth IS NOT NULL
GROUP BY project_id, path, depth_bucket, date;

-- Time on Page Aggregation
CREATE MATERIALIZED VIEW IF NOT EXISTS gosight.time_on_page_mv
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(date)
ORDER BY (project_id, path)
TTL date + INTERVAL 90 DAY
AS SELECT
    project_id,
    path,
    count() AS page_views,
    sum(duration_ms) AS total_duration_ms,
    toDate(started_at) AS date
FROM gosight.sessions
GROUP BY project_id, path, date;
```

---

### Mode 2: Replay Optimized (Short TTL)

**Đặc điểm:**
- Có Session Replay nhưng chỉ lưu 7-30 ngày
- Storage trung bình (~20-50 GB/ngày cho 100K sessions)
- Phù hợp: Cần replay để debug nhưng không cần lưu lâu dài

**TTL Configuration:**

```sql
-- Replay chunks với TTL 7 ngày
CREATE TABLE IF NOT EXISTS gosight.replay_chunks (
    session_id UUID,
    project_id String,
    chunk_index UInt16,
    timestamp_start DateTime64(3),
    timestamp_end DateTime64(3),
    data String CODEC(ZSTD(3)),  -- Higher compression
    data_size UInt32,
    event_count UInt16,
    has_full_snapshot UInt8 DEFAULT 0,
    chunk_date Date DEFAULT toDate(timestamp_start)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(chunk_date)
ORDER BY (project_id, session_id, chunk_index)
TTL chunk_date + INTERVAL 7 DAY;  -- 7 ngày retention

-- Events với TTL ngắn hơn
ALTER TABLE gosight.events MODIFY TTL event_date + INTERVAL 30 DAY;

-- Sessions với TTL ngắn hơn
ALTER TABLE gosight.sessions MODIFY TTL session_date + INTERVAL 30 DAY;
```

**Processor Config cho Replay Optimized:**

```yaml
# config/processor-replay-optimized.yaml
processor:
  batch_size: 5000
  flush_interval: 2s

replay:
  enabled: true
  compression: zstd
  compression_level: 3  # Higher compression (1-22)
  chunk_max_size: 500KB  # Smaller chunks
  chunk_max_events: 500

  # Sampling để giảm size
  sampling:
    enabled: true
    # Chỉ record 50% sessions
    rate: 0.5
    # Hoặc theo điều kiện
    conditions:
      - type: has_error
      - type: has_rage_click
      - type: duration_gt
        value: 60s  # Chỉ record sessions > 60s
```

---

### Comparison Table

| Feature | Full | Analytics-Only | Replay Optimized |
|---------|------|----------------|------------------|
| **Events & Sessions** | ✅ | ✅ | ✅ |
| **Click Heatmaps** | ✅ | ✅ | ✅ |
| **Scroll Depth** | ✅ | ✅ | ✅ |
| **Time on Page** | ✅ | ✅ | ✅ |
| **UX Insights** | ✅ | ✅ | ✅ |
| **Session Replay** | ✅ Full | ❌ Không | ✅ Giới hạn |
| **Replay Retention** | 90 ngày | N/A | 7-30 ngày |
| **Storage/100K sessions/day** | ~100 GB | ~1-2 GB | ~20-50 GB |
| **RAM Required** | 16-32 GB | 4-8 GB | 8-16 GB |
| **CPU Required** | 8-16 cores | 2-4 cores | 4-8 cores |

---

## 3. Deployment Commands

### 3.1 Initial Setup

```bash
# Clone repository
git clone https://github.com/gosight/gosight.git
cd gosight

# Copy environment template
cp .env.example .env

# Edit environment variables
nano .env

# Create data directories
mkdir -p data/{kafka,clickhouse,postgres,redis,minio,geoip}

# Download GeoIP database (optional)
./scripts/download-geoip.sh

# Start infrastructure first
docker-compose up -d kafka clickhouse postgres redis minio

# Wait for services to be healthy
docker-compose ps

# Initialize databases
docker-compose exec clickhouse clickhouse-client < scripts/init-clickhouse.sql

# Start GoSight services
docker-compose up -d ingestor processor api dashboard

# Check logs
docker-compose logs -f
```

### 3.2 Create Admin User

```bash
# Access API container
docker-compose exec api sh

# Create admin user
gosight-cli user create \
  --email admin@example.com \
  --password your_password \
  --role admin
```

### 3.3 Useful Commands

```bash
# View all logs
docker-compose logs -f

# View specific service logs
docker-compose logs -f ingestor

# Restart a service
docker-compose restart api

# Scale services
docker-compose up -d --scale ingestor=3

# Stop all
docker-compose down

# Stop and remove volumes (⚠️ data loss)
docker-compose down -v

# Update images
docker-compose pull
docker-compose up -d
```

---

## 4. Kubernetes Deployment

For production deployments, use Kubernetes with Helm.

### 4.1 Prerequisites

- Kubernetes cluster (1.25+)
- Helm 3.x
- kubectl configured
- Storage class for persistent volumes

### 4.2 Install with Helm

```bash
# Add GoSight Helm repo
helm repo add gosight https://charts.gosight.io
helm repo update

# Create namespace
kubectl create namespace gosight

# Create secrets
kubectl create secret generic gosight-secrets \
  --namespace gosight \
  --from-literal=postgres-password=your_password \
  --from-literal=clickhouse-password=your_password \
  --from-literal=redis-password=your_password \
  --from-literal=jwt-secret=your_jwt_secret

# Install
helm install gosight gosight/gosight \
  --namespace gosight \
  --values values.yaml

# Check status
kubectl get pods -n gosight
```

### 4.3 Helm Values

```yaml
# values.yaml

global:
  domain: gosight.example.com
  storageClass: standard

# Ingress
ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  tls:
    enabled: true

# Services
ingestor:
  replicas: 3
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: 1000m
      memory: 1Gi
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilization: 70

processor:
  replicas: 2
  resources:
    requests:
      cpu: 1000m
      memory: 1Gi
    limits:
      cpu: 2000m
      memory: 2Gi

api:
  replicas: 2
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: 1000m
      memory: 1Gi

dashboard:
  replicas: 2
  resources:
    requests:
      cpu: 200m
      memory: 256Mi
    limits:
      cpu: 500m
      memory: 512Mi

# Infrastructure
kafka:
  replicas: 3
  storage: 50Gi
  resources:
    requests:
      cpu: 1000m
      memory: 2Gi

clickhouse:
  replicas: 1  # Or use cluster mode
  storage: 200Gi
  resources:
    requests:
      cpu: 4000m
      memory: 8Gi

postgres:
  storage: 10Gi
  resources:
    requests:
      cpu: 500m
      memory: 1Gi

redis:
  storage: 5Gi
  resources:
    requests:
      cpu: 200m
      memory: 512Mi
```

---

## 5. Monitoring

### 5.1 Prometheus Metrics

All GoSight services expose metrics at `/metrics`:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'gosight-ingestor'
    static_configs:
      - targets: ['ingestor:9090']

  - job_name: 'gosight-processor'
    static_configs:
      - targets: ['processor:9090']

  - job_name: 'gosight-api'
    static_configs:
      - targets: ['api:9090']
```

### 5.2 Key Metrics

| Metric | Description |
|--------|-------------|
| `gosight_ingestor_events_total` | Events received |
| `gosight_ingestor_events_rejected` | Events rejected |
| `gosight_ingestor_latency_seconds` | Ingestion latency |
| `gosight_processor_events_processed` | Events processed |
| `gosight_processor_kafka_lag` | Kafka consumer lag |
| `gosight_api_requests_total` | API requests |
| `gosight_api_request_duration` | API latency |

### 5.3 Grafana Dashboard

Import the GoSight dashboard from `deploy/grafana/dashboard.json`.

---

## 6. Backup & Recovery

### 6.1 Backup Script

```bash
#!/bin/bash
# scripts/backup.sh

BACKUP_DIR=/backups/$(date +%Y-%m-%d)
mkdir -p $BACKUP_DIR

# PostgreSQL
docker-compose exec -T postgres pg_dump -U gosight gosight > $BACKUP_DIR/postgres.sql

# ClickHouse (metadata only, data is large)
docker-compose exec -T clickhouse clickhouse-client \
  --query "SELECT * FROM system.tables WHERE database = 'gosight' FORMAT TabSeparated" \
  > $BACKUP_DIR/clickhouse-schema.txt

# Compress
tar -czf $BACKUP_DIR.tar.gz $BACKUP_DIR
rm -rf $BACKUP_DIR

# Upload to S3/MinIO (optional)
# aws s3 cp $BACKUP_DIR.tar.gz s3://backups/gosight/
```

### 6.2 Recovery

```bash
# Restore PostgreSQL
docker-compose exec -T postgres psql -U gosight gosight < backup/postgres.sql

# Restore ClickHouse tables
docker-compose exec -T clickhouse clickhouse-client < backup/init-clickhouse.sql
```

---

## 7. Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| Kafka not starting | Check `KAFKA_KRAFT_CLUSTER_ID` is set |
| ClickHouse OOM | Increase memory, check `max_memory_usage` |
| High Kafka lag | Scale processors, check ClickHouse write speed |
| SSL errors | Check Traefik certificates |
| Slow queries | Add ClickHouse indexes, check materialized views |

### Health Checks

```bash
# Check all services
docker-compose ps

# Check Kafka topics
docker-compose exec kafka kafka-topics.sh --bootstrap-server localhost:9092 --list

# Check ClickHouse
docker-compose exec clickhouse clickhouse-client --query "SELECT count() FROM gosight.events"

# Check API
curl https://api.gosight.example.com/health
```

---

## 8. Upgrades

```bash
# Pull latest images
docker-compose pull

# Apply database migrations
docker-compose run --rm api gosight-migrate up

# Restart with new images
docker-compose up -d

# Verify
docker-compose ps
docker-compose logs -f
```

---

## 9. References

- [System Architecture](./02-system-architecture.md)
- [Data Models](./03-data-models.md)
- [API Specification](./05-api-specification.md)
