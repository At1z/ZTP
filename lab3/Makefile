.PHONY: up down reset run-product app-up app-down test test-unit test-integ

# Infrastructure only (PostgreSQL with the products, carts and orders databases).
up:
	docker compose up -d --wait

down:
	docker compose --profile app down

# Drops the volume so migrations run again on the next start.
reset:
	docker compose --profile app down -v

# Product service on the host (needs `make up`).
run-product:
	go run ./cmd/product

# Everything in containers.
app-up:
	docker compose --profile app up -d --build --wait

app-down:
	docker compose --profile app stop product

# Unit tests use in-memory fakes and need no services.
test-unit:
	go test -count=1 ./tests/unit/...

# Integration tests need PostgreSQL from `make up`. Each test creates and drops
# its own temporary database.
test-integ:
	go test -count=1 -v ./tests/integ/...

test: test-unit test-integ
