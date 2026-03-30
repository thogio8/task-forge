.PHONY: up down build test test-unit test-integration test-all logs logs-worker logs-db restart db-shell \
       migrate-up migrate-down migrate-create migrate-status migrate-force bench test-race pprof-goroutine \
       kafka-topics kafka-consume

include .env
export

DB_URL = postgres://$(DB_USER):$(DB_PASSWORD)@localhost:$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSL_MODE)

up:
	docker compose up -d --build

down:
	docker compose down

build:
	docker compose build

test: test-unit

test-unit:
	go test ./... -v -count=1

test-integration:
	DB_HOST=localhost go test ./... -v -count=1 -tags=integration

test-all: test-unit test-integration

logs:
	docker compose logs -f

logs-worker:
	docker compose logs -f worker-1 worker-2 worker-3

logs-db:
	docker compose logs -f db

restart:
	docker compose restart

db-shell:
	docker compose exec db psql -U $(DB_USER) -d $(DB_NAME)

migrate-up:
	docker run --rm --network host -v $(PWD)/migrations:/migrations migrate/migrate \
		-path=/migrations -database "$(DB_URL)" up

migrate-down:
	docker run --rm --network host -v $(PWD)/migrations:/migrations migrate/migrate \
		-path=/migrations -database "$(DB_URL)" down 1

migrate-status:
	docker run --rm --network host -v $(PWD)/migrations:/migrations migrate/migrate \
		-path=/migrations -database "$(DB_URL)" version

migrate-force:
	docker run --rm --network host -v $(PWD)/migrations:/migrations migrate/migrate \
		-path=/migrations -database "$(DB_URL)" force $(version)

migrate-create:
	docker run --rm -v $(PWD)/migrations:/migrations migrate/migrate \
		create -ext sql -dir /migrations -seq $(name)

bench:
	go test ./internal/worker/ -bench=. -benchmem -count=1

test-race:
	go test -race ./... -count=1

pprof-goroutine:
	go tool pprof http://localhost:$(HTTP_PORT)/debug/pprof/goroutine

kafka-topics:
	docker compose exec kafka kafka-topics --bootstrap-server localhost:9092 --list

kafka-consume:
	docker compose exec kafka kafka-console-consumer --bootstrap-server localhost:9092 --topic tasks --from-beginning
