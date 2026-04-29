package provider

import (
	"errors"
	"fmt"
)

var (
	ErrProviderNotFound          = errors.New("provider not found")
	ErrProviderAlreadyRegistered = errors.New("provider already registered")
	ErrInvalidInput              = errors.New("invalid input")
	ErrNotImplemented            = errors.New("feature not implemented")
)

// Error keeps provider and operation context for easier debugging.
type Error struct {
	Provider string
	Op       string
	Err      error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("provider(%s): %s: %v", e.Provider, e.Op, e.Err)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func WrapError(providerName, op string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Provider: providerName, Op: op, Err: err}
}
