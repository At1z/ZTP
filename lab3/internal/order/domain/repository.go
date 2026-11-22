package domain

import (
	"context"

	"github.com/google/uuid"
)

type OrderRepository interface {
	// Create stores the order, its items and the notification in one
	// transaction. It is idempotent per cart: when the cart already has an
	// order, that order is returned unchanged and created is false.
	Create(ctx context.Context, order Order, notification Notification) (stored Order, created bool, err error)

	// Get returns the order only if it belongs to the user.
	Get(ctx context.Context, userID, orderID uuid.UUID) (Order, error)
}

type NotificationRepository interface {
	// ListByUser returns the user's notifications, newest first.
	ListByUser(ctx context.Context, userID uuid.UUID) ([]Notification, error)
}
