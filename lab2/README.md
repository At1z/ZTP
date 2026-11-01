# Lab 2 – Notification system (Go + PostgreSQL + RabbitMQ)

## Project structure

```
lab2/
├── cmd/
│   ├── api/main.go            # REST API + scheduler
│   └── worker/main.go         # delivery worker, one channel per instance
├── internal/
│   ├── config/                # env configuration
│   ├── db/                    # PostgreSQL connection pool
│   ├── domain/                # shared types and errors
│   ├── handler/               # presentation layer (HTTP, JSON)
│   ├── service/               # business logic, validation, quiet hours
│   ├── repository/            # data access (SQL)
│   ├── queue/                 # RabbitMQ: queues, publish, consume, retry
│   ├── scheduler/             # moves due notifications to the queues
│   ├── worker/                # processes one message (attempts, retries)
│   └── sender/                # simulated delivery
├── migrations/001_init.sql    # database schema (notifications table)
├── Dockerfile                 # builds the api or the worker image
├── docker-compose.yml         # PostgreSQL, RabbitMQ, api, worker-email, worker-push
├── .env.example               # settings template
├── Makefile                   # up, down, reset, run, app-up, app-down
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

## Step 3 Endpoints and business logic

**Run in containers:** `make app-up` starts the API (with the scheduler) and two worker instances, `worker-email` and `worker-push`, next to PostgreSQL and RabbitMQ. Follow the deliveries with `docker compose logs -f worker-email worker-push`. Stop with `make app-down`.
**Run locally:** `make up && make run` (API only; add a worker with `CHANNEL=email go run ./cmd/worker`).

**Layers (N-Layer):** `handler` (HTTP/JSON, status codes) → `service` (validation, quiet hours) → `repository` (SQL). The scheduler and the worker use the repository and the broker through small interfaces, wired in `cmd/api/main.go` and `cmd/worker/main.go`.

| Method | Path | Description | Success |
|---|---|---|---|
| POST | `/api/v1/notifications` | create a notification | 201 |
| GET | `/api/v1/notifications` | list notifications | 200 |
| GET | `/api/v1/notifications/{id}` | get one notification | 200 |
| POST | `/api/v1/notifications/{id}/send-now` | deliver a scheduled notification immediately | 200 |
| POST | `/api/v1/notifications/{id}/cancel` | cancel a scheduled or queued notification | 200 |

**Body:** `{"channel": "email", "content": "Hello", "recipient": "ann@example.com", "timezone": "Europe/Warsaw", "priority": "high", "send_at": "2030-01-01T12:00:00Z"}`. `priority` (`low`/`high`, default `low`) and `send_at` (RFC 3339, default now) are optional. Channels: `email` (recipient must contain `@`), `push`.

**Errors** use the format `{"errors": [{"field": "channel", "message": "..."}]}`: `400` malformed JSON / bad id, `404` not found, `409` the status does not allow the operation, `422` rule violation (all invalid fields listed at once).

**How it works**
1. `POST` stores the notification (`scheduled`) in PostgreSQL, so it is kept regardless of the load.
2. The scheduler in the API polls every second (`SCHEDULER_INTERVAL`), marks due notifications `queued` (`FOR UPDATE SKIP LOCKED`, high priority first) and publishes them to the queue of their channel (`notifications.email`, `notifications.push`) with publisher confirms. If publishing fails, the notification goes back to `scheduled`.
3. Each worker instance consumes only its own channel, so the channels work independently. Queues are durable, messages persistent, and queues have `x-max-priority`, so `high` messages overtake `low` ones.
4. **Exactly once:** the worker counts an attempt only for a notification that is still `queued`, and marks it `sent` once; duplicate, stale or cancelled messages are acknowledged and dropped.
5. **Retries:** a failed delivery is retried through the `notifications.<channel>.retry` queue (message TTL = `RETRY_DELAY`, dead-lettered back to the main queue). After 3 attempts the status becomes `failed` and `last_error` holds the reason.
6. **Quiet hours:** notifications are delivered only between 08:00 and 22:00 in the recipient's `timezone`; otherwise `scheduled_at` moves to the nearest 08:00 local time. `send-now` skips both the planned time and the quiet hours.

Delivery is simulated: a worker logs the notification. `FAIL_RATE` (0..1) makes a share of deliveries fail, e.g. `FAIL_RATE=1 RETRY_DELAY=1s docker compose --profile app up -d --force-recreate worker-email` shows 3 attempts and then `failed`.

Known limit: if the API crashes between marking a notification `queued` and publishing it, it stays `queued` and is not published again.

Example:
```
curl -i -X POST localhost:8080/api/v1/notifications -H 'Content-Type: application/json' \
  -d '{"channel":"email","content":"Hello","recipient":"ann@example.com","timezone":"Europe/Warsaw","priority":"high"}'
```
