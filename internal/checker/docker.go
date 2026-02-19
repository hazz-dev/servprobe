package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/hazz-dev/servprobe/internal/config"
)

const dockerSockPath = "/var/run/docker.sock"

// ContainerState holds the minimal Docker container state we care about.
type ContainerState struct {
	Running bool
}

// DockerClient abstracts Docker Engine API access for testability.
type DockerClient interface {
	InspectContainer(ctx context.Context, name string) (*ContainerState, error)
}

type dockerChecker struct {
	svc    config.Service
	client DockerClient
}

func newDockerChecker(svc config.Service) *dockerChecker {
	return &dockerChecker{
		svc:    svc,
		client: newUnixDockerClient(svc.Timeout.Duration),
	}
}

// NewDockerCheckerWithClient creates a docker checker with a custom client (for testing).
func NewDockerCheckerWithClient(svc config.Service, client DockerClient) Checker {
	return &dockerChecker{svc: svc, client: client}
}

func (c *dockerChecker) Check(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		ServiceName: c.svc.Name,
		CheckedAt:   start,
	}

	state, err := c.client.InspectContainer(ctx, c.svc.Target)
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Status = StatusDown
		result.Error = err.Error()
		return result
	}

	if !state.Running {
		result.Status = StatusDown
		result.Error = fmt.Sprintf("container %q is not running", c.svc.Target)
		return result
	}

	result.Status = StatusUp
	return result
}

// unixDockerClient queries the Docker Engine API over the Unix socket.
type unixDockerClient struct {
	client *http.Client
}

func newUnixDockerClient(timeout time.Duration) *unixDockerClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.DialTimeout("unix", dockerSockPath, timeout)
		},
	}
	return &unixDockerClient{
		client: &http.Client{Transport: transport, Timeout: timeout},
	}
}

func (d *unixDockerClient) InspectContainer(ctx context.Context, name string) (*ContainerState, error) {
	url := fmt.Sprintf("http://localhost/containers/%s/json", name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying docker socket: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("container %q not found", name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker API returned status %d", resp.StatusCode)
	}

	var body struct {
		State ContainerState `json:"State"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding docker response: %w", err)
	}
	return &body.State, nil
}
