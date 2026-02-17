.PHONY: help up down logs test build clean db-shell nats-streams generate-sample load-test health check-deps

# Project name
PROJECT_NAME := wec-telemetry-hybrid-architecture
DC := docker-compose

help: ## Show this help message
	@echo "$(PROJECT_NAME) - Development Commands"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ==========================================
# Docker Compose Commands
# ==========================================

up: ## Start all services (docker-compose up -d)
	$(DC) up -d
	@echo "✅ Services started. Run 'make health' to verify."

down: ## Stop all services
	$(DC) down

restart: down up ## Restart all services

logs: ## View logs from all services (follow)
	$(DC) logs -f

logs-ingestion: ## View ingestion service logs
	$(DC) logs -f ingestion-service

logs-processor: ## View stream processor logs
	$(DC) logs -f stream-processor

logs-sink: ## View raw event sink logs
	$(DC) logs -f raw-event-sink

logs-api: ## View query API logs
	$(DC) logs -f query-api

logs-simulator: ## View ECU simulator logs
	$(DC) logs -f ecu-simulator

# ==========================================
# Health & Status
# ==========================================

health: ## Check all services health
	@echo "📊 Service Health Check"
	@echo "========================"
	@echo ""
	@echo "Checking Ingestion Service..."
	@curl -s -o /dev/null -w "HTTP %{http_code}\n" http://localhost:8080/health || echo "❌ Ingestion offline"
	@echo ""
	@echo "Checking Query API..."
	@curl -s -o /dev/null -w "HTTP %{http_code}\n" http://localhost:8081/health || echo "❌ Query API offline"
	@echo ""
	@echo "Checking NATS..."
	@curl -s -o /dev/null -w "HTTP %{http_code}\n" http://localhost:8222 || echo "❌ NATS offline"
	@echo ""
	@echo "Checking PostgreSQL..."
	@$(DC) exec -T postgres pg_isready -U postgres > /dev/null && echo "✅ PostgreSQL ready" || echo "❌ PostgreSQL not ready"
	@echo ""
	@docker-compose ps

# ==========================================
# Database Commands
# ==========================================

db-shell: ## Open PostgreSQL shell
	$(DC) exec postgres psql -U postgres -d telemetry

db-init: ## Initialize database schema
	@echo "🔧 Initializing database..."
	$(DC) exec -T postgres psql -U postgres -d telemetry < infrastructure/postgres/schema.sql
	@echo "✅ Database initialized"

db-backup: ## Backup database to file
	$(DC) exec -T postgres pg_dump -U postgres telemetry > backup_$(shell date +%Y%m%d_%H%M%S).sql
	@echo "✅ Backup created"

# ==========================================
# NATS JetStream Commands
# ==========================================

nats-streams: ## List all NATS streams
	@echo "📡 NATS Streams"
	@$(DC) exec -T nats nats stream list || echo "❌ NATS not accessible"

nats-consumers: ## List all consumers
	@echo "📡 NATS Consumers"
	@$(DC) exec -T nats nats consumer list telemetry-raw || echo "❌ NATS not accessible"

nats-watch: ## Watch telemetry events in real-time
	$(DC) exec nats nats sub 'telemetry.>'

nats-info: ## Show stream info
	$(DC) exec nats nats stream info telemetry-raw

# ==========================================
# Data Generation & Testing
# ==========================================

generate-sample: ## Generate sample telemetry data (1 car, 10 laps)
	@echo "🚗 Generating sample data (1 car, 10 laps)..."
	$(DC) exec ecu-simulator python generate_sample.py
	@echo "✅ Sample data generated"

load-test: ## Run load test (4 cars, 100 Hz)
	@echo "🏋️ Running load test (4 cars, 100 Hz for 60 sec)..."
	$(DC) exec ecu-simulator python load_test.py --cars 4 --frequency 100 --duration 60
	@echo "✅ Load test complete"

stress-test: ## Intensive stress test (10 cars, 200 Hz, 5 min)
	@echo "⚡ Running stress test (10 cars, 200 Hz for 300 sec)..."
	$(DC) exec ecu-simulator python load_test.py --cars 10 --frequency 200 --duration 300
	@echo "✅ Stress test complete"

# ==========================================
# API Testing
# ==========================================

test-ingestion: ## Test ingestion endpoint
	@echo "🧪 Testing ingestion endpoint..."
	curl -X POST http://localhost:8080/telemetry/ingest \
		-H "Content-Type: application/json" \
		-d '{"car_id": "TEST_001", "speed_kmh": 250, "rpm": 7000}' \
		-w "\nStatus: %{http_code}\n"

test-api-live: ## Test live telemetry API
	@echo "🧪 Testing live telemetry API..."
	curl -s http://localhost:8081/api/v1/live/telemetry?car_id=CAR_001&limit=10 | python -m json.tool

test-api-analytics: ## Test analytics API
	@echo "🧪 Testing analytics API..."
	curl -s http://localhost:8081/api/v1/analytics/fuel?car_id=CAR_001 | python -m json.tool

# ==========================================
# Building & Compilation
# ==========================================

build: ## Build all Docker images
	$(DC) build --no-cache

build-simulator: ## Build ECU simulator image
	$(DC) build ecu-simulator

build-ingestion: ## Build ingestion service image
	$(DC) build ingestion-service

build-processor: ## Build stream processor image
	$(DC) build stream-processor

build-sink: ## Build event sink image
	$(DC) build raw-event-sink

build-api: ## Build query API image
	$(DC) build query-api

# ==========================================
# Code Quality
# ==========================================

lint: ## Run linters on all Python services
	@echo "🔍 Running linters..."
	@for dir in services/*/; do \
		echo "Linting $$dir..."; \
		$(DC) run --rm $$(basename $$dir) pylint .; \
	done

format: ## Format all Python code
	@echo "🎨 Formatting code..."
	@for dir in services/*/; do \
		$(DC) run --rm $$(basename $$dir) black .; \
	done

test: ## Run test suite
	@echo "🧪 Running tests..."
	$(DC) run --rm -T pytest /app/tests -v

coverage: ## Generate test coverage report
	@echo "📊 Generating coverage report..."
	$(DC) run --rm -T pytest /app/tests --cov=app --cov-report=html
	@echo "Coverage report: htmlcov/index.html"

# ==========================================
# Storage & Persistence
# ==========================================

minio-console: ## Open MinIO web console
	@echo "Opening MinIO console at http://localhost:9001"
	@echo "Username: minioadmin"
	@echo "Password: minioadmin"
	open http://localhost:9001

minio-ls: ## List MinIO buckets and objects
	$(DC) exec minio mc ls minio

minio-info: ## Show MinIO info
	$(DC) exec minio mc du minio

# ==========================================
# Cleaning & Reset
# ==========================================

clean: ## Remove stopped containers, dangling images
	@echo "🧹 Cleaning up..."
	docker system prune -f

clean-volumes: ## Remove all volumes (data loss!)
	@echo "⚠️  Removing all volumes (this deletes data)..."
	$(DC) down -v
	@echo "✅ Volumes removed"

clean-all: clean-volumes build ## Full reset (builds fresh images, deletes data)
	@echo "✅ Full reset complete"

reset: clean-volumes up ## Reset to clean state and restart

# ==========================================
# Utilities
# ==========================================

check-deps: ## Check required dependencies
	@echo "🔍 Checking dependencies..."
	@command -v docker >/dev/null 2>&1 && echo "✅ Docker installed" || echo "❌ Docker not found"
	@command -v docker-compose >/dev/null 2>&1 && echo "✅ Docker Compose installed" || echo "❌ Docker Compose not found"
	@command -v git >/dev/null 2>&1 && echo "✅ Git installed" || echo "❌ Git not found"
	@command -v curl >/dev/null 2>&1 && echo "✅ curl installed" || echo "❌ curl not found"

version: ## Show versions of key components
	@echo "🔍 Component Versions"
	@echo "===================="
	@docker --version
	@docker-compose --version

# ==========================================
# Development Workflows
# ==========================================

dev-setup: check-deps build up health ## Full development setup
	@echo "✅ Development environment ready!"

fresh-start: clean-volumes build up generate-sample health ## Fresh start with sample data
	@echo "✅ Fresh start complete! Run 'make health' to verify."

demo: fresh-start ## Run full demo (setup + sample data)
	@echo ""
	@echo "🏁 Demo Ready!"
	@echo "Try these commands:"
	@echo "  make test-api-live"
	@echo "  make nats-watch"
	@echo "  make db-shell"

# ==========================================
# Documentation
# ==========================================

docs: ## Open architecture documentation
	@echo "📚 Opening Architecture.md..."
	open architecture/Architecture.md || less architecture/Architecture.md

api-docs: ## Show API documentation
	@echo "📚 API Documentation"
	@echo "===================="
	@cat dashboards/postman/README.md 2>/dev/null || echo "API docs coming soon"

# ==========================================
# Monitoring & Metrics
# ==========================================

metrics: ## Show all service metrics
	@echo "📊 Ingestion Metrics"
	@curl -s http://localhost:8080/metrics | grep -i ingestion || echo "No metrics available yet"
	@echo ""
	@echo "📊 Processor Metrics"
	@curl -s http://localhost:8080/metrics | grep -i processor || echo "No metrics available yet"

watch-metrics: ## Watch metrics in real-time
	@watch -n 1 'curl -s http://localhost:8080/metrics | grep -E ":\s[0-9]+"'

# Default target
.DEFAULT_GOAL := help
