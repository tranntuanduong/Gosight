# Phase 1: Foundation & Infrastructure

## Mục Tiêu

Thiết lập cơ sở hạ tầng và cấu trúc dự án cơ bản để các phases sau có thể xây dựng trên đó.

## Tasks

### 1.1 Khởi tạo Project Structure

```
gosight/
├── sdk/                      # JavaScript SDK (TypeScript)
│   ├── src/
│   │   ├── core/            # Core functionality
│   │   ├── events/          # Event handlers
│   │   ├── replay/          # rrweb integration
│   │   ├── transport/       # gRPC/HTTP transport
│   │   └── index.ts
│   ├── package.json
│   ├── tsconfig.json
│   └── rollup.config.js
│
├── ingestor/                 # Go gRPC server
│   ├── cmd/
│   │   └── ingestor/
│   │       └── main.go
│   ├── internal/
│   │   ├── server/          # gRPC & HTTP servers
│   │   ├── handler/         # Request handlers
│   │   ├── enricher/        # Event enrichment
│   │   ├── producer/        # Kafka producer
│   │   └── validation/      # Input validation
│   ├── go.mod
│   └── go.sum
│
├── processor/                # Go Kafka consumers
│   ├── cmd/
│   │   ├── event-processor/
│   │   ├── insight-processor/
│   │   ├── replay-processor/
│   │   └── alert-processor/
│   ├── internal/
│   │   ├── consumer/        # Kafka consumers
│   │   ├── storage/         # ClickHouse client
│   │   ├── insights/        # Insight algorithms
│   │   └── alerts/          # Alert handling
│   ├── go.mod
│   └── go.sum
│
├── api/                      # Go REST API
│   ├── cmd/
│   │   └── api/
│   │       └── main.go
│   ├── internal/
│   │   ├── handler/         # HTTP handlers
│   │   ├── middleware/      # Auth, CORS, etc.
│   │   ├── service/         # Business logic
│   │   └── repository/      # Database access
│   ├── go.mod
│   └── go.sum
│
├── dashboard/                # Next.js frontend
│   ├── src/
│   │   ├── app/             # App router
│   │   ├── components/      # React components
│   │   ├── hooks/           # Custom hooks
│   │   ├── lib/             # Utilities
│   │   └── types/           # TypeScript types
│   ├── package.json
│   └── next.config.js
│
├── proto/                    # Protobuf definitions
│   └── gosight/
│       ├── common.proto
│       ├── events.proto
│       └── ingest.proto
│
├── deploy/                   # Deployment configs
│   ├── docker/
│   │   ├── ingestor.Dockerfile
│   │   ├── processor.Dockerfile
│   │   ├── api.Dockerfile
│   │   └── dashboard.Dockerfile
│   ├── docker-compose.yml
│   ├── docker-compose.lite.yml
│   └── kubernetes/
│       └── helm/
│
├── scripts/                  # Utility scripts
│   ├── init-clickhouse.sql
│   ├── init-postgres.sql
│   ├── generate-proto.sh
│   └── download-geoip.sh
│
├── config/                   # Configuration files
│   ├── ingestor.yaml
│   ├── processor.yaml
│   └── api.yaml
│
├── docs/                     # Documentation
│
├── .env.example
├── Makefile
└── README.md
```

**Commands:**

```bash
# Tạo thư mục
mkdir -p gosight/{sdk/src/{core,events,replay,transport},ingestor/{cmd/ingestor,internal/{server,handler,enricher,producer,validation}},processor/{cmd/{event-processor,insight-processor,replay-processor,alert-processor},internal/{consumer,storage,insights,alerts}},api/{cmd/api,internal/{handler,middleware,service,repository}},dashboard/src/{app,components,hooks,lib,types},proto/gosight,deploy/{docker,kubernetes/helm},scripts,config}

cd gosight
```

---

### 1.2 Protobuf Definitions

**`proto/gosight/common.proto`**

```protobuf
syntax = "proto3";

package gosight;

option go_package = "github.com/gosight/gosight/proto/gosight";

// Timestamp với millisecond precision
message Timestamp {
  int64 seconds = 1;
  int32 millis = 2;
}

// Device information
message Device {
  string browser = 1;
  string browser_version = 2;
  string os = 3;
  string os_version = 4;
  string device_type = 5;  // desktop, mobile, tablet
  int32 screen_width = 6;
  int32 screen_height = 7;
  int32 viewport_width = 8;
  int32 viewport_height = 9;
}

// Page information
message Page {
  string url = 1;
  string path = 2;
  string title = 3;
  string referrer = 4;
}

// Session metadata
message SessionMeta {
  string session_id = 1;
  string user_id = 2;
  Device device = 3;
  string timezone = 4;
  string language = 5;
}

// Click target element
message TargetElement {
  string tag = 1;
  string selector = 2;
  repeated string classes = 3;
  string id = 4;
  string text = 5;  // truncated to 100 chars
  string href = 6;
  map<string, string> attributes = 7;
}
```

**`proto/gosight/events.proto`**

```protobuf
syntax = "proto3";

package gosight;

import "gosight/common.proto";

option go_package = "github.com/gosight/gosight/proto/gosight";

// Event type enum
enum EventType {
  EVENT_TYPE_UNSPECIFIED = 0;
  EVENT_TYPE_PAGE_VIEW = 1;
  EVENT_TYPE_CLICK = 2;
  EVENT_TYPE_SCROLL = 3;
  EVENT_TYPE_INPUT_CHANGE = 4;
  EVENT_TYPE_INPUT_FOCUS = 5;
  EVENT_TYPE_INPUT_BLUR = 6;
  EVENT_TYPE_MOUSE_MOVE = 7;
  EVENT_TYPE_VISIBILITY_CHANGE = 8;
  EVENT_TYPE_JS_ERROR = 9;
  EVENT_TYPE_NETWORK_ERROR = 10;
  EVENT_TYPE_CONSOLE_LOG = 11;
  EVENT_TYPE_WEB_VITALS = 12;
  EVENT_TYPE_PAGE_LOAD = 13;
  EVENT_TYPE_RESOURCE_LOAD = 14;
  EVENT_TYPE_CUSTOM = 15;
}

// Base event
message Event {
  string event_id = 1;
  EventType type = 2;
  int64 timestamp = 3;  // Unix milliseconds
  Page page = 4;

  oneof payload {
    ClickEvent click = 10;
    ScrollEvent scroll = 11;
    InputEvent input = 12;
    MouseMoveEvent mouse_move = 13;
    JsErrorEvent js_error = 14;
    WebVitalsEvent web_vitals = 15;
    PageLoadEvent page_load = 16;
    CustomEvent custom = 17;
  }
}

// Click event payload
message ClickEvent {
  int32 x = 1;
  int32 y = 2;
  TargetElement target = 3;
}

// Scroll event payload
message ScrollEvent {
  int32 scroll_top = 1;
  int32 scroll_height = 2;
  int32 viewport_height = 3;
  int32 depth_percent = 4;  // 0-100
}

// Input event payload
message InputEvent {
  TargetElement target = 1;
  string input_type = 2;
  bool is_masked = 3;
  string value = 4;  // only if not masked
}

// Mouse move event (batched positions)
message MouseMoveEvent {
  repeated MousePosition positions = 1;
}

message MousePosition {
  int32 x = 1;
  int32 y = 2;
  int64 t = 3;  // relative timestamp
}

// JavaScript error
message JsErrorEvent {
  string message = 1;
  string stack = 2;
  string source = 3;
  int32 line = 4;
  int32 column = 5;
  string error_type = 6;
}

// Web vitals metrics
message WebVitalsEvent {
  optional double lcp = 1;   // Largest Contentful Paint
  optional double fid = 2;   // First Input Delay
  optional double cls = 3;   // Cumulative Layout Shift
  optional double ttfb = 4;  // Time to First Byte
  optional double fcp = 5;   // First Contentful Paint
  optional double inp = 6;   // Interaction to Next Paint
}

// Page load timing
message PageLoadEvent {
  int64 dns_lookup = 1;
  int64 tcp_connect = 2;
  int64 request_start = 3;
  int64 response_start = 4;
  int64 response_end = 5;
  int64 dom_interactive = 6;
  int64 dom_complete = 7;
  int64 load_complete = 8;
}

// Custom event
message CustomEvent {
  string name = 1;
  map<string, string> properties = 2;
}

// Replay chunk (rrweb events)
message ReplayChunk {
  int32 chunk_index = 1;
  int64 timestamp_start = 2;
  int64 timestamp_end = 3;
  bytes data = 4;  // Compressed rrweb events JSON
  bool has_full_snapshot = 5;
}
```

**`proto/gosight/ingest.proto`**

```protobuf
syntax = "proto3";

package gosight;

import "gosight/common.proto";
import "gosight/events.proto";

option go_package = "github.com/gosight/gosight/proto/gosight";

// Ingest service
service IngestService {
  // Stream events from client
  rpc SendEvents(stream EventBatch) returns (stream EventAck);

  // Send replay chunks
  rpc SendReplay(stream ReplayChunk) returns (ReplayAck);
}

// Batch of events
message EventBatch {
  string project_key = 1;
  SessionMeta session = 2;
  repeated Event events = 3;
  int64 sent_at = 4;
}

// Event acknowledgment
message EventAck {
  bool success = 1;
  int32 accepted_count = 2;
  int32 rejected_count = 3;
  repeated string errors = 4;
}

// Replay acknowledgment
message ReplayAck {
  bool success = 1;
  string message = 2;
}
```

**Generate Proto:**

```bash
# scripts/generate-proto.sh
#!/bin/bash

# Install protoc plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate Go code
protoc --proto_path=proto \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  proto/gosight/*.proto

# Generate TypeScript (for SDK)
npx protoc --ts_out=sdk/src/proto \
  --proto_path=proto \
  proto/gosight/*.proto
```

---

### 1.3 Docker Compose Setup

**`docker-compose.yml`**

```yaml
version: '3.8'

services:
  # ─────────────────────────────────────────────
  # Message Queue
  # ─────────────────────────────────────────────
  kafka:
    image: bitnami/kafka:3.6
    container_name: gosight-kafka
    restart: unless-stopped
    ports:
      - "9092:9092"
    environment:
      - KAFKA_CFG_NODE_ID=0
      - KAFKA_CFG_PROCESS_ROLES=controller,broker
      - KAFKA_CFG_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093
      - KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
      - KAFKA_CFG_CONTROLLER_QUORUM_VOTERS=0@kafka:9093
      - KAFKA_CFG_CONTROLLER_LISTENER_NAMES=CONTROLLER
      - KAFKA_KRAFT_CLUSTER_ID=${KAFKA_CLUSTER_ID:-MkU3OEVBNTcwNTJENDM2Qg}
      - KAFKA_CFG_AUTO_CREATE_TOPICS_ENABLE=true
    volumes:
      - kafka_data:/bitnami/kafka
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
    ports:
      - "8123:8123"  # HTTP
      - "9000:9000"  # Native
    environment:
      - CLICKHOUSE_USER=${CLICKHOUSE_USER:-default}
      - CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD:-password}
      - CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1
    volumes:
      - clickhouse_data:/var/lib/clickhouse
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
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_USER=${POSTGRES_USER:-gosight}
      - POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-password}
      - POSTGRES_DB=${POSTGRES_DB:-gosight}
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./scripts/init-postgres.sql:/docker-entrypoint-initdb.d/init.sql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER:-gosight}"]
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
    ports:
      - "6379:6379"
    command: redis-server --requirepass ${REDIS_PASSWORD:-password}
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD:-password}", "ping"]
      interval: 30s
      timeout: 10s
      retries: 5

  # ─────────────────────────────────────────────
  # UI for Kafka (dev only)
  # ─────────────────────────────────────────────
  kafka-ui:
    image: provectuslabs/kafka-ui:latest
    container_name: gosight-kafka-ui
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - KAFKA_CLUSTERS_0_NAME=gosight
      - KAFKA_CLUSTERS_0_BOOTSTRAPSERVERS=kafka:9092
    depends_on:
      - kafka

networks:
  default:
    name: gosight-network

volumes:
  kafka_data:
  clickhouse_data:
  postgres_data:
  redis_data:
```

**`.env.example`**

```bash
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
KAFKA_CLUSTER_ID=MkU3OEVBNTcwNTJENDM2Qg

# JWT
JWT_SECRET=your_jwt_secret_min_32_characters

# Telegram (optional)
TELEGRAM_BOT_TOKEN=

# GeoIP (optional)
MAXMIND_LICENSE_KEY=
```

---

### 1.4 Database Initialization

**`scripts/init-clickhouse.sql`**

```sql
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

    -- Click data
    click_x Nullable(Int16),
    click_y Nullable(Int16),
    target_selector Nullable(String),
    target_tag Nullable(LowCardinality(String)),
    target_text Nullable(String),

    -- Scroll data
    scroll_depth Nullable(UInt8),

    -- Error data
    error_message Nullable(String),
    error_stack Nullable(String),
    error_type Nullable(LowCardinality(String)),

    -- Performance data
    lcp Nullable(UInt16),
    fid Nullable(UInt16),
    cls Nullable(Float32),
    ttfb Nullable(UInt16),

    -- Full payload (JSON)
    payload String DEFAULT '{}',

    -- Device info
    browser LowCardinality(String),
    browser_version String DEFAULT '',
    os LowCardinality(String),
    os_version String DEFAULT '',
    device_type LowCardinality(String),
    screen_width UInt16 DEFAULT 0,
    screen_height UInt16 DEFAULT 0,
    viewport_width UInt16 DEFAULT 0,
    viewport_height UInt16 DEFAULT 0,

    -- Geo info
    ip String DEFAULT '',
    country LowCardinality(String) DEFAULT '',
    city String DEFAULT '',

    -- Custom properties
    custom String DEFAULT '{}',

    -- Partitioning
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

    -- Timing
    started_at DateTime64(3),
    ended_at DateTime64(3),
    duration_ms UInt32 DEFAULT 0,

    -- Navigation
    entry_url String,
    entry_path String,
    exit_url String DEFAULT '',
    exit_path String DEFAULT '',

    -- Counts
    page_count UInt16 DEFAULT 0,
    event_count UInt32 DEFAULT 0,
    click_count UInt16 DEFAULT 0,
    error_count UInt16 DEFAULT 0,
    max_scroll_depth UInt8 DEFAULT 0,

    -- Flags
    has_error UInt8 DEFAULT 0,
    has_rage_click UInt8 DEFAULT 0,
    has_dead_click UInt8 DEFAULT 0,
    is_bounce UInt8 DEFAULT 0,

    -- UTM
    utm_source String DEFAULT '',
    utm_medium String DEFAULT '',
    utm_campaign String DEFAULT '',

    -- Device
    browser LowCardinality(String),
    os LowCardinality(String),
    device_type LowCardinality(String),

    -- Geo
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
    compressed_size UInt32,
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

**`scripts/init-postgres.sql`**

```sql
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
CREATE TABLE IF NOT EXISTS alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
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
    metric_value FLOAT NOT NULL,
    threshold FLOAT NOT NULL,
    context JSONB DEFAULT '{}'
);

-- Indexes
CREATE INDEX idx_api_keys_project ON api_keys(project_id);
CREATE INDEX idx_api_keys_hash ON api_keys(key_hash);
CREATE INDEX idx_project_members_user ON project_members(user_id);
CREATE INDEX idx_alert_rules_project ON alert_rules(project_id);
CREATE INDEX idx_alert_history_rule ON alert_history(rule_id);
```

---

### 1.5 Makefile

**`Makefile`**

```makefile
.PHONY: all proto infra dev build test clean

# Generate protobuf code
proto:
	./scripts/generate-proto.sh

# Start infrastructure
infra:
	docker-compose up -d kafka clickhouse postgres redis

# Stop infrastructure
infra-down:
	docker-compose down

# Start all services in dev mode
dev: infra
	@echo "Starting development servers..."
	@make -j4 dev-ingestor dev-processor dev-api dev-dashboard

dev-ingestor:
	cd ingestor && go run cmd/ingestor/main.go

dev-processor:
	cd processor && go run cmd/event-processor/main.go

dev-api:
	cd api && go run cmd/api/main.go

dev-dashboard:
	cd dashboard && npm run dev

# Build all services
build:
	docker-compose build

# Run tests
test:
	cd ingestor && go test ./...
	cd processor && go test ./...
	cd api && go test ./...
	cd sdk && npm test
	cd dashboard && npm test

# Clean up
clean:
	docker-compose down -v
	rm -rf data/
```

---

## Checklist

- [ ] Tạo project structure
- [ ] Viết Protobuf definitions
- [ ] Tạo script generate proto
- [ ] Viết docker-compose.yml
- [ ] Tạo .env.example
- [ ] Viết init-clickhouse.sql
- [ ] Viết init-postgres.sql
- [ ] Viết Makefile
- [ ] Chạy `docker-compose up -d` thành công
- [ ] Verify tất cả services healthy

## Kết Quả

Sau phase này:
- Infrastructure chạy hoàn chỉnh (Kafka, ClickHouse, PostgreSQL, Redis)
- Protobuf schemas được định nghĩa
- Database tables được tạo
- Sẵn sàng cho Phase 2

## Lệnh Kiểm Tra

```bash
# Kiểm tra containers
docker-compose ps

# Kiểm tra Kafka
docker-compose exec kafka kafka-topics.sh --bootstrap-server localhost:9092 --list

# Kiểm tra ClickHouse
docker-compose exec clickhouse clickhouse-client --query "SHOW DATABASES"

# Kiểm tra PostgreSQL
docker-compose exec postgres psql -U gosight -c "\dt"

# Kiểm tra Redis
docker-compose exec redis redis-cli -a password ping
```
