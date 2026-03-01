.PHONY: up down build logs logs-app logs-db restart db-shell app-shell \
       migrate-up migrate-down migrate-create migrate-status

include .env
export

DB_URL = postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432/$(POSTGRES_DB)?sslmode=disable

up:
	docker compose up -d --build

down:
	docker compose down

build:
	docker compose build

logs:
	docker compose logs -f

logs-app:
	docker compose logs -f app

logs-db:
	docker compose logs -f db

restart:
	docker compose restart

db-shell:
	docker exec -it taskforge_db psql -U taskforge -d taskforge

app-shell:
	docker exec -it taskforge_app sh

migrate-up:
	docker run --rm --network host -v $(PWD)/migrations:/migrations migrate/migrate \
		-path=/migrations -database "$(DB_URL)" up

migrate-down:
	docker run --rm --network host -v $(PWD)/migrations:/migrations migrate/migrate \
		-path=/migrations -database "$(DB_URL)" down 1

migrate-status:
	docker run --rm --network host -v $(PWD)/migrations:/migrations migrate/migrate \
		-path=/migrations -database "$(DB_URL)" version

migrate-create:
	docker run --rm -v $(PWD)/migrations:/migrations migrate/migrate \
		create -ext sql -dir /migrations -seq $(name)
