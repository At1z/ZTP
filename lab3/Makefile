.PHONY: up down reset run-product

# Infrastructure only (PostgreSQL with the products, carts and orders databases).
up:
	docker compose up -d --wait

down:
	docker compose down

# Drops the volume so migrations run again on the next start.
reset:
	docker compose down -v

run-product:
	go run ./cmd/product
