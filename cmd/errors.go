package cmd

// UsageError represents an invalid usage error that should exit with code 2.
type UsageError struct {
	msg string
}

// Error returns the error message.
func (e *UsageError) Error() string {
	return e.msg
}

// NewUsageError creates a new UsageError with the given message.
func NewUsageError(msg string) *UsageError {
	return &UsageError{msg: msg}
}
