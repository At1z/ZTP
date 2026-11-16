package worker

import (
	"context"
	"time"

	"lab2/internal/domain"
)

type Repository interface {
	StartAttempt(ctx context.Context, id int64) (n domain.Notification, ok bool, err error)
	MarkSent(ctx context.Context, id int64) error
	RecordFailure(ctx context.Context, id int64, msg string) error
	MarkFailed(ctx context.Context, id int64, msg string) error
}

type Sender interface {
	Send(ctx context.Context, n domain.Notification) error
}

type Retrier interface {
	Retry(ctx context.Context, channel string, id int64, priority domain.Priority, delay time.Duration) error
}

// Processor delivers the notifications named by queue messages.
type Processor struct {
	repo       Repository
	sender     Sender
	retrier    Retrier
	retryDelay time.Duration
}

func NewProcessor(repo Repository, sender Sender, retrier Retrier, retryDelay time.Duration) *Processor {
	return &Processor{repo: repo, sender: sender, retrier: retrier, retryDelay: retryDelay}
}

// Process handles one message. A nil result means the message can be
// acknowledged; an error makes the broker deliver it again.
//
// Exactly-once delivery rests on the database: an attempt is only counted for
// a notification that is still queued, and it becomes sent only once. Duplicate
// or stale messages (already sent, failed or cancelled) are dropped.
func (p *Processor) Process(ctx context.Context, id int64) error {
	n, ok, err := p.repo.StartAttempt(ctx, id)
	if err != nil || !ok {
		return err
	}

	sendErr := p.sender.Send(ctx, n)
	if sendErr == nil {
		return p.repo.MarkSent(ctx, id)
	}
	if n.Attempts >= domain.MaxAttempts {
		return p.repo.MarkFailed(ctx, id, sendErr.Error())
	}
	if err := p.repo.RecordFailure(ctx, id, sendErr.Error()); err != nil {
		return err
	}
	return p.retrier.Retry(ctx, string(n.Channel), id, n.Priority, p.retryDelay)
}
