package checker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hazz-dev/svcmon/internal/checker"
	"github.com/hazz-dev/svcmon/internal/config"
)

// mockDockerClient implements checker.DockerClient for testing.
type mockDockerClient struct {
	state *checker.ContainerState
	err   error
}

func (m *mockDockerClient) InspectContainer(ctx context.Context, name string) (*checker.ContainerState, error) {
	return m.state, m.err
}

func makeDockerService(t *testing.T, target string) config.Service {
	t.Helper()
	return config.Service{
		Name:    "test-docker",
		Type:    "docker",
		Target:  target,
		Timeout: config.Duration{Duration: 5 * time.Second},
	}
}

func TestDockerChecker_Running(t *testing.T) {
	svc := makeDockerService(t, "my-container")
	c := checker.NewDockerCheckerWithClient(svc, &mockDockerClient{
		state: &checker.ContainerState{Running: true},
	})

	result := c.Check(context.Background())
	if result.Status != checker.StatusUp {
		t.Errorf("expected StatusUp for running container, got %q: %s", result.Status, result.Error)
	}
	if result.ResponseTime <= 0 {
		t.Errorf("expected positive response time, got %v", result.ResponseTime)
	}
}

func TestDockerChecker_Stopped(t *testing.T) {
	svc := makeDockerService(t, "stopped-container")
	c := checker.NewDockerCheckerWithClient(svc, &mockDockerClient{
		state: &checker.ContainerState{Running: false},
	})

	result := c.Check(context.Background())
	if result.Status != checker.StatusDown {
		t.Errorf("expected StatusDown for stopped container, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message for stopped container")
	}
}

func TestDockerChecker_NotFound(t *testing.T) {
	svc := makeDockerService(t, "nonexistent")
	c := checker.NewDockerCheckerWithClient(svc, &mockDockerClient{
		err: errors.New(`container "nonexistent" not found`),
	})

	result := c.Check(context.Background())
	if result.Status != checker.StatusDown {
		t.Errorf("expected StatusDown for not-found container, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message for not-found container")
	}
}

func TestDockerChecker_SocketUnavailable(t *testing.T) {
	svc := makeDockerService(t, "my-container")
	c := checker.NewDockerCheckerWithClient(svc, &mockDockerClient{
		err: errors.New("dial unix /var/run/docker.sock: connect: no such file or directory"),
	})

	result := c.Check(context.Background())
	if result.Status != checker.StatusDown {
		t.Errorf("expected StatusDown when socket unavailable, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message when socket unavailable")
	}
}
