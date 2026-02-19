package checker

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/hazz-dev/svcmon/internal/config"
)

// CommandExecutor abstracts os/exec for testability.
type CommandExecutor interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}

type pingChecker struct {
	svc      config.Service
	executor CommandExecutor
}

func newPingChecker(svc config.Service) *pingChecker {
	return &pingChecker{svc: svc, executor: &osExecutor{}}
}

// NewPingCheckerWithExecutor creates a ping checker with a custom executor (for testing).
func NewPingCheckerWithExecutor(svc config.Service, exec CommandExecutor) Checker {
	return &pingChecker{svc: svc, executor: exec}
}

var rttRegex = regexp.MustCompile(`time=(\d+\.?\d*)\s*ms`)

func (c *pingChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		ServiceName: c.svc.Name,
		CheckedAt:   start,
	}

	timeoutSec := int(math.Ceil(c.svc.Timeout.Duration.Seconds()))
	if timeoutSec < 1 {
		timeoutSec = 1
	}

	var args []string
	if runtime.GOOS == "darwin" {
		args = []string{"-c", "1", "-t", strconv.Itoa(timeoutSec), c.svc.Target}
	} else {
		args = []string{"-c", "1", "-W", strconv.Itoa(timeoutSec), c.svc.Target}
	}

	stdout, _, err := c.executor.Run(ctx, "ping", args...)
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Status = StatusDown
		result.Error = fmt.Sprintf("ping %s: %v", c.svc.Target, err)
		return result
	}

	matches := rttRegex.FindSubmatch(stdout)
	if matches == nil {
		result.Status = StatusDown
		result.Error = "could not parse RTT from ping output"
		return result
	}

	ms, _ := strconv.ParseFloat(string(matches[1]), 64)
	result.ResponseTime = time.Duration(ms * float64(time.Millisecond))
	result.Status = StatusUp
	return result
}
