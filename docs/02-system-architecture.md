# GoSight - System Architecture

## 1. High-Level Overview

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                                   CLIENTS                                        │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐                         │
│  │ Website  │  │   SPA    │  │  Webapp  │  │  Mobile  │                         │
│  │   SDK    │  │   SDK    │  │   SDK    │  │  (future)│                         │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘                         │
└───────┼─────────────┼─────────────┼─────────────┼────────────────────────────────┘
        │             │             │             │
        └─────────────┴──────┬──────┴─────────────┘
                             │ gRPC / HTTP
                             ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                            INGESTION LAYER                                       │
│  ┌─────────────────────────────────────────────────────────────────────────┐    │
│  │                        Load Balancer (Traefik)                          │    │
│  └─────────────────────────────────┬───────────────────────────────────────┘    │
│                                    │                                             │
│       ┌────────────────────────────┼────────────────────────────┐               │
│       ▼                            ▼                            ▼               │
│  ┌──────────┐                ┌──────────┐                ┌──────────┐           │
│  │ Ingestor │                │ Ingestor │                │ Ingestor │           │
│  │  (Go)    │                │  (Go)    │                │  (Go)    │           │
│  └────┬─────┘                └────┬─────┘                └────┬─────┘           │
└───────┼───────────────────────────┼───────────────────────────┼──────────────────┘
        │                           │                           │
        └───────────────────────────┼───────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                            MESSAGE QUEUE (Kafka)                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │   events    │  │   replay    │  │   errors    │  │   alerts    │             │
│  │   topic     │  │   topic     │  │   topic     │  │   topic     │             │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘             │
└───────┬───────────────────────────┬───────────────────────────┬──────────────────┘
        │                           │                           │
        ▼                           ▼                           ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           PROCESSING LAYER                                       │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │   Event     │  │   Replay    │  │   Insight   │  │   Alert     │             │
│  │  Processor  │  │  Processor  │  │  Processor  │  │  Processor  │             │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘             │
└─────────┼────────────────┼────────────────┼────────────────┼─────────────────────┘
          │                │                │                │
          ▼                ▼                ▼                ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                            STORAGE LAYER                                         │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐               │
│  │    ClickHouse    │  │      Redis       │  │    PostgreSQL    │               │
│  │   (Analytics)    │  │    (Cache)       │  │   (Metadata)     │               │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘               │
│  ┌──────────────────┐                                                           │
│  │   MinIO (S3)     │  (Cold storage for old replays)                           │
│  └──────────────────┘                                                           │
└─────────────────────────────────────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                             API LAYER                                            │
│  ┌─────────────────────────────────────────────────────────────────────────┐    │
│  │                         API Gateway (Go)                                 │    │
│  │  • REST API            • WebSocket             • Authentication          │    │
│  └─────────────────────────────────────────────────────────────────────────┘    │
└───────┬─────────────────────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          PRESENTATION LAYER                                      │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐                  │
│  │    Dashboard    │  │  Replay Player  │  │    Grafana      │                  │
│  │   (Next.js)     │  │   (React)       │  │   (Optional)    │                  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Deployment Modes

GoSight hỗ trợ 3 chế độ triển khai tùy theo nhu cầu:

### 2.1 Mode Comparison

| Mode | Storage | Features | Use Case |
|------|---------|----------|----------|
| **Full** | ~100 GB/day | All features + Replay | Enterprise, debugging |
| **Analytics-Only** | ~1-2 GB/day | Heatmaps, analytics, NO replay | Cost-sensitive, startups |
| **Replay Optimized** | ~5-10 GB/day | Replay với sampling/triggers | Balanced |

### 2.2 Full Mode Architecture

```
SDK (30KB) → Ingestor → Kafka → [Event Processor  ] → ClickHouse
                              → [Replay Processor ] → ClickHouse + MinIO
                              → [Insight Processor] → ClickHouse
```

**Components:** 8 services, 300+ GB storage

### 2.3 Analytics-Only Mode Architecture

```
SDK (8KB) → Ingestor → Kafka → [Event Processor  ] → ClickHouse
                             → [Insight Processor] → ClickHouse
```

**Loại bỏ:**
- ❌ Replay Processor
- ❌ MinIO (S3)
- ❌ Kafka topic: `gosight.replay.chunks`
- ❌ ClickHouse table: `replay_chunks`
- ❌ rrweb trong SDK

**Vẫn có đầy đủ:**
- ✅ Click heatmaps
- ✅ Scroll heatmaps
- ✅ Time on page analytics
- ✅ User journey
- ✅ Rage click detection
- ✅ Error tracking
- ✅ Performance metrics
- ✅ Telegram alerts

**Resources:** 6 services, 10-50 GB storage, ~70% cost savings

### 2.4 Replay Optimized Mode

Giữ Session Replay nhưng tối ưu storage:

| Optimization | Storage Reduction |
|--------------|-------------------|
| Sampling 10% | -90% |
| Trigger-based recording | -80% |
| Reduce mouse movement | -35% |
| 7-day retention | Fixed cap |
| zstd compression | -40% |

**Config:**
```yaml
replay:
  enabled: true
  sampling_rate: 10           # 10% sessions
  mode: trigger               # Only on error/rage click
  retention_days: 7
  compression: zstd
  options:
    record_mouse_move: false
    max_duration_seconds: 300  # 5 min max
```

### 2.5 Storage Estimates

| Traffic | Full Mode | Analytics-Only | Replay Optimized |
|---------|-----------|----------------|------------------|
| 10 sites nhỏ | 50-100 GB/day | 500 MB/day | 5 GB/day |
| 10 sites vừa | 100-200 GB/day | 1-2 GB/day | 10-20 GB/day |
| 50 sites nhỏ | 100-200 GB/day | 1-2 GB/day | 10-20 GB/day |

### 2.6 Recommended Mode Selection

```
                    ┌─────────────────────────────────────┐
                    │  Bạn cần xem lại chính xác user     │
                    │  đã làm gì trên trang không?        │
                    └──────────────┬──────────────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                             ▼
                   YES                           NO
                    │                             │
                    ▼                             ▼
        ┌───────────────────────┐    ┌───────────────────────┐
        │ Budget > $100/month?  │    │   ANALYTICS-ONLY      │
        └───────────┬───────────┘    │   - Heatmaps          │
                    │                │   - Time on page      │
         ┌──────────┴──────────┐     │   - Click analytics   │
         ▼                     ▼     │   - $40/month         │
        YES                   NO     └───────────────────────┘
         │                     │
         ▼                     ▼
┌─────────────────┐  ┌─────────────────────┐
│   FULL MODE     │  │  REPLAY OPTIMIZED   │
│   - Everything  │  │  - 10% sampling     │
│   - $150/month  │  │  - Trigger-based    │
└─────────────────┘  │  - $60-80/month     │
                     └─────────────────────┘
```

---

## 3. Component Specifications

### 2.1 SDK Layer

**Package:** `@gosight/sdk`

| Attribute | Value |
|-----------|-------|
| Language | TypeScript |
| Bundle Size | < 30KB gzipped |
| Dependencies | rrweb, protobuf-js |
| Output | npm package + CDN bundle |

**Responsibilities:**
- Auto-track user interactions via event delegation
- Record DOM mutations using rrweb
- Batch and compress events
- Handle offline scenarios with IndexedDB queue
- Apply client-side privacy rules

**Architecture:**

```
┌─────────────────────────────────────────────────────────────┐
│                     GoSight SDK                              │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │   Event     │  │  Mutation   │  │   rrweb     │          │
│  │  Listeners  │  │  Observer   │  │  Recorder   │          │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘          │
│         │                │                │                  │
│         └────────────────┼────────────────┘                  │
│                          ▼                                   │
│  ┌─────────────────────────────────────────────────┐        │
│  │              Event Processor                     │        │
│  │  • Enrich metadata    • Apply privacy rules     │        │
│  │  • Throttle/debounce  • Generate selectors      │        │
│  └──────────────────────┬──────────────────────────┘        │
│                         ▼                                    │
│  ┌─────────────────────────────────────────────────┐        │
│  │              Event Buffer                        │        │
│  │  • Batch by count (10) or time (5s)             │        │
│  │  • Priority flush for critical events           │        │
│  └──────────────────────┬──────────────────────────┘        │
│                         ▼                                    │
│  ┌─────────────────────────────────────────────────┐        │
│  │              Transport Layer                     │        │
│  │  • gRPC-Web (primary)  • HTTP POST (fallback)   │        │
│  │  • Retry logic         • Offline queue          │        │
│  └─────────────────────────────────────────────────┘        │
└─────────────────────────────────────────────────────────────┘
```

---

### 2.2 Ingestor Service

**Binary:** `gosight-ingestor`

| Attribute | Value |
|-----------|-------|
| Language | Go 1.21+ |
| Protocol | gRPC + gRPC-Web |
| Performance | 10,000 events/sec per instance |
| Latency | < 10ms p99 |

**Responsibilities:**
- Receive event streams via gRPC
- Validate API keys and rate limit
- Decompress and deserialize Protobuf
- Enrich events with geo data (MaxMind)
- Parse user agent strings
- Route events to appropriate Kafka topics

**Endpoints:**

| Endpoint | Protocol | Description |
|----------|----------|-------------|
| `IngestService.SendEvents` | gRPC Stream | Primary event ingestion |
| `POST /v1/events` | HTTP | Fallback for non-gRPC clients |
| `GET /health` | HTTP | Health check |
| `GET /ready` | HTTP | Readiness check |

**Configuration:**

```yaml
server:
  grpc_port: 50051
  http_port: 8080
  max_connections: 10000

kafka:
  brokers:
    - kafka-1:9092
    - kafka-2:9092
    - kafka-3:9092
  topics:
    events: gosight.events.raw
    replay: gosight.replay.chunks
    errors: gosight.events.errors

rate_limit:
  requests_per_second: 1000
  burst: 5000

geoip:
  database_path: /data/GeoLite2-City.mmdb
```

---

### 2.3 Kafka Topics

| Topic | Partitions | Replication | Retention | Key | Content |
|-------|------------|-------------|-----------|-----|---------|
| `gosight.events.raw` | 12 | 3 | 7 days | `project_id:session_id` | All raw events |
| `gosight.replay.chunks` | 6 | 3 | 3 days | `session_id` | DOM snapshots/mutations |
| `gosight.events.errors` | 3 | 3 | 30 days | `project_id` | JS errors, resource errors |
| `gosight.insights` | 3 | 3 | 30 days | `project_id` | Derived insights |
| `gosight.alerts` | 1 | 3 | 7 days | - | Alert triggers |

**Partitioning Strategy:**

Events are partitioned by `project_id + session_id` to ensure:
- Events from same session go to same partition
- Ordered processing within a session
- Even distribution across partitions

---

### 2.4 Processing Services

#### Event Processor

**Binary:** `gosight-event-processor`

| Attribute | Value |
|-----------|-------|
| Consumes | `gosight.events.raw` |
| Produces | ClickHouse writes |
| Consumer Group | `gosight-event-processor` |

**Responsibilities:**
- Transform events to ClickHouse schema
- Batch inserts (1000 rows or 5 seconds)
- Update session aggregates
- Deduplicate events

---

#### Replay Processor

**Binary:** `gosight-replay-processor`

| Attribute | Value |
|-----------|-------|
| Consumes | `gosight.replay.chunks` |
| Produces | ClickHouse + MinIO |
| Consumer Group | `gosight-replay-processor` |

**Responsibilities:**
- Aggregate replay chunks by session
- Compress chunks (gzip)
- Store in ClickHouse (hot)
- Move old replays to MinIO (cold, >30 days)

---

#### Insight Processor

**Binary:** `gosight-insight-processor`

| Attribute | Value |
|-----------|-------|
| Consumes | `gosight.events.raw` |
| Produces | `gosight.insights`, `gosight.alerts` |
| Consumer Group | `gosight-insight-processor` |

**Responsibilities:**
- Detect rage clicks using sliding window
- Detect dead clicks (no DOM response)
- Detect thrashed cursor patterns
- Detect error clicks (click → JS error)
- Trigger alerts when thresholds exceeded

**State Management:**

Uses Redis for windowed aggregations:
```
clicks:{session_id}:{grid_cell} → [timestamp1, timestamp2, ...]
```

---

#### Alert Processor

**Binary:** `gosight-alert-processor`

| Attribute | Value |
|-----------|-------|
| Consumes | `gosight.alerts` |
| Produces | Telegram notifications |
| Consumer Group | `gosight-alert-processor` |

**Responsibilities:**
- Evaluate alert rules from PostgreSQL
- Deduplicate alerts (cooldown period)
- Send notifications to Telegram
- Store alert history

---

### 2.5 Storage Layer

#### ClickHouse

**Purpose:** Analytics queries, time-series data

**Tables:**
- `events` - All raw events
- `sessions` - Aggregated session data
- `replay_chunks` - DOM recording data
- `insights` - Derived insights
- `pageviews` - Page view aggregates

**Engine:** MergeTree with monthly partitions

**Scaling:** Start with single node, can scale to cluster

---

#### PostgreSQL

**Purpose:** Metadata, configuration, user management

**Tables:**
- `users` - Dashboard users
- `projects` - Project configurations
- `api_keys` - API key management
- `project_members` - Team membership
- `alert_rules` - Alert configurations
- `alert_history` - Alert log

---

#### Redis

**Purpose:** Caching, rate limiting, real-time state

**Key Patterns:**

| Pattern | TTL | Purpose |
|---------|-----|---------|
| `session:{id}:state` | 30 min | Active session state |
| `ratelimit:{project}:{min}` | 1 min | Rate limiting counters |
| `apikey:{hash}` | 5 min | API key cache |
| `realtime:{project}:sessions` | 1 hour | HyperLogLog for live sessions |
| `clicks:{session}:{cell}` | 10 sec | Rage click detection |

---

#### MinIO (S3-compatible)

**Purpose:** Cold storage for old session replays

**Bucket Structure:**
```
gosight-replays/
└── {project_id}/
    └── {year}/{month}/
        └── {session_id}.gz
```

---

### 2.6 API Gateway

**Binary:** `gosight-api`

| Attribute | Value |
|-----------|-------|
| Language | Go |
| Framework | Chi / Fiber |
| Auth | JWT + API Keys |

**Endpoint Groups:**

| Group | Base Path | Description |
|-------|-----------|-------------|
| Auth | `/api/v1/auth` | Login, logout, refresh |
| Projects | `/api/v1/projects` | Project CRUD |
| Analytics | `/api/v1/projects/:id/...` | Metrics, sessions, events |
| Alerts | `/api/v1/projects/:id/alerts` | Alert rules |
| WebSocket | `/api/v1/projects/:id/realtime` | Live updates |

---

### 2.7 Dashboard

**Framework:** Next.js 14 (App Router)

**Pages:**

| Route | Description |
|-------|-------------|
| `/` | Home / Project list |
| `/[project]/overview` | Analytics dashboard |
| `/[project]/sessions` | Session list |
| `/[project]/sessions/[id]` | Session detail + replay |
| `/[project]/errors` | Error list |
| `/[project]/heatmaps` | Heatmap viewer |
| `/[project]/alerts` | Alert configuration |
| `/[project]/settings` | Project settings |

---

## 3. Data Flow Diagrams

### 3.1 Event Ingestion Flow

```
┌──────┐         ┌──────────┐         ┌─────────┐         ┌───────────┐         ┌───────────┐
│ SDK  │────────▶│ Ingestor │────────▶│  Kafka  │────────▶│ Processor │────────▶│ClickHouse│
└──────┘         └──────────┘         └─────────┘         └───────────┘         └───────────┘
   │                  │
   │ Batch            │ Enrich
   │ Protobuf         │ • GeoIP
   │ gzip             │ • User Agent
   │                  │ • Validate
   │                  │
   │                  ▼
   │             ┌─────────┐
   │             │  Redis  │ (rate limit check)
   │             └─────────┘
   │
   ▼
 10 events OR 5 seconds
 ~500 bytes per event
 ~5KB per batch
```

### 3.2 Session Replay Flow

```
┌──────┐         ┌──────────┐         ┌─────────┐         ┌───────────┐
│rrweb │────────▶│ Ingestor │────────▶│  Kafka  │────────▶│  Replay   │
│Record│         │          │         │ (replay │         │ Processor │
└──────┘         └──────────┘         │  topic) │         └───────────┘
   │                                  └─────────┘               │
   │                                                            │
   │ Full snapshot (initial)                                    ▼
   │ + Incremental mutations                              ┌───────────┐
   │                                                      │ClickHouse│
   │                                                      │ (< 30d)  │
   │                                                      └───────────┘
   │                                                            │
   │                                                            │ Age > 30d
   │                                                            ▼
   │                                                      ┌───────────┐
   │                                                      │   MinIO   │
   │                                                      │  (cold)   │
   │                                                      └───────────┘
```

### 3.3 Insight Detection Flow

```
┌─────────┐         ┌───────────┐         ┌─────────┐
│  Kafka  │────────▶│  Insight  │────────▶│  Kafka  │
│ (events)│         │ Processor │         │(insights│
└─────────┘         └───────────┘         └─────────┘
                         │                     │
                         │                     │
                    ┌────┴────┐                │
                    ▼         ▼                ▼
               ┌─────────┐  ┌───────────┐  ┌───────────┐
               │  Redis  │  │   Alert   │  │ClickHouse│
               │ (state) │  │ Processor │  │           │
               └─────────┘  └─────┬─────┘  └───────────┘
                                  │
                                  ▼
                            ┌──────────┐
                            │ Telegram │
                            └──────────┘
```

---

## 4. Scaling Strategy

### 4.1 Horizontal Scaling

| Component | Scaling Method | Trigger |
|-----------|----------------|---------|
| Ingestor | Add replicas | CPU > 70% |
| Kafka | Add partitions | Lag > 10K |
| Event Processor | Add consumers | Lag > 10K |
| API | Add replicas | Latency > 100ms |
| ClickHouse | Add shards | Storage > 80% |

### 4.2 Bottleneck Analysis

| Expected Bottleneck | Solution |
|---------------------|----------|
| Kafka throughput | Increase partitions, add brokers |
| ClickHouse writes | Batch larger, add replicas |
| ClickHouse reads | Materialized views, add replicas |
| Redis memory | Add replicas, increase memory |

---

## 5. Reliability

### 5.1 Failure Scenarios

| Scenario | Mitigation |
|----------|------------|
| Ingestor down | Load balancer routes to healthy instances |
| Kafka broker down | Replication factor 3, automatic failover |
| Processor down | Kafka retains events, consumer catches up |
| ClickHouse down | Events buffered in Kafka |
| Redis down | Graceful degradation, rebuild from DB |
| API down | Multiple replicas behind load balancer |

### 5.2 Data Durability

- Kafka: 3x replication, acks=all
- ClickHouse: Replicated tables (optional)
- PostgreSQL: Streaming replication
- MinIO: Erasure coding

---

## 6. Security

### 6.1 Network Security

```
Internet
    │
    ▼
┌─────────┐
│ Traefik │ ← TLS termination
└────┬────┘
     │
     ▼
┌─────────────────────────────────────┐
│          Internal Network           │
│  (No direct internet access)        │
│                                     │
│  Ingestor ←→ Kafka ←→ Processors   │
│                ↕                    │
│           ClickHouse                │
│           PostgreSQL                │
│              Redis                  │
└─────────────────────────────────────┘
```

### 6.2 Authentication

| Component | Method |
|-----------|--------|
| SDK → Ingestor | API Key (project key) |
| Dashboard → API | JWT (access + refresh) |
| Service → Service | mTLS (optional) |

### 6.3 Authorization

- Role-based access control (RBAC)
- Project-level isolation
- Replay access restricted by role

---

## 7. Monitoring

### 7.1 Metrics (Prometheus)

| Metric | Type | Description |
|--------|------|-------------|
| `ingestor_events_total` | Counter | Events received |
| `ingestor_latency_seconds` | Histogram | Processing latency |
| `kafka_consumer_lag` | Gauge | Consumer lag |
| `clickhouse_insert_rows` | Counter | Rows inserted |
| `api_request_duration` | Histogram | API latency |

### 7.2 Logging (Structured JSON)

```json
{
  "level": "info",
  "service": "ingestor",
  "event": "events_received",
  "project_id": "abc123",
  "count": 10,
  "duration_ms": 5,
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### 7.3 Tracing (OpenTelemetry)

Distributed tracing across:
- SDK → Ingestor → Kafka → Processor → ClickHouse

---

## 8. References

- [Data Models](./03-data-models.md)
- [SDK Specification](./04-sdk-specification.md)
- [API Specification](./05-api-specification.md)
- [Deployment Guide](./11-deployment-guide.md)
