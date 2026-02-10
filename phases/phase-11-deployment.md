# Phase 11: Documentation & Deployment

## Mục Tiêu

Chuẩn bị production deployment và documentation.

## Prerequisites

- Phase 10 hoàn thành (Testing passed)

## Tasks

### 11.1 Dockerfiles

**`deploy/docker/ingestor.Dockerfile`**

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY ingestor/go.mod ingestor/go.sum ./
RUN go mod download

# Copy source
COPY ingestor/ ./
COPY proto/ ../proto/

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /ingestor ./cmd/ingestor

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary
COPY --from=builder /ingestor .
COPY config/ingestor.yaml ./config/

# Create non-root user
RUN adduser -D -u 1000 gosight
USER gosight

EXPOSE 8080 50051 9090

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s \
    CMD wget -q --spider http://localhost:8080/health || exit 1

CMD ["./ingestor"]
```

**`deploy/docker/processor.Dockerfile`**

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY processor/go.mod processor/go.sum ./
RUN go mod download

COPY processor/ ./

# Build all processors
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /event-processor ./cmd/event-processor && \
    go build -ldflags="-w -s" -o /insight-processor ./cmd/insight-processor && \
    go build -ldflags="-w -s" -o /replay-processor ./cmd/replay-processor && \
    go build -ldflags="-w -s" -o /alert-processor ./cmd/alert-processor

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /event-processor .
COPY --from=builder /insight-processor .
COPY --from=builder /replay-processor .
COPY --from=builder /alert-processor .
COPY config/processor.yaml ./config/

RUN adduser -D -u 1000 gosight
USER gosight

# Default to event processor
CMD ["./event-processor"]
```

**`deploy/docker/api.Dockerfile`**

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY api/go.mod api/go.sum ./
RUN go mod download

COPY api/ ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /api ./cmd/api

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /api .
COPY config/api.yaml ./config/

RUN adduser -D -u 1000 gosight
USER gosight

EXPOSE 8080 9090

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s \
    CMD wget -q --spider http://localhost:8080/health || exit 1

CMD ["./api"]
```

**`deploy/docker/dashboard.Dockerfile`**

```dockerfile
# Build stage
FROM node:20-alpine AS builder

WORKDIR /app

COPY dashboard/package*.json ./
RUN npm ci

COPY dashboard/ ./

# Build with environment
ARG NEXT_PUBLIC_API_URL
ARG NEXT_PUBLIC_INGEST_URL

ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_INGEST_URL=$NEXT_PUBLIC_INGEST_URL

RUN npm run build

# Runtime stage
FROM node:20-alpine

WORKDIR /app

RUN adduser -D -u 1000 gosight

COPY --from=builder --chown=gosight:gosight /app/.next/standalone ./
COPY --from=builder --chown=gosight:gosight /app/.next/static ./.next/static
COPY --from=builder --chown=gosight:gosight /app/public ./public

USER gosight

EXPOSE 3000

ENV NODE_ENV=production

CMD ["node", "server.js"]
```

---

### 11.2 Docker Compose Production

**`docker-compose.prod.yml`**

```yaml
version: '3.8'

services:
  traefik:
    image: traefik:v2.10
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./config/traefik:/etc/traefik
      - traefik_acme:/acme
    networks:
      - gosight
    deploy:
      resources:
        limits:
          memory: 256M

  kafka:
    image: bitnami/kafka:3.6
    restart: unless-stopped
    environment:
      - KAFKA_CFG_NODE_ID=0
      - KAFKA_CFG_PROCESS_ROLES=controller,broker
      - KAFKA_CFG_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093
      - KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
      - KAFKA_CFG_CONTROLLER_QUORUM_VOTERS=0@kafka:9093
      - KAFKA_CFG_CONTROLLER_LISTENER_NAMES=CONTROLLER
      - KAFKA_KRAFT_CLUSTER_ID=${KAFKA_CLUSTER_ID}
      - KAFKA_HEAP_OPTS=-Xmx1G -Xms1G
    volumes:
      - kafka_data:/bitnami/kafka
    networks:
      - gosight
    deploy:
      resources:
        limits:
          memory: 2G

  clickhouse:
    image: clickhouse/clickhouse-server:23.12
    restart: unless-stopped
    environment:
      - CLICKHOUSE_USER=${CLICKHOUSE_USER}
      - CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD}
    volumes:
      - clickhouse_data:/var/lib/clickhouse
      - ./scripts/init-clickhouse.sql:/docker-entrypoint-initdb.d/init.sql
    ulimits:
      nofile:
        soft: 262144
        hard: 262144
    networks:
      - gosight
    deploy:
      resources:
        limits:
          memory: 8G

  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      - POSTGRES_USER=${POSTGRES_USER}
      - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
      - POSTGRES_DB=${POSTGRES_DB}
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./scripts/init-postgres.sql:/docker-entrypoint-initdb.d/init.sql
    networks:
      - gosight
    deploy:
      resources:
        limits:
          memory: 1G

  redis:
    image: redis:7-alpine
    restart: unless-stopped
    command: redis-server --requirepass ${REDIS_PASSWORD} --maxmemory 1gb --maxmemory-policy allkeys-lru
    volumes:
      - redis_data:/data
    networks:
      - gosight
    deploy:
      resources:
        limits:
          memory: 1G

  ingestor:
    image: gosight/ingestor:${VERSION:-latest}
    restart: unless-stopped
    depends_on:
      - kafka
      - redis
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
    networks:
      - gosight
    deploy:
      replicas: 2
      resources:
        limits:
          memory: 512M

  event-processor:
    image: gosight/processor:${VERSION:-latest}
    restart: unless-stopped
    command: ["./event-processor"]
    depends_on:
      - kafka
      - clickhouse
    environment:
      - CONFIG_PATH=/etc/gosight/processor.yaml
      - KAFKA_BROKERS=kafka:9092
      - CLICKHOUSE_DSN=clickhouse://${CLICKHOUSE_USER}:${CLICKHOUSE_PASSWORD}@clickhouse:9000/gosight
      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379
    networks:
      - gosight
    deploy:
      replicas: 2
      resources:
        limits:
          memory: 1G

  insight-processor:
    image: gosight/processor:${VERSION:-latest}
    restart: unless-stopped
    command: ["./insight-processor"]
    depends_on:
      - kafka
      - clickhouse
      - redis
    environment:
      - CONFIG_PATH=/etc/gosight/processor.yaml
      - KAFKA_BROKERS=kafka:9092
      - CLICKHOUSE_DSN=clickhouse://${CLICKHOUSE_USER}:${CLICKHOUSE_PASSWORD}@clickhouse:9000/gosight
      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379
    networks:
      - gosight
    deploy:
      replicas: 1
      resources:
        limits:
          memory: 512M

  alert-processor:
    image: gosight/processor:${VERSION:-latest}
    restart: unless-stopped
    command: ["./alert-processor"]
    depends_on:
      - kafka
      - postgres
      - redis
    environment:
      - CONFIG_PATH=/etc/gosight/processor.yaml
      - KAFKA_BROKERS=kafka:9092
      - DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}
      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379
      - TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}
    networks:
      - gosight
    deploy:
      replicas: 1
      resources:
        limits:
          memory: 256M

  api:
    image: gosight/api:${VERSION:-latest}
    restart: unless-stopped
    depends_on:
      - postgres
      - clickhouse
      - redis
    environment:
      - CONFIG_PATH=/etc/gosight/api.yaml
      - DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}
      - CLICKHOUSE_DSN=clickhouse://${CLICKHOUSE_USER}:${CLICKHOUSE_PASSWORD}@clickhouse:9000/gosight
      - REDIS_URL=redis://:${REDIS_PASSWORD}@redis:6379
      - JWT_SECRET=${JWT_SECRET}
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.api.rule=Host(`api.${DOMAIN}`)"
      - "traefik.http.routers.api.tls=true"
      - "traefik.http.routers.api.tls.certresolver=letsencrypt"
    networks:
      - gosight
    deploy:
      replicas: 2
      resources:
        limits:
          memory: 512M

  dashboard:
    image: gosight/dashboard:${VERSION:-latest}
    restart: unless-stopped
    environment:
      - NEXT_PUBLIC_API_URL=https://api.${DOMAIN}
      - NEXT_PUBLIC_INGEST_URL=https://ingest.${DOMAIN}
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.dashboard.rule=Host(`${DOMAIN}`)"
      - "traefik.http.routers.dashboard.tls=true"
      - "traefik.http.routers.dashboard.tls.certresolver=letsencrypt"
    networks:
      - gosight
    deploy:
      replicas: 2
      resources:
        limits:
          memory: 256M

networks:
  gosight:
    driver: bridge

volumes:
  traefik_acme:
  kafka_data:
  clickhouse_data:
  postgres_data:
  redis_data:
```

---

### 11.3 Makefile

**`Makefile`**

```makefile
.PHONY: all build push deploy clean

VERSION ?= $(shell git describe --tags --always --dirty)
REGISTRY ?= ghcr.io/gosight

# Build all Docker images
build:
	docker build -f deploy/docker/ingestor.Dockerfile -t $(REGISTRY)/ingestor:$(VERSION) .
	docker build -f deploy/docker/processor.Dockerfile -t $(REGISTRY)/processor:$(VERSION) .
	docker build -f deploy/docker/api.Dockerfile -t $(REGISTRY)/api:$(VERSION) .
	docker build -f deploy/docker/dashboard.Dockerfile \
		--build-arg NEXT_PUBLIC_API_URL=$(API_URL) \
		--build-arg NEXT_PUBLIC_INGEST_URL=$(INGEST_URL) \
		-t $(REGISTRY)/dashboard:$(VERSION) .

# Push to registry
push:
	docker push $(REGISTRY)/ingestor:$(VERSION)
	docker push $(REGISTRY)/processor:$(VERSION)
	docker push $(REGISTRY)/api:$(VERSION)
	docker push $(REGISTRY)/dashboard:$(VERSION)

# Deploy with Docker Compose
deploy:
	VERSION=$(VERSION) docker-compose -f docker-compose.prod.yml up -d

# Deploy specific service
deploy-%:
	VERSION=$(VERSION) docker-compose -f docker-compose.prod.yml up -d $*

# View logs
logs:
	docker-compose -f docker-compose.prod.yml logs -f

logs-%:
	docker-compose -f docker-compose.prod.yml logs -f $*

# Scale service
scale-%:
	docker-compose -f docker-compose.prod.yml up -d --scale $*=$(REPLICAS)

# Stop all
stop:
	docker-compose -f docker-compose.prod.yml down

# Clean
clean:
	docker-compose -f docker-compose.prod.yml down -v
	docker system prune -f

# Database migrations
migrate:
	docker-compose exec api ./api migrate up

# Create admin user
create-admin:
	docker-compose exec api ./api user create --email=$(EMAIL) --password=$(PASSWORD) --role=admin
```

---

### 11.4 GitHub Actions CI/CD

**`.github/workflows/ci.yml`**

```yaml
name: CI

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  test-go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Test Ingestor
        run: |
          cd ingestor
          go test -v -race -coverprofile=coverage.out ./...

      - name: Test Processor
        run: |
          cd processor
          go test -v -race -coverprofile=coverage.out ./...

      - name: Test API
        run: |
          cd api
          go test -v -race -coverprofile=coverage.out ./...

  test-sdk:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install dependencies
        run: cd sdk && npm ci

      - name: Test
        run: cd sdk && npm test

      - name: Build
        run: cd sdk && npm run build

  test-dashboard:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install dependencies
        run: cd dashboard && npm ci

      - name: Lint
        run: cd dashboard && npm run lint

      - name: Build
        run: cd dashboard && npm run build

  build-images:
    needs: [test-go, test-sdk, test-dashboard]
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4

      - uses: docker/setup-buildx-action@v3

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push
        run: |
          VERSION=${{ github.sha }}
          make build push VERSION=$VERSION
```

---

### 11.5 Monitoring Setup

**`deploy/monitoring/prometheus.yml`**

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'gosight-ingestor'
    static_configs:
      - targets: ['ingestor:9090']

  - job_name: 'gosight-processor'
    static_configs:
      - targets: ['event-processor:9090', 'insight-processor:9090']

  - job_name: 'gosight-api'
    static_configs:
      - targets: ['api:9090']

  - job_name: 'kafka'
    static_configs:
      - targets: ['kafka:9092']

  - job_name: 'clickhouse'
    static_configs:
      - targets: ['clickhouse:9363']
```

---

### 11.6 Backup Scripts

**`scripts/backup.sh`**

```bash
#!/bin/bash
set -e

BACKUP_DIR=/backups/$(date +%Y-%m-%d)
S3_BUCKET=s3://gosight-backups

mkdir -p $BACKUP_DIR

echo "Backing up PostgreSQL..."
docker-compose exec -T postgres pg_dump -U gosight gosight | gzip > $BACKUP_DIR/postgres.sql.gz

echo "Backing up ClickHouse metadata..."
docker-compose exec -T clickhouse clickhouse-client \
    --query "SELECT * FROM system.tables WHERE database = 'gosight' FORMAT TabSeparated" \
    | gzip > $BACKUP_DIR/clickhouse-schema.txt.gz

echo "Uploading to S3..."
aws s3 sync $BACKUP_DIR $S3_BUCKET/$(date +%Y-%m-%d)/

echo "Cleaning old local backups..."
find /backups -type d -mtime +7 -exec rm -rf {} +

echo "Backup complete!"
```

---

### 11.7 SDK Documentation

**`sdk/README.md`**

```markdown
# GoSight SDK

JavaScript SDK for GoSight Analytics.

## Installation

```bash
npm install @gosight/sdk
```

Or via CDN:

```html
<script src="https://cdn.gosight.io/v1/gosight.min.js"></script>
```

## Quick Start

```javascript
import GoSight from '@gosight/sdk';

GoSight.init({
  projectKey: 'gs_your_project_key',
});
```

## Configuration

```javascript
GoSight.init({
  projectKey: 'gs_xxx',

  // Ingest endpoint (for self-hosted)
  ingestUrl: 'https://ingest.your-domain.com',

  // Session timeout (default: 30 minutes)
  sessionTimeout: 30 * 60 * 1000,

  // Privacy settings
  privacy: {
    maskAllInputs: true,
    blockSelectors: ['.sensitive', '#payment-form'],
    blockUrls: ['/admin/*', '/checkout/payment'],
  },

  // Enable debug mode
  debug: true,
});
```

## API

### `GoSight.identify(userId, traits?)`

Identify the current user.

```javascript
GoSight.identify('user-123', {
  email: 'user@example.com',
  plan: 'pro',
});
```

### `GoSight.track(eventName, properties?)`

Track a custom event.

```javascript
GoSight.track('purchase', {
  amount: 99.99,
  currency: 'USD',
});
```

### `GoSight.getSessionId()`

Get the current session ID.

### `GoSight.getSessionUrl()`

Get a link to view this session in the dashboard.
```

---

## Deployment Checklist

### Pre-deployment

- [ ] All tests passing
- [ ] Environment variables configured
- [ ] SSL certificates ready
- [ ] Domain DNS configured
- [ ] Backup strategy in place

### Infrastructure

- [ ] Docker images built and pushed
- [ ] Database initialized
- [ ] Kafka topics created
- [ ] GeoIP database downloaded

### Security

- [ ] Secrets in environment variables
- [ ] Firewall rules configured
- [ ] HTTPS enforced
- [ ] API rate limiting enabled

### Monitoring

- [ ] Prometheus configured
- [ ] Grafana dashboards imported
- [ ] Alert rules created
- [ ] Log aggregation setup

### Post-deployment

- [ ] Health checks passing
- [ ] Test event ingestion
- [ ] Test session replay
- [ ] Test alerts
- [ ] Documentation updated

---

## Kết Quả

Sau phase này:
- Production-ready deployment
- CI/CD pipeline
- Monitoring và alerting
- Backup automation
- Full documentation

## Quick Deploy Commands

```bash
# Clone repo
git clone https://github.com/gosight/gosight.git
cd gosight

# Configure environment
cp .env.example .env
nano .env

# Build images
make build

# Deploy
make deploy

# Check status
docker-compose -f docker-compose.prod.yml ps

# View logs
make logs

# Create admin user
make create-admin EMAIL=admin@example.com PASSWORD=secure123
```
