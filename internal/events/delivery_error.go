package events

import "time"

// TerminalError marks a publish that must not be retried.
// The dispatcher sets that sink to dead on the row immediately.
// LastError is Error(), so the caller should include the HTTP status.
type TerminalError struct {
	Err error
}

func (e *TerminalError) Error() string {
	if e == nil || e.Err == nil {
		return "terminal"
	}
	return e.Err.Error()
}

func (e *TerminalError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// RetryAfterError is a failed publish that should be retried, but not before After.
// The dispatcher waits at least the normal backoff and at least After, then caps
// the delay at the backoff maximum (5 minutes).
type RetryAfterError struct {
	Err   error
	After time.Duration
}

func (e *RetryAfterError) Error() string {
	if e == nil || e.Err == nil {
		return "retry"
	}
	return e.Err.Error()
}

func (e *RetryAfterError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
