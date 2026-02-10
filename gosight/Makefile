.PHONY: help dev dev-full down restart logs proto clean build test

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
	@echo "  Development:"
	@echo "    make proto      - Generate protobuf code"
	@echo "    make build      - Build all Go services"
	@echo "    make test       - Run all tests"
	@echo "    make lint       - Run linter"
	@echo ""
	@echo "  Database:"
	@echo "    make db-reset   - Reset all databases (WARNING: deletes data)"
	@echo "    make ch-cli     - Open ClickHouse CLI"
	@echo "    make pg-cli     - Open PostgreSQL CLI"
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
	@echo "   PostgreSQL: localhost:5432"
	@echo "   Redis:      localhost:6379"

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
	@until docker compose exec -T kafka kafka-topics.sh --bootstrap-server localhost:9092 --list > /dev/null 2>&1; do sleep 1; done
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

# Run tests
test:
	go test ./... -v

# Run linter
lint:
	golangci-lint run ./...

# ===========================================
# Database
# ===========================================

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
# Cleanup
# ===========================================

# Remove everything (containers + volumes)
clean:
	docker compose --profile dev down -v --remove-orphans
	rm -rf bin/
	@echo "✅ Cleaned up!"
