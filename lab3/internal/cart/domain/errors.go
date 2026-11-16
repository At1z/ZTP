package domain

import "errors"

var (
	ErrInvalidInput              = errors.New("invalid input")
	ErrCartNotFound              = errors.New("no open cart")
	ErrItemNotFound              = errors.New("item not in cart")
	ErrProductNotFound           = errors.New("product not found")
	ErrInsufficientStock         = errors.New("insufficient stock")
	ErrEmptyCart                 = errors.New("cart is empty")
	ErrReservationExpired        = errors.New("reservation expired, add the products again")
	ErrProductServiceUnavailable = errors.New("product service unavailable")
	ErrOrderServiceUnavailable   = errors.New("order service unavailable")
)
