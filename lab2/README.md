# Lab 2 – Notification system (Go + PostgreSQL + RabbitMQ)

## Project structure

```
lab2/
├── cmd/api/main.go            # entry point, graceful shutdown
├── internal/
│   ├── config/                # env configuration
│   ├── db/                    # PostgreSQL connection pool
│   ├── queue/                 # RabbitMQ connection, queue declaration
│   └── handler/               # HTTP (health check for now)
├── migrations/001_init.sql    # database schema (notifications table)
├── docker-compose.yml         # PostgreSQL 16 + RabbitMQ 3 (management UI)
├── .env.example               # settings template
├── Makefile                   # up, down, reset, run
├── go.mod / go.sum
└── README.md
```

## Step 1 Docker environment with PostgreSQL and RabbitMQ

**Requirements:** Docker + Docker Compose.

1. Copy the config file: `cp .env.example .env` (adjust values if needed).
2. Start the services: `docker compose up -d --wait` (or `make up`).
3. Check their state: `docker compose ps` – both should be `healthy`.
4. Open `psql`: `docker compose exec postgres psql -U lab2 -d lab2`, then `\dt` lists the `notifications` table.
5. Open the RabbitMQ management UI at http://localhost:15672 (login `lab2` / `lab2`). AMQP listens on `localhost:5672`.

The schema comes from `migrations/001_init.sql` – Postgres runs it automatically **only on first start** (empty volume). To start over: `docker compose down -v && docker compose up -d` (or `make reset && make up`).

To stop: `docker compose down` (data stays in the `pgdata` and `rabbitmqdata` volumes).

## Step 2 Go connection to PostgreSQL and RabbitMQ

**Requirements:** Go 1.22+ (`brew install go`), the services from Step 1 running.

1. Start the services: `make up`.
2. Run the API: `go run ./cmd/api`.
3. Check the connections: `curl -i localhost:8080/health` – expect `200` and `{"status":"ok"}`.
4. Stop the database (`docker compose stop postgres`) and call `/health` again – expect `503 database unavailable`. Start it again and stop RabbitMQ (`docker compose stop rabbitmq`) – expect `503 broker unavailable` (the API must be restarted if RabbitMQ was down, there is no reconnect yet).
5. In the RabbitMQ UI (Queues tab) there are two durable queues, `notifications.email` and `notifications.push`, both with max priority 10.

Configuration is read from environment variables (defaults match `.env.example`): `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_HOST`, `POSTGRES_PORT`, or a full `DATABASE_URL`; `RABBITMQ_USER`, `RABBITMQ_PASSWORD`, `RABBITMQ_HOST`, `RABBITMQ_PORT`, or a full `AMQP_URL`; the HTTP address is set with `HTTP_ADDR` (default `:8080`).

Structure: `cmd/api/main.go` (startup, graceful shutdown on Ctrl+C), `internal/config` (env loading), `internal/db` (`pgxpool` connection + ping), `internal/queue` (AMQP connection; declares one queue per channel on start).
