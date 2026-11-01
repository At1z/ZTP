package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lab2/internal/domain"
)

const notificationColumns = "id, channel, content, recipient, timezone, priority, status, " +
	"scheduled_at, attempts, last_error, created_at, sent_at"

type NotificationRepository struct {
	pool *pgxpool.Pool
}

func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

type scanner interface {
	Scan(dest ...any) error
}

func scanNotification(row scanner) (domain.Notification, error) {
	var n domain.Notification
	err := row.Scan(&n.ID, &n.Channel, &n.Content, &n.Recipient, &n.Timezone, &n.Priority,
		&n.Status, &n.ScheduledAt, &n.Attempts, &n.LastError, &n.CreatedAt, &n.SentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return n, domain.ErrNotFound
	}
	return n, err
}

func (r *NotificationRepository) Create(ctx context.Context, n *domain.Notification) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO notifications (channel, content, recipient, timezone, priority, scheduled_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, status, created_at`,
		n.Channel, n.Content, n.Recipient, n.Timezone, n.Priority, n.ScheduledAt,
	).Scan(&n.ID, &n.Status, &n.CreatedAt)
}

func (r *NotificationRepository) GetByID(ctx context.Context, id int64) (domain.Notification, error) {
	return scanNotification(r.pool.QueryRow(ctx,
		"SELECT "+notificationColumns+" FROM notifications WHERE id = $1", id))
}

func (r *NotificationRepository) List(ctx context.Context) ([]domain.Notification, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+notificationColumns+" FROM notifications ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []domain.Notification{}
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, n)
	}
	return list, rows.Err()
}

// Cancel stops a notification that has not been sent yet. It returns
// ErrNotFound for an unknown id and ErrInvalidState if it is already final.
func (r *NotificationRepository) Cancel(ctx context.Context, id int64) (domain.Notification, error) {
	n, err := scanNotification(r.pool.QueryRow(ctx,
		`UPDATE notifications SET status = 'cancelled'
		 WHERE id = $1 AND status IN ('scheduled', 'queued')
		 RETURNING `+notificationColumns, id))
	if errors.Is(err, domain.ErrNotFound) {
		return n, r.missingOrInvalid(ctx, id)
	}
	return n, err
}

// SendNow makes a scheduled notification due immediately; the scheduler picks
// it up on its next tick.
func (r *NotificationRepository) SendNow(ctx context.Context, id int64) (domain.Notification, error) {
	n, err := scanNotification(r.pool.QueryRow(ctx,
		`UPDATE notifications SET scheduled_at = now()
		 WHERE id = $1 AND status = 'scheduled'
		 RETURNING `+notificationColumns, id))
	if errors.Is(err, domain.ErrNotFound) {
		return n, r.missingOrInvalid(ctx, id)
	}
	return n, err
}

func (r *NotificationRepository) missingOrInvalid(ctx context.Context, id int64) error {
	if _, err := r.GetByID(ctx, id); err != nil {
		return err
	}
	return domain.ErrInvalidState
}

// ClaimDue atomically moves due notifications (high priority first) from
// scheduled to queued. FOR UPDATE SKIP LOCKED lets several schedulers run
// without claiming the same row twice.
func (r *NotificationRepository) ClaimDue(ctx context.Context, limit int) ([]domain.Notification, error) {
	rows, err := r.pool.Query(ctx,
		`UPDATE notifications SET status = 'queued'
		 WHERE id IN (
		     SELECT id FROM notifications
		     WHERE status = 'scheduled' AND scheduled_at <= now()
		     ORDER BY (priority = 'high') DESC, scheduled_at
		     LIMIT $1
		     FOR UPDATE SKIP LOCKED)
		 RETURNING `+notificationColumns, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claimed []domain.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, n)
	}
	return claimed, rows.Err()
}

// Release puts a claimed notification back to scheduled after publishing failed.
func (r *NotificationRepository) Release(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx,
		"UPDATE notifications SET status = 'scheduled' WHERE id = $1 AND status = 'queued'", id)
	return err
}

// StartAttempt counts a delivery attempt. ok is false when the notification is
// no longer queued (already sent, failed or cancelled), so a duplicate or stale
// message is dropped instead of being delivered twice.
func (r *NotificationRepository) StartAttempt(ctx context.Context, id int64) (n domain.Notification, ok bool, err error) {
	n, err = scanNotification(r.pool.QueryRow(ctx,
		`UPDATE notifications SET attempts = attempts + 1
		 WHERE id = $1 AND status = 'queued'
		 RETURNING `+notificationColumns, id))
	if errors.Is(err, domain.ErrNotFound) {
		return n, false, nil
	}
	return n, err == nil, err
}

func (r *NotificationRepository) MarkSent(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET status = 'sent', sent_at = now(), last_error = ''
		 WHERE id = $1 AND status = 'queued'`, id)
	return err
}

func (r *NotificationRepository) RecordFailure(ctx context.Context, id int64, msg string) error {
	_, err := r.pool.Exec(ctx,
		"UPDATE notifications SET last_error = $2 WHERE id = $1 AND status = 'queued'", id, msg)
	return err
}

func (r *NotificationRepository) MarkFailed(ctx context.Context, id int64, msg string) error {
	_, err := r.pool.Exec(ctx,
		"UPDATE notifications SET status = 'failed', last_error = $2 WHERE id = $1 AND status = 'queued'", id, msg)
	return err
}
