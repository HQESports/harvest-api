package processor

import (
	"fmt"
)

// Status types
const (
	StatusSuccess = "success"
	StatusFailure = "failure"
	StatusWarning = "warning"
	StatusSkipped = "skipped"
)

type StatusError interface {
	Error() string
	Status() string
	Message() string
}

type statusError struct {
	status  string
	message string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("%s: %s", e.status, e.message)
}

func (e *statusError) Status() string {
	return e.status
}

func (e *statusError) Message() string {
	return e.message
}

func NewSuccessError(message string) StatusError {
	return &statusError{status: StatusSuccess, message: message}
}

func NewFailureError(err error) StatusError {
	return &statusError{status: StatusFailure, message: err.Error()}
}

func NewWarningError(message string) StatusError {
	return &statusError{status: StatusWarning, message: message}
}

func NewSkippedError(message string) StatusError {
	return &statusError{status: StatusSkipped, message: message}
}
