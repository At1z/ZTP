package domain

import "errors"

var (
	ErrInvalidInput              = errors.New("invalid input")
	ErrCartNotFound              = errors.New("no open cart")
	ErrItemNotFound              = errors.New("item not in cart")
	ErrProductNotFound           = errors.New("product not found")
	ErrInsufficientStock         = errors.New("insufficient stock")
	ErrProductServiceUnavailable = errors.New("product service unavailable")
)
