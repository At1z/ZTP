package domain

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusCancelled  Status = "cancelled"
)

// Order is created from a checked-out cart and never depends on the cart
// domain: it keeps its own copy of the purchased lines.
type Order struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	CartID     uuid.UUID
	TotalCents int64
	Status     Status
	CreatedAt  time.Time
	Items      []Item
}

type Item struct {
	ProductID      uuid.UUID
	ProductName    string
	UnitPriceCents int64
	Quantity       int
}

func (i Item) LineTotalCents() int64 {
	return i.UnitPriceCents * int64(i.Quantity)
}

func (o Order) ComputeTotalCents() int64 {
	var total int64
	for _, i := range o.Items {
		total += i.LineTotalCents()
	}
	return total
}

// Notification tells the user that something happened to their order.
type Notification struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	OrderID   uuid.UUID
	Message   string
	CreatedAt time.Time
}
