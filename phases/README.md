# GoSight - Kế Hoạch Triển Khai

## Tổng Quan

**GoSight** là nền tảng analytics và session replay self-hosted với các tính năng:

- Event tracking tự động
- Session replay (pixel-perfect)
- UX insights (rage clicks, dead clicks, error clicks)
- Heatmaps (click, scroll)
- Real-time dashboard
- Alerting qua Telegram

## Tech Stack

| Layer | Technology |
|-------|------------|
| SDK | TypeScript, rrweb |
| Protocol | gRPC, Protobuf |
| Ingestion | Go |
| Message Queue | Kafka |
| Processing | Go |
| Analytics DB | ClickHouse |
| Metadata DB | PostgreSQL |
| Cache | Redis |
| API | Go (Chi/Fiber) |
| Dashboard | Next.js 14 |

## Phases Overview

| Phase | Tên | Mô tả | Status |
|-------|-----|-------|--------|
| 1 | Foundation & Infrastructure | Thiết lập cơ sở hạ tầng | :white_check_mark: |
| 2 | Ingestor Service | gRPC server nhận events | :white_check_mark: |
| 3 | SDK | Client-side JavaScript SDK | |
| 4 | Event Processor | Xử lý events vào ClickHouse | |
| 5 | Insight Processor | Phát hiện UX insights | |
| 6 | Replay Processor | Xử lý session replay | |
| 7 | API Service | REST API cho dashboard | |
| 8 | Dashboard | Next.js web interface | |
| 9 | Alert Processor | Xử lý và gửi alerts | |
| 10 | Testing & Optimization | Testing và tối ưu | |
| 11 | Documentation & Deployment | Docs và deployment | |

## Dependencies Graph

```
Phase 1 (Foundation)
    │
    ├── Phase 2 (Ingestor)
    │       │
    │       └── Phase 4 (Event Processor)
    │               │
    │               ├── Phase 5 (Insight Processor)
    │               │       │
    │               │       └── Phase 9 (Alert Processor)
    │               │
    │               ├── Phase 6 (Replay Processor)
    │               │
    │               └── Phase 7 (API)
    │                       │
    │                       └── Phase 8 (Dashboard)
    │
    └── Phase 3 (SDK) [có thể song song với Phase 2]

Phase 10 (Testing) ← Phase 8, 9
Phase 11 (Deployment) ← Phase 10
```

## Quick Start

Xem chi tiết từng phase trong các file:

1. [Phase 01 - Foundation](./phase-01-foundation.md)
2. [Phase 02 - Ingestor](./phase-02-ingestor.md)
3. [Phase 03 - SDK](./phase-03-sdk.md)
4. [Phase 04 - Event Processor](./phase-04-event-processor.md)
5. [Phase 05 - Insight Processor](./phase-05-insight-processor.md)
6. [Phase 06 - Replay Processor](./phase-06-replay-processor.md)
7. [Phase 07 - API Service](./phase-07-api.md)
8. [Phase 08 - Dashboard](./phase-08-dashboard.md)
9. [Phase 09 - Alert Processor](./phase-09-alert-processor.md)
10. [Phase 10 - Testing](./phase-10-testing.md)
11. [Phase 11 - Deployment](./phase-11-deployment.md)

## Tài Liệu Tham Khảo

- [Project Overview](../docs/01-project-overview.md)
- [System Architecture](../docs/02-system-architecture.md)
- [Data Models](../docs/03-data-models.md)
- [SDK Specification](../docs/04-sdk-specification.md)
- [API Specification](../docs/05-api-specification.md)
