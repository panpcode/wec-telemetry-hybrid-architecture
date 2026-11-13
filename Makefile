.PHONY: help up down restart logs ps health test vet build clean-volumes reset \
        db-shell laps alerts streams s3-ls dashboard filer

DC := docker compose
SERVICES := ecu-simulator ingestion-service cloud-uploader dashboard stream-processor

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ── Stack ────────────────────────────────────────────────────────────────────

up: ## Build and start the whole stack in the background
	$(DC) up -d --build

down: ## Stop the stack (keeps data volumes)
	$(DC) down

restart: down up ## Restart the stack

logs: ## Follow logs from all services (make logs s=stream-processor for one)
	$(DC) logs -f $(s)

ps: ## Show container status
	$(DC) ps

health: ## Check each service endpoint
	@curl -sf -o /dev/null http://localhost:8080/health && echo "✅ ingestion"   || echo "❌ ingestion"
	@curl -sf -o /dev/null http://localhost:8222/healthz && echo "✅ nats"       || echo "❌ nats"
	@curl -sf -o /dev/null http://localhost:3000        && echo "✅ dashboard"   || echo "❌ dashboard"
	@curl -sf -o /dev/null http://localhost:9333/cluster/status 2>/dev/null || curl -sf -o /dev/null http://localhost:8888 && echo "✅ seaweedfs" || echo "❌ seaweedfs"
	@$(DC) exec -T postgres pg_isready -U postgres -d telemetry >/dev/null 2>&1 && echo "✅ postgres" || echo "❌ postgres"

clean-volumes: ## Stop the stack and DELETE all data volumes
	$(DC) down -v

reset: clean-volumes up ## Wipe data and start fresh

# ── Go ───────────────────────────────────────────────────────────────────────

test: ## Run Go tests for every service
	@for s in $(SERVICES); do (cd services/$$s && go test ./...) || exit 1; done

vet: ## Run go vet for every service
	@for s in $(SERVICES); do (cd services/$$s && go vet ./...) || exit 1; done

build: ## Build all Docker images
	$(DC) build

# ── Data ─────────────────────────────────────────────────────────────────────

db-shell: ## Open a psql shell on the telemetry database
	$(DC) exec postgres psql -U postgres -d telemetry

laps: ## Show the latest 20 completed laps
	@$(DC) exec -T postgres psql -U postgres -d telemetry -c \
	  "SELECT car_id, lap, round(lap_time_s::numeric,1) AS lap_s, round(fuel_used_l::numeric,2) AS fuel_l, \
	          round(avg_speed_kmh::numeric) AS avg_kmh, round(max_tire_c::numeric) AS max_tire, \
	          round(max_brake_c::numeric) AS max_brake, pitted \
	   FROM lap_metrics ORDER BY ended_at DESC LIMIT 20"

alerts: ## Show the latest 20 alerts
	@$(DC) exec -T postgres psql -U postgres -d telemetry -c \
	  "SELECT raised_at, car_id, lap, kind, detail FROM alerts ORDER BY raised_at DESC LIMIT 20"

streams: ## Show JetStream stream and consumer state
	@curl -s 'http://localhost:8222/jsz?streams=true&consumers=true' | python3 -m json.tool

s3-ls: ## List archived telemetry files in the bucket
	@curl -s 'http://localhost:8888/buckets/wec-telemetry/?pretty=y' -H 'Accept: application/json' | python3 -c \
	  'import json,sys; [print(e["FullPath"]) for e in (json.load(sys.stdin).get("Entries") or [])]'

dashboard: ## Open the pit-wall dashboard
	open http://localhost:3000

filer: ## Open the SeaweedFS file browser (archived telemetry)
	open http://localhost:8888/buckets/wec-telemetry/

.DEFAULT_GOAL := help
