package cli

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

type uiE2EStep struct {
	Args  []string
	Stdin string
}

func TestGolden_UIE2E_CoreWorkspaceWorkflow(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env := testutil.NewEnv(t)

	steps := []uiE2EStep{
		{Args: []string{"init", "--root", env.Root, "--context", "ui-e2e"}},
		{Args: []string{"ws", "list"}},
		{Args: []string{"ws", "create", "--no-prompt", "UI-100"}},
		{Args: []string{"ws", "list"}},
		{Args: []string{"ws", "--act", "go", "UI-100"}},
		{Args: []string{"ws", "--act", "close", "UI-100"}},
		{Args: []string{"ws", "list", "--archived"}},
	}

	var transcript strings.Builder
	for i, step := range steps {
		stdout, stderr, code := runUIE2EStep(t, step)
		if i == 0 {
			configureRootGitUserForUIE2E(t, env.Root)
		}

		transcript.WriteString(fmt.Sprintf("$ kra %s\n", strings.Join(step.Args, " ")))
		transcript.WriteString(fmt.Sprintf("exit: %d\n", code))
		if strings.TrimSpace(stdout) != "" {
			transcript.WriteString("stdout:\n")
			transcript.WriteString(stdout)
			if !strings.HasSuffix(stdout, "\n") {
				transcript.WriteString("\n")
			}
		}
		if strings.TrimSpace(stderr) != "" {
			transcript.WriteString("stderr:\n")
			transcript.WriteString(stderr)
			if !strings.HasSuffix(stderr, "\n") {
				transcript.WriteString("\n")
			}
		}
		transcript.WriteString("\n")
	}

	got := normalizeUIE2ETranscript(transcript.String(), env.Root)
	assertGolden(t, "ui_e2e_core_workspace_workflow.golden", got)
}

func TestGolden_UIE2E_ArchivePurgeFlow(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env := testutil.NewEnv(t)

	steps := []uiE2EStep{
		{Args: []string{"init", "--root", env.Root, "--context", "ui-e2e"}},
		{Args: []string{"ws", "create", "--no-prompt", "UI-200"}},
		{Args: []string{"ws", "--act", "close", "UI-200"}},
		{Args: []string{"ws", "--act", "reopen", "UI-200"}},
		{Args: []string{"ws", "--act", "close", "UI-200"}},
		{Args: []string{"ws", "unlock", "UI-200"}},
		{Args: []string{"ws", "--act", "purge", "--no-prompt", "--force", "UI-200"}},
		{Args: []string{"ws", "list", "--archived"}},
		{Args: []string{"ws", "list"}},
	}

	var transcript strings.Builder
	for i, step := range steps {
		stdout, stderr, code := runUIE2EStep(t, step)
		if i == 0 {
			configureRootGitUserForUIE2E(t, env.Root)
		}

		transcript.WriteString(fmt.Sprintf("$ kra %s\n", strings.Join(step.Args, " ")))
		transcript.WriteString(fmt.Sprintf("exit: %d\n", code))
		if strings.TrimSpace(stdout) != "" {
			transcript.WriteString("stdout:\n")
			transcript.WriteString(stdout)
			if !strings.HasSuffix(stdout, "\n") {
				transcript.WriteString("\n")
			}
		}
		if strings.TrimSpace(stderr) != "" {
			transcript.WriteString("stderr:\n")
			transcript.WriteString(stderr)
			if !strings.HasSuffix(stderr, "\n") {
				transcript.WriteString("\n")
			}
		}
		transcript.WriteString("\n")
	}

	got := normalizeUIE2ETranscript(transcript.String(), env.Root)
	assertGolden(t, "ui_e2e_archive_purge_flow.golden", got)
}

func runUIE2EStep(t *testing.T, step uiE2EStep) (stdout string, stderr string, code int) {
	t.Helper()

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	if strings.TrimSpace(step.Stdin) != "" {
		var in bytes.Buffer
		in.WriteString(step.Stdin)
		c.In = &in
	}

	code = c.Run(step.Args)
	return out.String(), err.String(), code
}

func configureRootGitUserForUIE2E(t *testing.T, root string) {
	t.Helper()

	cmd := exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.email failed: %v (output=%s)", err, strings.TrimSpace(string(out)))
	}

	cmd = exec.Command("git", "config", "user.name", "test")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config user.name failed: %v (output=%s)", err, strings.TrimSpace(string(out)))
	}
}

func normalizeUIE2ETranscript(s string, root string) string {
	normalized := strings.ReplaceAll(s, root, "<ROOT>")
	reShortSHA := regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)
	normalized = reShortSHA.ReplaceAllString(normalized, "<SHA>")
	return normalized
}
