package procexec

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func RunCombined(ctx context.Context, dir string, name string, defaultTimeout time.Duration, args ...string) ([]byte, error) {
	runCtx, cancel, appliedTimeout := withDefaultTimeout(ctx, defaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil && runCtx.Err() == context.DeadlineExceeded {
		return out, wrapDeadlineError(name, args, defaultTimeout, appliedTimeout)
	}
	return out, err
}

func RunOutput(ctx context.Context, dir string, name string, defaultTimeout time.Duration, args ...string) ([]byte, []byte, error) {
	runCtx, cancel, appliedTimeout := withDefaultTimeout(ctx, defaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	stdout, err := cmd.Output()
	if err == nil {
		return stdout, nil, nil
	}
	stderr := []byte(nil)
	if ee, ok := err.(*exec.ExitError); ok {
		stderr = ee.Stderr
	}
	if runCtx.Err() == context.DeadlineExceeded {
		return stdout, stderr, wrapDeadlineError(name, args, defaultTimeout, appliedTimeout)
	}
	return stdout, stderr, err
}

func withDefaultTimeout(ctx context.Context, defaultTimeout time.Duration) (context.Context, context.CancelFunc, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if defaultTimeout <= 0 {
		return ctx, func() {}, false
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}, false
	}
	derived, cancel := context.WithTimeout(ctx, defaultTimeout)
	return derived, cancel, true
}

func formatCommand(name string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, strings.TrimSpace(name))
	for _, arg := range args {
		parts = append(parts, strings.TrimSpace(arg))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func wrapDeadlineError(name string, args []string, defaultTimeout time.Duration, appliedTimeout bool) error {
	command := formatCommand(name, args...)
	if appliedTimeout {
		return fmt.Errorf("%s timed out after %s: %w", command, defaultTimeout, context.DeadlineExceeded)
	}
	return fmt.Errorf("%s exceeded context deadline: %w", command, context.DeadlineExceeded)
}
