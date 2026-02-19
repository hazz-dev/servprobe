package checker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hazz-dev/svcmon/internal/checker"
	"github.com/hazz-dev/svcmon/internal/config"
)

// mockExecutor implements checker.CommandExecutor for testing.
type mockExecutor struct {
	stdout []byte
	stderr []byte
	err    error
}

func (m *mockExecutor) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	return m.stdout, m.stderr, m.err
}

func makePingService(t *testing.T, target string) config.Service {
	t.Helper()
	return config.Service{
		Name:    "test-ping",
		Type:    "ping",
		Target:  target,
		Timeout: config.Duration{Duration: 5 * time.Second},
	}
}

func TestPingChecker_Success(t *testing.T) {
	svc := makePingService(t, "127.0.0.1")
	c := checker.NewPingCheckerWithExecutor(svc, &mockExecutor{
		stdout: []byte("PING 127.0.0.1 (127.0.0.1) 56(84) bytes of data.\n64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time=0.123 ms\n\n--- 127.0.0.1 ping statistics ---\n1 packets transmitted, 1 received, 0% packet loss\nrtt min/avg/max/mdev = 0.123/0.123/0.123/0.000 ms\n"),
	})

	result := c.Check(context.Background())
	if result.Status != checker.StatusUp {
		t.Errorf("expected StatusUp, got %q: %s", result.Status, result.Error)
	}
	if result.ResponseTime <= 0 {
		t.Errorf("expected positive response time, got %v", result.ResponseTime)
	}
}

func TestPingChecker_Failed(t *testing.T) {
	svc := makePingService(t, "192.0.2.1")
	c := checker.NewPingCheckerWithExecutor(svc, &mockExecutor{
		stdout: []byte("PING 192.0.2.1 (192.0.2.1) 56(84) bytes of data.\n"),
		err:    errors.New("exit status 1"),
	})

	result := c.Check(context.Background())
	if result.Status != checker.StatusDown {
		t.Errorf("expected StatusDown, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message for failed ping")
	}
}

func TestPingChecker_Timeout(t *testing.T) {
	svc := makePingService(t, "192.0.2.1")
	c := checker.NewPingCheckerWithExecutor(svc, &mockExecutor{
		err: context.DeadlineExceeded,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := c.Check(ctx)
	if result.Status != checker.StatusDown {
		t.Errorf("expected StatusDown on timeout, got %q", result.Status)
	}
}

func TestPingChecker_MalformedOutput(t *testing.T) {
	svc := makePingService(t, "127.0.0.1")
	c := checker.NewPingCheckerWithExecutor(svc, &mockExecutor{
		stdout: []byte("some unexpected output without time field\n"),
	})

	result := c.Check(context.Background())
	if result.Status != checker.StatusDown {
		t.Errorf("expected StatusDown for malformed output, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message for malformed output")
	}
}

func TestPingChecker_RTTParsing(t *testing.T) {
	tests := []struct {
		name   string
		output string
		wantMs float64
	}{
		{
			name:   "linux format integer",
			output: "64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time=5 ms",
			wantMs: 5,
		},
		{
			name:   "linux format decimal",
			output: "64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time=12.345 ms",
			wantMs: 12.345,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := makePingService(t, "127.0.0.1")
			c := checker.NewPingCheckerWithExecutor(svc, &mockExecutor{
				stdout: []byte(tc.output),
			})

			result := c.Check(context.Background())
			if result.Status != checker.StatusUp {
				t.Errorf("expected StatusUp, got %q: %s", result.Status, result.Error)
			}
			gotMs := float64(result.ResponseTime) / float64(time.Millisecond)
			if abs(gotMs-tc.wantMs) > 0.001 {
				t.Errorf("expected RTT %.3fms, got %.3fms", tc.wantMs, gotMs)
			}
		})
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
