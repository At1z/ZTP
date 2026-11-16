package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"lab2/internal/config"
	"lab2/internal/db"
	"lab2/internal/queue"
	"lab2/internal/repository"
	"lab2/internal/sender"
	"lab2/internal/worker"
)

// One worker instance serves exactly one channel (CHANNEL=email|push).
func main() {
	cfg := config.Load()
	if !slices.Contains(queue.Channels, cfg.Channel) {
		log.Fatalf("CHANNEL must be one of %v, got %q", queue.Channels, cfg.Channel)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	broker, err := queue.Connect(cfg.AMQPURL, cfg.QueuePrefix)
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer broker.Close()

	processor := worker.NewProcessor(
		repository.NewNotificationRepository(pool),
		sender.Simulated{FailRate: cfg.FailRate},
		broker,
		cfg.RetryDelay,
	)

	log.Printf("worker for channel %q started", cfg.Channel)
	if err := broker.Consume(ctx, cfg.Channel, processor.Process); err != nil {
		log.Fatalf("consume: %v", err)
	}
}
