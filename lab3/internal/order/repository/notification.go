package repository

import (
	"context"
	"fmt"

	"lab3/internal/order/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NotificationRepository struct {
	pool *pgxpool.Pool
}

func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

func (r *NotificationRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.Notification, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, order_id, message, created_at FROM notifications
		WHERE user_id = $1 ORDER BY created_at DESC, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[domain.Notification])
	if err != nil {
		return nil, fmt.Errorf("scan notifications: %w", err)
	}
	return list, nil
}
