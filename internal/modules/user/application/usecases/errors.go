package usecases

import "errors"

var (
	ErrEmailAlreadyRegistered = errors.New("email is already registered")
	ErrInvalidCredentials     = errors.New("credentials are invalid")
	ErrUnauthenticated        = errors.New("authentication is required")
	ErrAccountDisabled        = errors.New("account is disabled")
)

type FieldError struct {
	Field   string
	Code    string
	Message string
}

type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return "request validation failed" }
