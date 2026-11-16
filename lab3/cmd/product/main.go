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

	"lab3/internal/platform/config"
	"lab3/internal/platform/db"
	"lab3/internal/product/handler"
	"lab3/internal/product/repository"
	"lab3/internal/product/service"
)

func main() {
	cfg := config.Load("products", ":8081")
	ttl := config.Duration("RESERVATION_TTL", 15*time.Minute)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	products := repository.NewProductRepository(pool)
	reservations := repository.NewReservationRepository(pool)

	router := handler.NewRouter(
		service.NewProductService(products),
		service.NewReservationService(reservations, ttl),
		pool.Ping,
	)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("product service listening on %s (reservation ttl %s)", cfg.HTTPAddr, ttl)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
}
