package service

import (
	"context"

	"lab3/internal/order/domain"

	"github.com/google/uuid"
)

// OrderQueryService is the query side: it only reads.
type OrderQueryService struct {
	orders        domain.OrderRepository
	notifications domain.NotificationRepository
}

func NewOrderQueryService(orders domain.OrderRepository, notifications domain.NotificationRepository) *OrderQueryService {
	return &OrderQueryService{orders: orders, notifications: notifications}
}

type GetOrderQuery struct {
	UserID  uuid.UUID
	OrderID uuid.UUID
}

func (s *OrderQueryService) GetOrder(ctx context.Context, q GetOrderQuery) (domain.Order, error) {
	return s.orders.Get(ctx, q.UserID, q.OrderID)
}

type ListNotificationsQuery struct {
	UserID uuid.UUID
}

func (s *OrderQueryService) ListNotifications(ctx context.Context, q ListNotificationsQuery) ([]domain.Notification, error) {
	return s.notifications.ListByUser(ctx, q.UserID)
}
