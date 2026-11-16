package scheduler

import (
	"context"
	"log"
	"time"

	"lab2/internal/domain"
)

const batchSize = 100

type Repository interface {
	ClaimDue(ctx context.Context, limit int) ([]domain.Notification, error)
	Release(ctx context.Context, id int64) error
}

type Publisher interface {
	Publish(ctx context.Context, channel string, id int64, priority domain.Priority) error
}

// Scheduler moves notifications that reached their send time to the queue of
// their channel.
type Scheduler struct {
	repo     Repository
	queue    Publisher
	interval time.Duration
}

func New(repo Repository, queue Publisher, interval time.Duration) *Scheduler {
	return &Scheduler{repo: repo, queue: queue, interval: interval}
}

// Run polls until ctx is done.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
				log.Printf("scheduler: %v", err)
			}
		}
	}
}

// Tick publishes every due notification once. Rows are marked queued before
// publishing, so a worker never sees a message whose row is still scheduled; a
// notification that cannot be published goes back to scheduled.
func (s *Scheduler) Tick(ctx context.Context) error {
	due, err := s.repo.ClaimDue(ctx, batchSize)
	if err != nil {
		return err
	}
	for _, n := range due {
		if err := s.queue.Publish(ctx, string(n.Channel), n.ID, n.Priority); err != nil {
			log.Printf("scheduler: publish notification %d: %v", n.ID, err)
			if err := s.repo.Release(context.WithoutCancel(ctx), n.ID); err != nil {
				log.Printf("scheduler: release notification %d: %v", n.ID, err)
			}
		}
	}
	return nil
}
