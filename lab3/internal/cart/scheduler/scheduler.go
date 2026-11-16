package scheduler

import (
	"context"
	"log"
	"time"
)

type Expirer interface {
	ExpireCarts(ctx context.Context) (int, error)
}

// Run expires carts every interval until ctx is cancelled. Several instances
// can run it at once: each expired cart is handed to exactly one of them.
func Run(ctx context.Context, e Expirer, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := e.ExpireCarts(ctx)
			if err != nil {
				log.Printf("expire carts: %v", err)
			} else if n > 0 {
				log.Printf("expired %d carts", n)
			}
		}
	}
}
