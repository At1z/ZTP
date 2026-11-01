.PHONY: up down reset run app-up app-down

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
