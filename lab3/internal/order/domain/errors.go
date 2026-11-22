package domain

import "errors"

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrOrderNotFound = errors.New("order not found")
)
