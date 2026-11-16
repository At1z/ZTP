package service

import (
	"context"
	"fmt"

	"lab3/internal/order/domain"

	"github.com/google/uuid"
)

// OrderCommandService is the command side: every method changes state.
type OrderCommandService struct {
	orders domain.OrderRepository
}

func NewOrderCommandService(orders domain.OrderRepository) *OrderCommandService {
	return &OrderCommandService{orders: orders}
}

type CreateOrderCommand struct {
	UserID uuid.UUID
	CartID uuid.UUID
	Items  []domain.Item
}

// CreateOrder starts the fulfilment of an order and notifies the user about it.
// The total is computed here from the lines, never taken from the caller.
// Calling it again for the same cart returns the existing order, so the caller
// can safely retry.
func (s *OrderCommandService) CreateOrder(ctx context.Context, cmd CreateOrderCommand) (order domain.Order, created bool, err error) {
	switch {
	case cmd.UserID == uuid.Nil:
		return domain.Order{}, false, fmt.Errorf("%w: user_id is required", domain.ErrInvalidInput)
	case cmd.CartID == uuid.Nil:
		return domain.Order{}, false, fmt.Errorf("%w: cart_id is required", domain.ErrInvalidInput)
	case len(cmd.Items) == 0:
		return domain.Order{}, false, fmt.Errorf("%w: an order needs at least one item", domain.ErrInvalidInput)
	}
	for _, it := range cmd.Items {
		switch {
		case it.ProductID == uuid.Nil:
			return domain.Order{}, false, fmt.Errorf("%w: product_id is required", domain.ErrInvalidInput)
		case it.Quantity <= 0:
			return domain.Order{}, false, fmt.Errorf("%w: quantity must be positive", domain.ErrInvalidInput)
		case it.UnitPriceCents < 0:
			return domain.Order{}, false, fmt.Errorf("%w: unit price must not be negative", domain.ErrInvalidInput)
		}
	}

	order = domain.Order{
		ID:     uuid.New(),
		UserID: cmd.UserID,
		CartID: cmd.CartID,
		Status: domain.StatusProcessing,
		Items:  cmd.Items,
	}
	order.TotalCents = order.ComputeTotalCents()

	notification := domain.Notification{
		ID:      uuid.New(),
		UserID:  cmd.UserID,
		OrderID: order.ID,
		Message: fmt.Sprintf("Rozpoczęto realizację zamówienia %s", order.ID),
	}
	return s.orders.Create(ctx, order, notification)
}
