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

	"lab3/internal/cart/client"
	"lab3/internal/cart/handler"
	"lab3/internal/cart/repository"
	"lab3/internal/cart/scheduler"
	"lab3/internal/cart/service"
	"lab3/internal/platform/config"
	"lab3/internal/platform/db"
)

func main() {
	cfg := config.Load("carts", ":8082")
	cartTTL := config.Duration("CART_TTL", 15*time.Minute)
	sweepInterval := config.Duration("CART_SWEEP_INTERVAL", 10*time.Second)
	productURL := config.String("PRODUCT_SERVICE_URL", "http://localhost:8081")
	orderURL := config.String("ORDER_SERVICE_URL", "http://localhost:8083")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	carts := repository.NewCartRepository(pool)
	httpClient := &http.Client{Timeout: 5 * time.Second}
	products := client.NewProductClient(productURL, httpClient)
	orders := client.NewOrderClient(orderURL, httpClient)
	commands := service.NewCartCommandService(carts, products, orders, cartTTL)

	go scheduler.Run(ctx, commands, sweepInterval)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler.NewRouter(service.NewCartQueryService(carts), commands, pool.Ping),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("cart service listening on %s (cart ttl %s, product service %s, order service %s)", cfg.HTTPAddr, cartTTL, productURL, orderURL)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
}
