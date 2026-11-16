package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type CartRepository interface {
	// OpenCart returns the user's open cart, creating it when there is none.
	// created tells which of the two happened. It returns ErrCartNotFound when
	// the user's existing cart has just expired; the caller should sweep and retry.
	OpenCart(ctx context.Context, userID uuid.UUID, ttl time.Duration) (cart Cart, created bool, err error)

	// GetOpenCart returns the user's open, non-expired cart with its items.
	GetOpenCart(ctx context.Context, userID uuid.UUID) (Cart, error)

	// Mutate runs fn in one transaction with the user's open cart locked
	// exclusively. Operations on the same cart (from any device or instance) are
	// therefore executed one after another, while carts of different users
	// never wait for each other. If fn fails, nothing is changed.
	// It returns ErrCartNotFound when the user has no open cart.
	Mutate(ctx context.Context, userID uuid.UUID, fn func(ctx context.Context, tx CartTx) error) error

	// DeleteExpired removes every open cart past its expires_at and returns
	// their ids. Each cart is returned to exactly one caller.
	DeleteExpired(ctx context.Context) ([]uuid.UUID, error)
}

// CartTx is the locked cart handed to Mutate.
type CartTx interface {
	Cart() Cart // header only, without items
	Items(ctx context.Context) ([]Item, error)
	// UpsertItem adds the quantity to an existing line or creates the line.
	UpsertItem(ctx context.Context, item Item) error
	// DeleteItem removes the line and reports whether it existed.
	DeleteItem(ctx context.Context, productID uuid.UUID) (bool, error)
	// MarkCheckedOut closes the cart; the user can then open a new one.
	MarkCheckedOut(ctx context.Context) error
}

// ProductClient talks to the product service over HTTP.
type ProductClient interface {
	GetProduct(ctx context.Context, id uuid.UUID) (Product, error)
	// Reserve locks stock for the cart; ErrInsufficientStock when none is left.
	Reserve(ctx context.Context, req ReserveRequest) error
	Release(ctx context.Context, cartID, productID uuid.UUID) error
	ReleaseCart(ctx context.Context, cartID uuid.UUID) error
	// Confirm buys the reserved items: the stock is decremented for good. It
	// returns ErrReservationExpired when a reservation is gone or expired and
	// is idempotent per cart, so a checkout can be retried.
	Confirm(ctx context.Context, cartID uuid.UUID, items []ConfirmItem) error
}

// OrderClient talks to the order service over HTTP.
type OrderClient interface {
	// CreateOrder is idempotent per cart: it returns the existing order when
	// the cart already has one.
	CreateOrder(ctx context.Context, req CreateOrderRequest) (Order, error)
}
