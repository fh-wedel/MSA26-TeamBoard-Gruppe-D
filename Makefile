.PHONY: help up up-build down restart logs ps clean clean-volumes \
        seed demo-board-types migrate generate test test-unit test-integration test-shared \
        lint lint-fix build build-services build-frontend \
        shell-postgres shell-redis shell-rabbit urls wait-healthy

SERVICES := auth project task document notification plugin boardregistry

# Default target
help:
	@echo "TeamBoard — Make Targets"
	@echo ""
	@echo "Lifecycle:"
	@echo "  make up                Start all services (cached images)"
	@echo "  make up-build          Rebuild and start all services"
	@echo "  make down              Stop all services (keeps data)"
	@echo "  make restart           Restart services"
	@echo "  make clean             Remove containers and networks"
	@echo "  make clean-volumes     Remove containers, networks, AND volumes (DATA LOSS!)"
	@echo ""
	@echo "Operations:"
	@echo "  make logs              Tail logs from all services"
	@echo "  make logs-<svc>        Tail logs from one service (e.g. logs-auth)"
	@echo "  make ps                Show running containers"
	@echo "  make urls              List all service URLs"
	@echo ""
	@echo "Development:"
	@echo "  make seed              Insert seed data"
	@echo "  make demo-board-types  Register scrum + gantt at runtime via the API (notebook demo)"
	@echo "  make generate          Run sqlc + oapi-codegen for all services"
	@echo "  make test              Run all tests"
	@echo "  make test-unit         Run unit tests only (-short)"
	@echo "  make test-integration  Run integration tests (requires Docker)"
	@echo "  make test-shared       Run shared library tests"
	@echo "  make lint              Lint all services"
	@echo "  make lint-fix          Lint with --fix"
	@echo ""
	@echo "Build:"
	@echo "  make build             Build all images"

up:
	docker compose up -d
	@echo ""
	@echo "Waiting for services to become healthy..."
	@$(MAKE) -s wait-healthy
	@$(MAKE) -s urls

up-build:
	docker compose up -d --build
	@$(MAKE) -s wait-healthy
	@$(MAKE) -s urls

down:
	docker compose down

restart: down up

clean:
	docker compose down --remove-orphans

clean-volumes:
	@echo "WARNING: This will delete all data (databases, MinIO, etc)."
	@read -p "Continue? [y/N] " confirm; \
	if [ "$$confirm" = "y" ] || [ "$$confirm" = "Y" ]; then \
		docker compose down -v --remove-orphans; \
	else \
		echo "Aborted."; \
	fi

wait-healthy:
	@python3 scripts/wait-healthy.py 2>/dev/null || python scripts/wait-healthy.py

logs:
	docker compose logs -f --tail=100

logs-%:
	docker compose logs -f --tail=200 $*

ps:
	docker compose ps

urls:
	@echo ""
	@echo "TeamBoard is running:"
	@echo "  Frontend:           http://localhost:3000"
	@echo "  API (via Gateway):  http://localhost"
	@echo ""
	@echo "Tools:"
	@echo "  Traefik Dashboard:  http://localhost:8080"
	@echo "  RabbitMQ UI:        http://localhost:15672  (teamboard / teamboard)"
	@echo "  MinIO Console:      http://localhost:9001   (teamboard / teamboard-secret)"
	@echo "  MailHog:            http://localhost:8025"
	@echo "  Jaeger UI:          http://localhost:16686"

migrate:
	docker compose run --rm migrate

seed:
	@echo "Seeding test data..."
	@python3 scripts/seed.py 2>/dev/null || python scripts/seed.py

# Demonstrates runtime board-type registration (scrum + gantt) against the running
# stack via the public Board Registry API. The built-in kanban/calendar types come
# from the migration; this notebook adds the rest at runtime. Requires Jupyter.
demo-board-types:
	@echo "Registering scrum + gantt via the Board Registry API (notebook demo)..."
	@command -v jupyter >/dev/null 2>&1 || { echo "jupyter not found — install with 'pip install jupyter', or open docs/demo/board-types.ipynb manually"; exit 1; }
	@jupyter execute docs/demo/board-types.ipynb
	@echo "Done. See docs/demo/board-types.ipynb for the full walkthrough."

generate:
	@for svc in $(SERVICES); do \
		echo "=== Generating $$svc ==="; \
		(cd services/$$svc && sqlc generate) || true; \
	done
	@echo "Generated."

test: test-shared test-unit test-integration

test-unit:
	@for svc in $(SERVICES); do \
		echo "=== Testing $$svc (unit) ==="; \
		(cd services/$$svc && go test -short -race -cover ./...); \
	done

test-integration:
	@for svc in $(SERVICES); do \
		echo "=== Testing $$svc (integration) ==="; \
		(cd services/$$svc && go test -race ./internal/repository/... ./internal/events/... 2>/dev/null || true); \
	done

test-shared:
	@echo "=== Testing shared/go ==="
	(cd shared/go && go test -race -coverprofile=coverage.out ./...)
	(cd shared/go && go tool cover -func=coverage.out | tail -1)

lint:
	@for svc in $(SERVICES); do \
		echo "=== Linting $$svc ==="; \
		(cd services/$$svc && golangci-lint run ./...); \
	done
	(cd shared/go && golangci-lint run ./...)

lint-fix:
	@for svc in $(SERVICES); do \
		(cd services/$$svc && golangci-lint run --fix ./...); \
	done
	(cd shared/go && golangci-lint run --fix ./...)

build: build-services build-frontend

build-services:
	docker compose -f docker-compose.yml build $(SERVICES)

build-frontend:
	docker compose -f docker-compose.yml build frontend

shell-postgres:
	docker compose exec postgres psql -U teamboard

shell-redis:
	docker compose exec redis redis-cli

shell-rabbit:
	docker compose exec rabbitmq rabbitmqctl list_queues
