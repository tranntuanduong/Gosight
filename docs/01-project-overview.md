# GoSight - Project Overview

## 1. Introduction

**GoSight** is a self-hosted, real-time user analytics and session replay platform focused on understanding user behavior. The system is designed for high throughput with a target of **10,000 events/second**.

### Vision

Transform raw user interaction data (clicks, scrolls, mouse movements) into actionable insights (Rage clicks, Dead clicks, Session Replay) to help product teams understand and improve user experience.

### Key Features

| Feature | Description |
|---------|-------------|
| **Event Tracking** | Automatic tracking of all user interactions |
| **Session Replay** | Pixel-perfect playback of user sessions using DOM recording |
| **UX Insights** | Automated detection of rage clicks, dead clicks, error clicks |
| **Heatmaps** | Click and scroll heatmaps for any page |
| **Error Tracking** | JavaScript error capture with session context |
| **Real-time Dashboard** | Live metrics and session monitoring |
| **Alerting** | Telegram notifications for anomalies |

---

## 2. Core Concepts

### 2.1 Event-Driven Architecture

GoSight follows an event-driven pipeline pattern:

```
SDK → Ingestor → Kafka → Processors → ClickHouse → API → Dashboard
```

All user interactions are captured as **events**, streamed through the pipeline, processed for insights, and stored for analysis.

### 2.2 Session

A **session** represents a continuous period of user activity on a website. A new session is created when:

- User first visits the site
- User returns after 30 minutes of inactivity
- User visits from a different browser/device
- Calendar day changes (midnight)

### 2.3 Events

Events are atomic units of user interaction:

- **Interaction Events**: clicks, scrolls, input changes
- **Navigation Events**: page views, page exits
- **System Events**: errors, performance metrics
- **Replay Events**: DOM snapshots and mutations

### 2.4 Insights

Insights are derived metrics computed from raw events:

- **Rage Click**: User clicking rapidly (≥5 clicks in 2 seconds)
- **Dead Click**: Click on non-interactive element
- **Error Click**: Click that leads to JavaScript error
- **Thrashed Cursor**: Erratic mouse movement indicating confusion

---

## 3. Technology Stack

| Layer | Technology | Purpose |
|-------|------------|---------|
| **SDK** | TypeScript, rrweb | Client-side event capture |
| **Protocol** | gRPC, Protobuf | Efficient data transmission |
| **Ingestion** | Go | High-performance event receiver |
| **Message Queue** | Kafka | Event buffering and distribution |
| **Processing** | Go | Event processing and insight detection |
| **Analytics DB** | ClickHouse | Time-series analytics storage |
| **Metadata DB** | PostgreSQL | User, project, settings storage |
| **Cache** | Redis | Session state, rate limiting |
| **API** | Go | REST API for dashboard |
| **Dashboard** | React, Next.js | Web interface |

---

## 4. Design Principles

### 4.1 Privacy First

- All sensitive inputs masked by default
- Configurable blocklist for URLs and selectors
- IP anonymization support
- GDPR-compliant data handling
- Role-based access to session replays

### 4.2 Performance

- SDK bundle < 30KB gzipped
- < 1% CPU overhead on client
- Event batching to minimize network requests
- Protobuf for 70% smaller payloads vs JSON

### 4.3 Reliability

- Kafka ensures no event loss
- Automatic retry with exponential backoff
- Offline event queuing in SDK
- Graceful degradation

### 4.4 Scalability

- Horizontal scaling of all services
- Kafka partitioning for parallel processing
- ClickHouse for efficient analytics queries
- Stateless services for easy replication

---

## 5. Deployment Model

GoSight is designed for **self-hosted** deployment:

### Minimum Requirements

| Resource | Specification |
|----------|---------------|
| **CPU** | 12 cores |
| **RAM** | 20 GB |
| **Storage** | 300 GB SSD |
| **Network** | 100 Mbps |

### Deployment Options

1. **Docker Compose** - Single server, development/small scale
2. **Kubernetes** - Production, scalable deployment

### Data Retention

- Events: Configurable (default unlimited)
- Session Replays: Hot storage (ClickHouse) + Cold storage (Object Store)
- No automatic data deletion

---

## 6. User Roles

| Role | Permissions |
|------|-------------|
| **Owner** | Full access, billing, delete project |
| **Admin** | Manage members, settings, alerts |
| **Member** | View analytics, session replays |
| **Viewer** | View analytics only (no replay) |

---

## 7. Integration

### SDK Installation

```html
<!-- One-line installation -->
<script>
  (function(w,d,s,k){
    w.GoSight=w.GoSight||{q:[]};
    w.GoSight.q.push(['init',k]);
    var f=d.getElementsByTagName(s)[0],
        j=d.createElement(s);
    j.async=1;
    j.src='https://your-domain.com/sdk.js';
    f.parentNode.insertBefore(j,f);
  })(window,document,'script','YOUR_PROJECT_KEY');
</script>
```

### NPM Package

```bash
npm install @gosight/sdk
```

```typescript
import { GoSight } from '@gosight/sdk';
GoSight.init({ projectKey: 'YOUR_PROJECT_KEY' });
```

---

## 8. Project Structure

```
gosight/
├── sdk/                    # JavaScript SDK
│   ├── src/
│   └── package.json
├── ingestor/               # Go gRPC server
│   ├── cmd/
│   ├── internal/
│   └── go.mod
├── processor/              # Go Kafka consumers
│   ├── cmd/
│   ├── internal/
│   └── go.mod
├── api/                    # Go REST API
│   ├── cmd/
│   ├── internal/
│   └── go.mod
├── dashboard/              # React/Next.js frontend
│   ├── src/
│   └── package.json
├── proto/                  # Protobuf definitions
│   └── gosight/
├── deploy/                 # Deployment configs
│   ├── docker-compose.yml
│   └── k8s/
└── docs/                   # Documentation
```

---

## 9. Roadmap

### Phase 1: Core Platform
- [x] Event tracking SDK
- [x] Session replay
- [x] Basic analytics dashboard
- [x] Error tracking

### Phase 2: Insights
- [ ] Rage click detection
- [ ] Dead click detection
- [ ] Heatmaps
- [ ] Funnel analysis

### Phase 3: Advanced
- [ ] A/B testing integration
- [ ] Custom events API
- [ ] Webhooks
- [ ] Mobile SDK (React Native)

---

## 10. References

- [System Architecture](./02-system-architecture.md)
- [Data Models](./03-data-models.md)
- [SDK Specification](./04-sdk-specification.md)
- [API Specification](./05-api-specification.md)
- [Event Catalog](./06-event-catalog.md)
- [Deployment Guide](./11-deployment-guide.md)
