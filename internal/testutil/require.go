package testutil

import (
	"os/exec"
	"testing"
)

func RequireCommand(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not found in PATH", name)
	}
}
