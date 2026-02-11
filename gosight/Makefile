.PHONY: help dev dev-full down restart logs proto clean build test test-integration test-coverage run-ingestor run-processor ch-init ch-events ch-sessions ch-vitals ch-errors

# Environment variables
export POSTGRES_USER ?= gosight
export POSTGRES_PASSWORD ?= gosight_pass
export POSTGRES_DB ?= gosight
export REDIS_PASSWORD ?=

# Default target
help:
	@echo "GoSight - Available commands:"
	@echo ""
	@echo "  Infrastructure:"
	@echo "    make dev        - Start core services (kafka, clickhouse, postgres, redis)"
	@echo "    make dev-full   - Start all services including kafka-ui"
	@echo "    make down       - Stop all services"
	@echo "    make restart    - Restart all services"
	@echo "    make logs       - View logs (all services)"
	@echo "    make logs-kafka - View Kafka logs"
	@echo "    make logs-ch    - View ClickHouse logs"
	@echo "    make ps         - Show running containers"
	@echo ""
	@echo "  Run Services:"
	@echo "    make run-ingestor          - Run ingestor service"
	@echo "    make run-processor         - Run event processor"
	@echo "    make run-insight-processor - Run insight processor"
	@echo "    make run-api               - Run API service"
	@echo ""
	@echo "  Development:"
	@echo "    make proto            - Generate protobuf code"
	@echo "    make build            - Build all Go services"
	@echo "    make test             - Run unit tests"
	@echo "    make test-integration - Run integration tests (needs infra)"
	@echo "    make test-coverage    - Run tests with coverage report"
	@echo "    make lint             - Run linter"
	@echo ""
	@echo "  Database:"
	@echo "    make db-reset   - Reset all databases (WARNING: deletes data)"
	@echo "    make db-seed    - Seed test data (API keys, etc.)"
	@echo "    make ch-cli     - Open ClickHouse CLI"
	@echo "    make ch-init    - Initialize ClickHouse schema"
	@echo "    make ch-events  - Show events from ClickHouse"
	@echo "    make ch-sessions- Show sessions from ClickHouse"
	@echo "    make ch-vitals  - Show web vitals from ClickHouse"
	@echo "    make ch-errors  - Show errors from ClickHouse"
	@echo "    make pg-cli     - Open PostgreSQL CLI"
	@echo ""
	@echo "  Testing:"
	@echo "    make test-api   - Test ingestor API with curl"
	@echo "    make kafka-consume - Consume messages from Kafka"
	@echo ""
	@echo "  Cleanup:"
	@echo "    make clean      - Remove containers and volumes"

# ===========================================
# Infrastructure
# ===========================================

# Start core services
dev:
	docker compose up -d
	@echo ""
	@echo "✅ Services started!"
	@echo "   Kafka:      localhost:9094"
	@echo "   ClickHouse: localhost:8123 (HTTP), localhost:9000 (Native)"
	@echo "   PostgreSQL: localhost:5433"
	@echo "   Redis:      localhost:6379"

# Full setup: start services, create topics, init db, seed data
setup: dev
	@echo "Waiting for services..."
	@sleep 10
	@$(MAKE) kafka-topics
	@$(MAKE) db-init
	@$(MAKE) ch-init
	@$(MAKE) db-seed
	@echo ""
	@echo "✅ Setup complete!"
	@echo "   Run 'make run-ingestor' to start the ingestor."
	@echo "   Run 'make run-processor' to start the event processor."

# Start all services including dev tools
dev-full:
	docker compose --profile dev up -d
	@echo ""
	@echo "✅ All services started!"
	@echo "   Kafka UI:   http://localhost:8080"

# Stop services
down:
	docker compose --profile dev down

# Restart services
restart: down dev

# View logs
logs:
	docker compose logs -f

logs-kafka:
	docker compose logs -f kafka

logs-ch:
	docker compose logs -f clickhouse

logs-pg:
	docker compose logs -f postgres

logs-processor:
	docker compose logs -f event-processor

# Show status
ps:
	docker compose ps

# Wait for services to be healthy
wait:
	@echo "Waiting for services to be healthy..."
	@until docker compose exec -T postgres pg_isready -U gosight > /dev/null 2>&1; do sleep 1; done
	@echo "✅ PostgreSQL is ready"
	@until docker compose exec -T clickhouse wget -q --spider http://localhost:8123/ping; do sleep 1; done
	@echo "✅ ClickHouse is ready"
	@until docker compose exec -T redis redis-cli ping > /dev/null 2>&1; do sleep 1; done
	@echo "✅ Redis is ready"
	@until docker compose exec -T kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list > /dev/null 2>&1; do sleep 1; done
	@echo "✅ Kafka is ready"

# ===========================================
# Development
# ===========================================

# Generate protobuf code
proto:
	./scripts/generate-proto.sh

# Build all Go services
build:
	go build -o bin/ingestor ./ingestor/cmd/ingestor
	go build -o bin/event-processor ./processor/cmd/event-processor
	go build -o bin/api ./api/cmd/api

# Run linter
lint:
	golangci-lint run ./...

# ===========================================
# Database
# ===========================================

# Initialize database schemas
db-init:
	@echo "Initializing PostgreSQL schema..."
	@docker exec -i gosight-postgres psql -U gosight -d gosight < scripts/init-postgres.sql 2>/dev/null || true
	@echo "✅ PostgreSQL initialized!"

# Reset databases (WARNING: deletes all data)
db-reset:
	@echo "⚠️  This will delete all data. Are you sure? [y/N]" && read ans && [ $${ans:-N} = y ]
	docker compose down -v
	docker compose up -d
	@$(MAKE) wait
	@echo "✅ Databases reset!"

# Open ClickHouse CLI
ch-cli:
	docker compose exec clickhouse clickhouse-client -u gosight --password gosight_pass

# Open PostgreSQL CLI
pg-cli:
	docker compose exec postgres psql -U gosight -d gosight

# Open Redis CLI
redis-cli:
	docker compose exec redis redis-cli

# ===========================================
# Run Services
# ===========================================

# Run ingestor service
run-ingestor:
	@echo "Starting Ingestor..."
	cd ingestor && CONFIG_PATH=../config/ingestor.yaml go run ./cmd/ingestor/

# Run event processor
run-processor:
	@echo "Starting Event Processor..."
	cd processor && CONFIG_PATH=../config/processor.yaml go run ./cmd/event-processor/

# Run insight processor
run-insight-processor:
	@echo "Starting Insight Processor..."
	cd processor && CONFIG_PATH=../config/processor.yaml go run ./cmd/insight-processor/

# Run API service
run-api:
	@echo "Starting API..."
	cd api && CONFIG_PATH=../config/api.yaml go run ./cmd/api/

# ===========================================
# Testing
# ===========================================

# Test ingestor API
test-api:
	@echo "Testing ingestor API..."
	curl -X POST http://localhost:8081/v1/events \
		-H "Content-Type: application/json" \
		-d '{"project_key":"gs_test_key_123456","session_id":"test-session","events":[{"type":"page_view","timestamp":1704067200000,"url":"https://example.com/","path":"/"}]}'

# Show Kafka message count and latest messages
kafka-consume:
	@echo "=== Message count per partition ==="
	@docker exec gosight-kafka /opt/kafka/bin/kafka-get-offsets.sh \
		--bootstrap-server localhost:9092 \
		--topic gosight.events.raw 2>/dev/null
	@echo ""
	@echo "=== Latest 5 messages (partition 0) ==="
	@OFFSET=$$(docker exec gosight-kafka /opt/kafka/bin/kafka-get-offsets.sh \
		--bootstrap-server localhost:9092 \
		--topic gosight.events.raw 2>/dev/null | grep ":0:" | cut -d: -f3); \
	START=$$((OFFSET > 5 ? OFFSET - 5 : 0)); \
	docker exec gosight-kafka /opt/kafka/bin/kafka-console-consumer.sh \
		--bootstrap-server localhost:9092 \
		--topic gosight.events.raw \
		--partition 0 --offset $$START --max-messages 5 2>/dev/null || true

# Consume all Kafka messages
kafka-tail:
	@echo "=== Reading all messages from gosight.events.raw ==="
	@docker exec gosight-kafka /opt/kafka/bin/kafka-console-consumer.sh \
		--bootstrap-server localhost:9092 \
		--topic gosight.events.raw \
		--partition 0 --offset earliest --max-messages 100 2>/dev/null || true

# Create Kafka topics
kafka-topics:
	docker exec gosight-kafka /opt/kafka/bin/kafka-topics.sh --create \
		--bootstrap-server localhost:9092 \
		--topic gosight.events.raw \
		--partitions 1 --replication-factor 1 2>/dev/null || true
	docker exec gosight-kafka /opt/kafka/bin/kafka-topics.sh --create \
		--bootstrap-server localhost:9092 \
		--topic gosight.replay.chunks \
		--partitions 1 --replication-factor 1 2>/dev/null || true
	docker exec gosight-kafka /opt/kafka/bin/kafka-topics.sh --list \
		--bootstrap-server localhost:9092

# Initialize ClickHouse schema
ch-init:
	@echo "Initializing ClickHouse schema..."
	@docker exec -i gosight-clickhouse clickhouse-client --multiquery < scripts/init-clickhouse.sql
	@echo "✅ ClickHouse initialized!"

# Check events count in ClickHouse
ch-events:
	@echo "=== Events count ==="
	@docker exec gosight-clickhouse clickhouse-client --query "SELECT count() as total FROM gosight.events"
	@echo ""
	@echo "=== Recent events ==="
	@docker exec gosight-clickhouse clickhouse-client --query "SELECT event_id, event_type, timestamp, page_path FROM gosight.events ORDER BY timestamp DESC LIMIT 10"

# Check sessions in ClickHouse
ch-sessions:
	@echo "=== Sessions count ==="
	@docker exec gosight-clickhouse clickhouse-client --query "SELECT count() as total FROM gosight.sessions"
	@echo ""
	@echo "=== Recent sessions ==="
	@docker exec gosight-clickhouse clickhouse-client --query "SELECT session_id, page_views, events_count, duration_ms FROM gosight.sessions ORDER BY started_at DESC LIMIT 10"

# Check web vitals in ClickHouse
ch-vitals:
	@echo "=== Web Vitals count ==="
	@docker exec gosight-clickhouse clickhouse-client --query "SELECT count() as total FROM gosight.web_vitals"
	@echo ""
	@echo "=== Recent vitals ==="
	@docker exec gosight-clickhouse clickhouse-client --query "SELECT page_path, lcp, fid, cls, ttfb FROM gosight.web_vitals ORDER BY timestamp DESC LIMIT 10"

# Check errors in ClickHouse
ch-errors:
	@echo "=== Errors count ==="
	@docker exec gosight-clickhouse clickhouse-client --query "SELECT count() as total FROM gosight.errors"
	@echo ""
	@echo "=== Recent errors ==="
	@docker exec gosight-clickhouse clickhouse-client --query "SELECT message, error_type, page_path FROM gosight.errors ORDER BY timestamp DESC LIMIT 10"

# Check insights in ClickHouse
ch-insights:
	@echo "=== Insights count ==="
	@docker exec gosight-clickhouse clickhouse-client --query "SELECT count() as total FROM gosight.insights"
	@echo ""
	@echo "=== Recent insights ==="
	@docker exec gosight-clickhouse clickhouse-client --query "SELECT insight_type, path, details, timestamp FROM gosight.insights ORDER BY timestamp DESC LIMIT 10"

# Seed test data
db-seed:
	@echo "Seeding test data..."
	docker exec gosight-postgres psql -U gosight -d gosight -c "\
		INSERT INTO users (id, email, password_hash, name, created_at, updated_at) \
		VALUES ('c2aade00-0e2d-4a00-ad8f-8dd1df502c33'::uuid, 'test@example.com', 'hash', 'Test User', NOW(), NOW()) \
		ON CONFLICT (id) DO NOTHING; \
		INSERT INTO projects (id, name, domain, owner_id, created_at, updated_at) \
		VALUES ('a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'::uuid, 'Test Project', 'example.com', 'c2aade00-0e2d-4a00-ad8f-8dd1df502c33'::uuid, NOW(), NOW()) \
		ON CONFLICT (id) DO NOTHING; \
		INSERT INTO api_keys (id, project_id, name, key_hash, key_prefix, is_active, created_at) \
		VALUES ('b1ffcd88-8d1c-4a09-ac7e-7cc0ce491b22'::uuid, 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11'::uuid, 'Test Key', encode(sha256('gs_test_key_123456'::bytea), 'hex'), 'gs_test_', true, NOW()) \
		ON CONFLICT (id) DO NOTHING;"
	@echo "✅ Test data seeded!"
	@echo "   API Key: gs_test_key_123456"

# ===========================================
# Cleanup
# ===========================================

# Remove everything (containers + volumes)
clean:
	docker compose --profile dev down -v --remove-orphans
	rm -rf bin/
	@echo "✅ Cleaned up!"
