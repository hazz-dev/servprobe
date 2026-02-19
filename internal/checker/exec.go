package checker

import (
	"context"
	"os/exec"
)

// osExecutor is the real CommandExecutor that uses os/exec.
type osExecutor struct{}

func (e *osExecutor) Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err = cmd.Output()
	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr = exitErr.Stderr
	}
	return stdout, stderr, err
}
