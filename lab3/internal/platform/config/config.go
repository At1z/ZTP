package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
}

// Load reads configuration from environment variables, falling back to
// defaults matching .env.example. Every service runs on its own database,
// so the caller passes the default database name and port.
func Load(defaultDB, defaultAddr string) Config {
	user := getenv("POSTGRES_USER", "lab3")
	password := getenv("POSTGRES_PASSWORD", "lab3")
	host := getenv("POSTGRES_HOST", "localhost")
	port := getenv("POSTGRES_PORT", "5432")
	name := getenv("SERVICE_DB", defaultDB)

	return Config{
		HTTPAddr: getenv("HTTP_ADDR", defaultAddr),
		DatabaseURL: getenv("DATABASE_URL",
			fmt.Sprintf("postgres://%s:%s@%s:%s/%s", user, password, host, port, name)),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Duration reads a duration such as "15m" from the environment.
func Duration(key string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return fallback
}
