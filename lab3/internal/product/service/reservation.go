package service

import (
	"context"
	"fmt"
	"time"

	"lab3/internal/product/domain"

	"github.com/google/uuid"
)

// ReservationService is the command side: every method changes state.
type ReservationService struct {
	reservations domain.ReservationRepository
	ttl          time.Duration
}

func NewReservationService(reservations domain.ReservationRepository, ttl time.Duration) *ReservationService {
	return &ReservationService{reservations: reservations, ttl: ttl}
}

type ReserveProductCommand struct {
	ProductID uuid.UUID
	CartID    uuid.UUID
	UserID    uuid.UUID
	Quantity  int
}

func (s *ReservationService) Reserve(ctx context.Context, cmd ReserveProductCommand) (domain.Reservation, error) {
	switch {
	case cmd.ProductID == uuid.Nil:
		return domain.Reservation{}, fmt.Errorf("%w: product_id is required", domain.ErrInvalidInput)
	case cmd.CartID == uuid.Nil:
		return domain.Reservation{}, fmt.Errorf("%w: cart_id is required", domain.ErrInvalidInput)
	case cmd.UserID == uuid.Nil:
		return domain.Reservation{}, fmt.Errorf("%w: user_id is required", domain.ErrInvalidInput)
	case cmd.Quantity <= 0:
		return domain.Reservation{}, fmt.Errorf("%w: quantity must be positive", domain.ErrInvalidInput)
	}
	return s.reservations.Reserve(ctx, domain.ReserveParams{
		ProductID: cmd.ProductID,
		CartID:    cmd.CartID,
		UserID:    cmd.UserID,
		Quantity:  cmd.Quantity,
		TTL:       s.ttl,
	})
}

type ReleaseReservationCommand struct {
	CartID    uuid.UUID
	ProductID uuid.UUID
}

func (s *ReservationService) Release(ctx context.Context, cmd ReleaseReservationCommand) error {
	return s.reservations.Release(ctx, cmd.CartID, cmd.ProductID)
}

type ReleaseCartCommand struct {
	CartID uuid.UUID
}

func (s *ReservationService) ReleaseCart(ctx context.Context, cmd ReleaseCartCommand) error {
	return s.reservations.ReleaseCart(ctx, cmd.CartID)
}

type ConfirmReservationsCommand struct {
	CartID uuid.UUID
	Items  []domain.ConfirmItem
}

func (s *ReservationService) Confirm(ctx context.Context, cmd ConfirmReservationsCommand) error {
	if cmd.CartID == uuid.Nil {
		return fmt.Errorf("%w: cart_id is required", domain.ErrInvalidInput)
	}
	if len(cmd.Items) == 0 {
		return fmt.Errorf("%w: items are required", domain.ErrInvalidInput)
	}
	seen := make(map[uuid.UUID]bool, len(cmd.Items))
	for _, it := range cmd.Items {
		switch {
		case it.ProductID == uuid.Nil:
			return fmt.Errorf("%w: product_id is required", domain.ErrInvalidInput)
		case it.Quantity <= 0:
			return fmt.Errorf("%w: quantity must be positive", domain.ErrInvalidInput)
		case seen[it.ProductID]:
			return fmt.Errorf("%w: duplicate product %s", domain.ErrInvalidInput, it.ProductID)
		}
		seen[it.ProductID] = true
	}
	return s.reservations.Confirm(ctx, cmd.CartID, cmd.Items)
}
