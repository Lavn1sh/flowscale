package worker

// NonRetryableError is a wrapper indicating that an activity failed
// permanently and should not be retried by the engine.
type NonRetryableError struct {
	Err error
}

func (e *NonRetryableError) Error() string {
	return e.Err.Error()
}

func (e *NonRetryableError) Unwrap() error {
	return e.Err
}

// NewNonRetryableError creates a new NonRetryableError.
func NewNonRetryableError(err error) error {
	if err == nil {
		return nil
	}
	return &NonRetryableError{Err: err}
}
