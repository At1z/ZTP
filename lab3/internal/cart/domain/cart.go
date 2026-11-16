package domain

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusOpen       Status = "open"
	StatusCheckedOut Status = "checked_out"
	StatusExpired    Status = "expired"
)

// Cart belongs to exactly one user. A user has at most one open cart.
type Cart struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Status    Status
	CreatedAt time.Time
	ExpiresAt time.Time
	Items     []Item
}

// Item stores the product name and price as they were when it was first added.
type Item struct {
	ProductID      uuid.UUID
	ProductName    string
	UnitPriceCents int64
	Quantity       int
}

func (i Item) LineTotalCents() int64 {
	return i.UnitPriceCents * int64(i.Quantity)
}

func (c Cart) TotalCents() int64 {
	var total int64
	for _, i := range c.Items {
		total += i.LineTotalCents()
	}
	return total
}

// Product is what the cart needs to know about a product of the product service.
type Product struct {
	ID         uuid.UUID
	Name       string
	PriceCents int64
}

type ReserveRequest struct {
	CartID    uuid.UUID
	UserID    uuid.UUID
	ProductID uuid.UUID
	Quantity  int
}
