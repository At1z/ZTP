package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"lab3/internal/platform/config"
	"lab3/internal/platform/db"
)

func main() {
	cfg := config.Load("products", ":8081")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	var products int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM products").Scan(&products); err != nil {
		log.Fatalf("query: %v", err)
	}
	log.Printf("product service connected to database, %d products", products)
}
