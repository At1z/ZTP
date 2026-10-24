package config

import (
	"fmt"
	"os"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
}

// Load reads configuration from environment variables, falling back to
// defaults matching .env.example.
func Load() Config {
	user := getenv("POSTGRES_USER", "lab1")
	password := getenv("POSTGRES_PASSWORD", "lab1")
	name := getenv("POSTGRES_DB", "lab1")
	host := getenv("POSTGRES_HOST", "localhost")
	port := getenv("POSTGRES_PORT", "5432")

	return Config{
		HTTPAddr: getenv("HTTP_ADDR", ":8080"),
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
