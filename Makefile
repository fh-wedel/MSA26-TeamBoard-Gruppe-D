.PHONY: up down logs build rebuild ps test clean

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f

build:
	docker compose build

rebuild:
	docker compose build --no-cache

ps:
	docker compose ps

test:
	python tests/smoke_test.py

clean:
	docker compose down -v
	rm -rf data/documents
