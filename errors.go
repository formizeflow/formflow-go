package formflow

import "fmt"

// Error is any non-2xx answer. Code is the API error code (validation_failed, rate_limited, ...)
// or "network_error" when no response arrived.
type Error struct {
	Status     int
	Code       string
	Message    string
	Details    any
	RequestID  string
	RetryAfter float64 // seconds, 0 when absent
}

func (e *Error) Error() string { return fmt.Sprintf("%s (%d): %s", e.Code, e.Status, e.Message) }
