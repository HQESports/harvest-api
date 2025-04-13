package processor

// Status represents the outcome of a string operation
type Status int

const (
	Success Status = iota
	Warning
	Failure
)

// StatusError represents an error with an associated status
type StatusError struct {
	Message string
	Status  Status
}

func (e StatusError) Error() string {
	return e.Message
}

func NewSuccessError(msg string) StatusError {
	return StatusError{
		Message: msg,
		Status:  Success,
	}
}

// NewWarningError creates a new warning error
func NewWarningError(msg string) StatusError {
	return StatusError{
		Message: msg,
		Status:  Warning,
	}
}

// NewFailureError creates a new failure error
func NewFailureError(err error) StatusError {
	return StatusError{
		Message: err.Error(),
		Status:  Failure,
	}
}

// Helper functions to check error status
func GetErrorStatus(err error) Status {
	if err == nil {
		return Success
	}

	if statusErr, ok := err.(*StatusError); ok {
		return statusErr.Status
	}

	// Default to treating unknown errors as failures
	return Failure
}
