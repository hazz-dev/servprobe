package checker

import "time"

// Status represents the health state of a service.
type Status string

const (
	StatusUp   Status = "up"
	StatusDown Status = "down"
)

// CheckResult is the outcome of a single health check.
type CheckResult struct {
	ServiceName  string
	Status       Status
	ResponseTime time.Duration
	Error        string
	CheckedAt    time.Time
}
