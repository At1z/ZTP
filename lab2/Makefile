.PHONY: up down reset run app-up app-down test test-unit test-integ

# Infrastructure only (PostgreSQL + RabbitMQ).
up:
	docker compose up -d --wait

down:
	docker compose --profile app down

# Drops volumes so migrations run again on the next start.
reset:
	docker compose --profile app down -v

# API + scheduler on the host (needs `make up`).
run:
	go run ./cmd/api

# Everything in containers: api, worker-email, worker-push.
app-up:
	docker compose --profile app up -d --build --wait

app-down:
	docker compose --profile app stop api worker-email worker-push

# Unit tests use in-memory fakes and need no services.
test-unit:
	go test -count=1 ./tests/unit/...

# Integration tests need PostgreSQL and RabbitMQ from `make up`. They use a
# separate database (lab2_test) and separate queues (lab2test.*).
test-integ:
	go test -count=1 -v ./tests/integ/...

test: test-unit test-integ
