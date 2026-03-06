package procexec

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestRunCombined_DefaultTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell timing test is unix-specific")
	}

	_, err := RunCombined(context.Background(), "", "sh", 50*time.Millisecond, "-c", "sleep 0.2")
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestRunCombined_RespectsExistingDeadline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell timing test is unix-specific")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := RunCombined(ctx, "", "sh", time.Second, "-c", "sleep 0.2")
	if err == nil {
		t.Fatalf("expected deadline exceeded")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestRunOutput_Succeeds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell output test is unix-specific")
	}

	stdout, stderr, err := RunOutput(context.Background(), "", "sh", time.Second, "-c", "printf ok")
	if err != nil {
		t.Fatalf("RunOutput() error = %v", err)
	}
	if string(stdout) != "ok" {
		t.Fatalf("stdout = %q, want %q", string(stdout), "ok")
	}
	if len(stderr) != 0 {
		t.Fatalf("stderr = %q, want empty", string(stderr))
	}
}
