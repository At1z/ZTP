package domain

import (
	"errors"
	"strings"
)

var (
	ErrNotFound = errors.New("not found")
	// ErrInvalidState is returned when the notification's status does not allow
	// the requested operation (e.g. cancelling an already sent notification).
	ErrInvalidState = errors.New("invalid state")
)

// FieldError points at the input field that needs to be corrected.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct {
	Errors []FieldError
}

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Errors))
	for i, fe := range e.Errors {
		msgs[i] = fe.Field + ": " + fe.Message
	}
	return "validation failed: " + strings.Join(msgs, "; ")
}

// ConflictError is returned when the request conflicts with the current state.
type ConflictError struct {
	FieldError
}

func (e *ConflictError) Error() string {
	return e.Field + ": " + e.Message
}
