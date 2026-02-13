package workspace

import (
	"errors"
	"fmt"
)

// ErrorKind classifies domain/application errors for CLI exit mapping.
type ErrorKind string

const (
	ErrorKindUsage    ErrorKind = "usage"
	ErrorKindNotFound ErrorKind = "not_found"
	ErrorKindConflict ErrorKind = "conflict"
	ErrorKindBackend  ErrorKind = "backend"
)

// Error is a typed domain error.
type Error struct {
	Kind    ErrorKind
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *Error) Unwrap() error { return e.Cause }

// IsKind checks if err (or wrapped err) matches kind.
func IsKind(err error, kind ErrorKind) bool {
	var target *Error
	if errors.As(err, &target) {
		return target.Kind == kind
	}
	return false
}

func usageError(msg string) error    { return &Error{Kind: ErrorKindUsage, Message: msg} }
func notFoundError(msg string) error { return &Error{Kind: ErrorKindNotFound, Message: msg} }
func conflictError(msg string) error { return &Error{Kind: ErrorKindConflict, Message: msg} }
func backendError(msg string, cause error) error {
	return &Error{Kind: ErrorKindBackend, Message: msg, Cause: cause}
}

// NewErrorUsage creates a usage error.
func NewErrorUsage(msg string) error { return usageError(msg) }

// NewErrorNotFound creates a not-found error.
func NewErrorNotFound(msg string) error { return notFoundError(msg) }

// NewErrorConflict creates a conflict error.
func NewErrorConflict(msg string) error { return conflictError(msg) }

// NewErrorBackend creates a backend error.
func NewErrorBackend(msg string, cause error) error { return backendError(msg, cause) }
