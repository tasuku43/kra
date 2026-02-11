package cli

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	gionxHome, err := os.MkdirTemp("", "gionx-test-home-*")
	if err == nil {
		_ = os.Setenv("GIONX_HOME", gionxHome)
		defer os.RemoveAll(gionxHome)
	}
	_ = os.Setenv("GIT_AUTHOR_NAME", "gionx-test")
	_ = os.Setenv("GIT_AUTHOR_EMAIL", "gionx-test@example.com")
	_ = os.Setenv("GIT_COMMITTER_NAME", "gionx-test")
	_ = os.Setenv("GIT_COMMITTER_EMAIL", "gionx-test@example.com")
	os.Exit(m.Run())
}
