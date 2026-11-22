package domain

import (
	"context"

	"github.com/google/uuid"
)

// ProductRepository is the read side.
type ProductRepository interface {
	List(ctx context.Context) ([]Product, error)
	Get(ctx context.Context, id uuid.UUID) (Product, error)
}

// ReservationRepository is the write side. Every method is atomic.
type ReservationRepository interface {
	// Reserve locks the quantity for the cart or returns ErrInsufficientStock.
	Reserve(ctx context.Context, p ReserveParams) (Reservation, error)
	// Release drops the cart's reservation of one product. It is idempotent.
	Release(ctx context.Context, cartID, productID uuid.UUID) error
	// ReleaseCart drops all reservations of the cart. It is idempotent.
	ReleaseCart(ctx context.Context, cartID uuid.UUID) error
	// Confirm turns the cart's reservations of the given items into a purchase:
	// it decrements the stock and removes the reservations. It fails without
	// changing anything with ErrReservationNotFound, ErrReservationExpired or
	// ErrReservationMismatch. It is idempotent per cart: confirming an already
	// confirmed cart succeeds and does nothing.
	Confirm(ctx context.Context, cartID uuid.UUID, items []ConfirmItem) error
}
