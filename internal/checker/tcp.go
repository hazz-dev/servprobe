package checker

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/hazz-dev/svcmon/internal/config"
)

type tcpChecker struct {
	svc config.Service
}

func newTCPChecker(svc config.Service) *tcpChecker {
	return &tcpChecker{svc: svc}
}

func (c *tcpChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		ServiceName: c.svc.Name,
		CheckedAt:   start,
	}

	dialer := &net.Dialer{Timeout: c.svc.Timeout.Duration}
	conn, err := dialer.DialContext(ctx, "tcp", c.svc.Target)
	result.ResponseTime = time.Since(start)
	if err != nil {
		result.Status = StatusDown
		result.Error = fmt.Sprintf("dial tcp %s: %v", c.svc.Target, err)
		return result
	}
	conn.Close()
	result.Status = StatusUp
	return result
}
