package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"lab2/internal/config"
	"lab2/internal/db"
	"lab2/internal/handler"
	"lab2/internal/queue"
	"lab2/internal/repository"
	"lab2/internal/scheduler"
	"lab2/internal/service"
)

func main() {
	cfg := config.Load()

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

	repo := repository.NewNotificationRepository(pool)

	// The scheduler uses the pool and the broker, so main waits for it before
	// the deferred Close calls run.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scheduler.New(repo, broker, cfg.SchedulerInterval).Run(ctx)
	}()
	defer wg.Wait()

	router := handler.NewRouter(service.NewNotificationService(repo, time.Now), pool, broker)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// ListenAndServe returns as soon as Shutdown is called, so main must wait
	// for shutdownDone or in-flight requests would be cut off by pool.Close().
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("listening on %s", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
	<-shutdownDone
}
