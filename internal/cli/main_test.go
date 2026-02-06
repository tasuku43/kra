package cli

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("GIT_AUTHOR_NAME", "gionx-test")
	_ = os.Setenv("GIT_AUTHOR_EMAIL", "gionx-test@example.com")
	_ = os.Setenv("GIT_COMMITTER_NAME", "gionx-test")
	_ = os.Setenv("GIT_COMMITTER_EMAIL", "gionx-test@example.com")
	os.Exit(m.Run())
}

