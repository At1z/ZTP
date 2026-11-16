package domain

import (
	"time"

	"github.com/google/uuid"
)

type Product struct {
	ID          uuid.UUID
	Name        string
	Description string
	PriceCents  int64
	Stock       int // physical inventory
	Available   int // Stock minus the active, non-expired reservations
}

// Reservation is a temporary lock on a quantity of one product for one cart.
type Reservation struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	CartID    uuid.UUID
	UserID    uuid.UUID
	Quantity  int
	ExpiresAt time.Time
}

type ReserveParams struct {
	ProductID uuid.UUID
	CartID    uuid.UUID
	UserID    uuid.UUID
	Quantity  int
	TTL       time.Duration
}

// ConfirmItem is one line of the purchase: the reservation of the cart for this
// product must exist, be active and have exactly this quantity.
type ConfirmItem struct {
	ProductID uuid.UUID
	Quantity  int
}
