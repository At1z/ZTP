package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	AMQPURL     string
	QueuePrefix string // queues are named <prefix>.<channel>

	SchedulerInterval time.Duration // how often the API looks for due notifications
	RetryDelay        time.Duration // wait before a failed delivery is retried

	Channel  string  // worker only: the channel this instance serves
	FailRate float64 // worker only: share of simulated deliveries that fail
}

// Load reads configuration from environment variables, falling back to
// defaults matching .env.example.
func Load() Config {
	pgUser := getenv("POSTGRES_USER", "lab2")
	pgPassword := getenv("POSTGRES_PASSWORD", "lab2")
	pgName := getenv("POSTGRES_DB", "lab2")
	pgHost := getenv("POSTGRES_HOST", "localhost")
	pgPort := getenv("POSTGRES_PORT", "5432")

	mqUser := getenv("RABBITMQ_USER", "lab2")
	mqPassword := getenv("RABBITMQ_PASSWORD", "lab2")
	mqHost := getenv("RABBITMQ_HOST", "localhost")
	mqPort := getenv("RABBITMQ_PORT", "5672")

	failRate, _ := strconv.ParseFloat(getenv("FAIL_RATE", "0"), 64)

	return Config{
		HTTPAddr: getenv("HTTP_ADDR", ":8080"),
		DatabaseURL: getenv("DATABASE_URL",
			fmt.Sprintf("postgres://%s:%s@%s:%s/%s", pgUser, pgPassword, pgHost, pgPort, pgName)),
		AMQPURL: getenv("AMQP_URL",
			fmt.Sprintf("amqp://%s:%s@%s:%s/", mqUser, mqPassword, mqHost, mqPort)),
		QueuePrefix:       getenv("QUEUE_PREFIX", "notifications"),
		SchedulerInterval: getDuration("SCHEDULER_INTERVAL", time.Second),
		RetryDelay:        getDuration("RETRY_DELAY", 5*time.Second),
		Channel:           os.Getenv("CHANNEL"),
		FailRate:          failRate,
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return fallback
}
