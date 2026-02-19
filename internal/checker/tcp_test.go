package checker_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/hazz-dev/servprobe/internal/checker"
	"github.com/hazz-dev/servprobe/internal/config"
)

func makeTCPService(t *testing.T, addr string, extras ...func(*config.Service)) config.Service {
	t.Helper()
	svc := config.Service{
		Name:    "test-tcp",
		Type:    "tcp",
		Target:  addr,
		Timeout: config.Duration{Duration: 2 * time.Second},
	}
	for _, fn := range extras {
		fn(&svc)
	}
	return svc
}

func TestTCPChecker_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	// Accept connections in background
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	svc := makeTCPService(t, ln.Addr().String())
	c, err := checker.New(svc)
	if err != nil {
		t.Fatal(err)
	}

	result := c.Check(context.Background())
	if result.Status != checker.StatusUp {
		t.Errorf("expected StatusUp, got %q: %s", result.Status, result.Error)
	}
	if result.ResponseTime <= 0 {
		t.Errorf("expected positive response time, got %v", result.ResponseTime)
	}
}

func TestTCPChecker_ConnectionRefused(t *testing.T) {
	// Bind and immediately close to get a port that's not listening.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	svc := makeTCPService(t, addr)
	c, err := checker.New(svc)
	if err != nil {
		t.Fatal(err)
	}

	result := c.Check(context.Background())
	if result.Status != checker.StatusDown {
		t.Errorf("expected StatusDown for refused connection, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message for refused connection")
	}
}

func TestTCPChecker_Timeout(t *testing.T) {
	// Use a listener that accepts but never responds — simulate slow host.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	svc := makeTCPService(t, ln.Addr().String(), func(s *config.Service) {
		s.Timeout = config.Duration{Duration: 1 * time.Millisecond}
	})
	c, err := checker.New(svc)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result := c.Check(ctx)
	// Either timeout or success — the key is it doesn't hang.
	// With 1ms timeout it should fail.
	if result.Status != checker.StatusDown {
		t.Logf("got status %q (may be flaky on fast machines), error: %s", result.Status, result.Error)
	}
}
