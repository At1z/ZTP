.PHONY: up down reset run

up:
	docker compose up -d --wait

down:
	docker compose down

# Drops volumes so migrations run again on the next start.
reset:
	docker compose down -v

run:
	go run ./cmd/api
