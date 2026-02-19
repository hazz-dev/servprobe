package checker

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hazz-dev/servprobe/internal/config"
)

type httpChecker struct {
	svc    config.Service
	client *http.Client
}

func newHTTPChecker(svc config.Service) *httpChecker {
	return &httpChecker{
		svc:    svc,
		client: &http.Client{Timeout: svc.Timeout.Duration},
	}
}

func (c *httpChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		ServiceName: c.svc.Name,
		CheckedAt:   start,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.svc.Target, nil)
	if err != nil {
		result.Status = StatusDown
		result.Error = fmt.Sprintf("creating request: %v", err)
		result.ResponseTime = time.Since(start)
		return result
	}
	for k, v := range c.svc.Headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	result.ResponseTime = time.Since(start)
	if err != nil {
		result.Status = StatusDown
		result.Error = err.Error()
		return result
	}
	resp.Body.Close()

	expected := c.svc.ExpectedStatus
	if expected == 0 {
		expected = http.StatusOK
	}

	if resp.StatusCode != expected {
		result.Status = StatusDown
		result.Error = fmt.Sprintf("expected status %d, got %d", expected, resp.StatusCode)
		return result
	}

	result.Status = StatusUp
	return result
}
