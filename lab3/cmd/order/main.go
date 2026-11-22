package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lab3/internal/order/handler"
	"lab3/internal/order/repository"
	"lab3/internal/order/service"
	"lab3/internal/platform/config"
	"lab3/internal/platform/db"
)

func main() {
	cfg := config.Load("orders", ":8083")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	orders := repository.NewOrderRepository(pool)
	notifications := repository.NewNotificationRepository(pool)

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: handler.NewRouter(
			service.NewOrderQueryService(orders, notifications),
			service.NewOrderCommandService(orders),
			pool.Ping,
		),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("order service listening on %s", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
}
