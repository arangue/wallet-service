include .env
export

APP_NAME=wallet-app

.PHONY: help run dev build stop restart logs test test-unit bench lint fmt tidy clean migrate

help:
	@echo "Available commands:"
	@echo "  make run        -> Build and start everything in Docker"
	@echo "  make dev        -> Run DB in Docker and app locally (fast development)"
	@echo "  make build      -> Build docker images"
	@echo "  make stop       -> Stop containers"
	@echo "  make restart    -> Restart containers"
	@echo "  make logs       -> Follow container logs"
	@echo "  make test       -> Run all tests (requires DB via make dev)"
	@echo "  make test-unit  -> Run unit tests only (no DB required)"
	@echo "  make bench      -> Run benchmarks (requires DB via make dev)"
	@echo "  make lint       -> Run golangci-lint"
	@echo "  make fmt        -> Format Go code"
	@echo "  make tidy       -> Run go mod tidy"
	@echo "  make migrate    -> Run database migrations"
	@echo "  make clean      -> Remove containers and volumes"

run:
	docker compose up --build

dev:
	docker compose up -d wallet-db
	go run ./cmd/server

build:
	docker compose build

stop:
	docker compose down

restart:
	docker compose down && docker compose up --build

logs:
	docker compose logs -f

test:
	TEST_DATABASE_URL=$(DATABASE_URL) go test ./...

test-unit:
	go test ./...

bench:
	TEST_DATABASE_URL=$(DATABASE_URL) go test -bench=. -benchmem -run='^$$' ./internal/infrastructure/postgres/

lint:
	golangci-lint run

fmt:
	go fmt ./...

tidy:
	go mod tidy

migrate:
	migrate -path internal/infrastructure/postgres/migrations -database "$(DATABASE_URL)" up

clean:
	docker compose down -v
