package config

import (
	"fmt"
	"os"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	AMQPURL     string
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

	return Config{
		HTTPAddr: getenv("HTTP_ADDR", ":8080"),
		DatabaseURL: getenv("DATABASE_URL",
			fmt.Sprintf("postgres://%s:%s@%s:%s/%s", pgUser, pgPassword, pgHost, pgPort, pgName)),
		AMQPURL: getenv("AMQP_URL",
			fmt.Sprintf("amqp://%s:%s@%s:%s/", mqUser, mqPassword, mqHost, mqPort)),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
