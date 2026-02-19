package checker

import (
	"context"
	"fmt"

	"github.com/hazz-dev/svcmon/internal/config"
)

// Checker performs a single health check.
type Checker interface {
	Check(ctx context.Context) CheckResult
}

// New returns the appropriate Checker for the given service configuration.
func New(svc config.Service) (Checker, error) {
	switch svc.Type {
	case "http":
		return newHTTPChecker(svc), nil
	case "tcp":
		return newTCPChecker(svc), nil
	case "ping":
		return newPingChecker(svc), nil
	case "docker":
		return newDockerChecker(svc), nil
	default:
		return nil, fmt.Errorf("unknown checker type %q", svc.Type)
	}
}
