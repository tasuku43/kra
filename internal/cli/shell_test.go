package cli

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestCLI_Shell_Help(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"shell", "--help"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra shell") {
		t.Fatalf("stdout missing usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_Shell_Init_Zsh_PrintsEvalReadyFunction(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"shell", "init", "zsh"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	text := out.String()
	if !strings.Contains(text, `eval "$(kra shell init zsh)"`) {
		t.Fatalf("missing bootstrap hint: %q", text)
	}
	if !strings.Contains(text, "kra() {") {
		t.Fatalf("missing function definition: %q", text)
	}
	if !strings.Contains(text, `KRA_SHELL_ACTION_FILE="$__kra_action_file"`) {
		t.Fatalf("missing action file env wiring: %q", text)
	}
	if !strings.Contains(text, `command kra "$@"`) {
		t.Fatalf("missing unified command passthrough: %q", text)
	}
	if !strings.Contains(text, `if [ -s "$__kra_action_file" ]; then`) {
		t.Fatalf("missing action file apply block: %q", text)
	}
}

func TestRenderShellInitScript_UnsupportedShell(t *testing.T) {
	_, err := renderShellInitScript("nushell", false)
	if err == nil {
		t.Fatal("expected unsupported shell error")
	}
}

func TestCLI_Shell_Init_WithCompletion_Zsh_IncludesCompletionScript(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"shell", "init", "zsh", "--with-completion"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	text := out.String()
	if !strings.Contains(text, "kra() {") {
		t.Fatalf("missing shell init function: %q", text)
	}
	if !strings.Contains(text, "compdef _kra_completion kra") {
		t.Fatalf("missing zsh completion section: %q", text)
	}
}

func TestCLI_Shell_Init_WithCompletion_Bash_IncludesCompletionScript(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"shell", "init", "bash", "--with-completion"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	text := out.String()
	if !strings.Contains(text, "kra() {") {
		t.Fatalf("missing shell init function: %q", text)
	}
	if !strings.Contains(text, "complete -o default -F _kra_completion kra") {
		t.Fatalf("missing bash completion section: %q", text)
	}
}

func TestCLI_Shell_Init_WithCompletion_Fish_IncludesCompletionScript(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"shell", "init", "fish", "--with-completion"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	text := out.String()
	if !strings.Contains(text, "function kra") {
		t.Fatalf("missing fish init function: %q", text)
	}
	if !strings.Contains(text, "complete -c kra -f") {
		t.Fatalf("missing fish completion section: %q", text)
	}
}

func TestCLI_Shell_Completion_Zsh_PrintsCompdef(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"shell", "completion", "zsh"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	text := out.String()
	if !strings.Contains(text, "compdef _kra_completion kra") {
		t.Fatalf("missing zsh compdef: %q", text)
	}
	if !strings.Contains(text, "context") || !strings.Contains(text, "ws") {
		t.Fatalf("missing command candidates: %q", text)
	}
	deprecatedSkill := "ag" + "ent-skills"
	deprecatedCommand := `"` + "ag" + `ent")`
	if strings.Contains(text, deprecatedSkill) || strings.Contains(text, deprecatedCommand) {
		t.Fatalf("deprecated completion tokens should be removed: %q", text)
	}
	if !strings.Contains(text, "\"ws add-repo\") flags=(") || !strings.Contains(text, "--base-ref") {
		t.Fatalf("missing ws add-repo flag candidates: %q", text)
	}
	if !strings.Contains(text, "\"context create\") flags=(") || !strings.Contains(text, "--path") {
		t.Fatalf("missing context create flag candidates: %q", text)
	}
	if !strings.Contains(text, "\"ws add-repo\")\n      has_target=0") {
		t.Fatalf("missing target-selector gate for ws add-repo: %q", text)
	}
	if !strings.Contains(text, `flags=("--id" "--current" "--select" "--help")`) {
		t.Fatalf("missing selector-first candidates: %q", text)
	}
	if !strings.Contains(text, `flags=("--format" "--repo" "--branch" "--base-ref" "--yes" "--refresh" "--no-fetch" "--help")`) {
		t.Fatalf("missing post-selector candidates: %q", text)
	}
	if strings.Contains(text, "\"-h\"") {
		t.Fatalf("short help alias should not be suggested in completion: %q", text)
	}
}

func TestCLI_Shell_Completion_Bash_PrintsCompleteHook(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"shell", "completion", "bash"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	text := out.String()
	if !strings.Contains(text, "complete -o default -F _kra_completion kra") {
		t.Fatalf("missing bash complete hook: %q", text)
	}
	deprecatedSkill := "ag" + "ent-skills"
	deprecatedCommand := `"` + "ag" + `ent"`
	if strings.Contains(text, deprecatedSkill) || strings.Contains(text, deprecatedCommand) {
		t.Fatalf("deprecated completion tokens should be removed: %q", text)
	}
	if !strings.Contains(text, "context") || !strings.Contains(text, "add-repo") {
		t.Fatalf("missing subcommand candidates: %q", text)
	}
	if !strings.Contains(text, "\"ws add-repo\")") || !strings.Contains(text, "--base-ref") {
		t.Fatalf("missing ws add-repo flag candidates: %q", text)
	}
	if !strings.Contains(text, "\"ws import jira\")") || !strings.Contains(text, "--sprint") {
		t.Fatalf("missing ws import jira flag candidates: %q", text)
	}
	if !strings.Contains(text, "\"ws add-repo\")\n        has_target=0") {
		t.Fatalf("missing target-selector gate for ws add-repo: %q", text)
	}
	if !strings.Contains(text, `"--id --current --select --help"`) {
		t.Fatalf("missing selector-first candidates: %q", text)
	}
	if !strings.Contains(text, `"--format --repo --branch --base-ref --yes --refresh --no-fetch --help"`) {
		t.Fatalf("missing post-selector candidates: %q", text)
	}
	if strings.Contains(text, " -h") {
		t.Fatalf("short help alias should not be suggested in completion: %q", text)
	}
}

func TestCLI_Shell_Completion_Fish_PrintsCompleteRules(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"shell", "completion", "fish"})
	if code != exitOK {
		t.Fatalf("exit code=%d, want=%d (stderr=%q)", code, exitOK, err.String())
	}
	text := out.String()
	if !strings.Contains(text, "complete -c kra -f") {
		t.Fatalf("missing fish base completion: %q", text)
	}
	if !strings.Contains(text, "__fish_seen_subcommand_from shell") {
		t.Fatalf("missing fish shell subcommand rule: %q", text)
	}
	if !strings.Contains(text, "__fish_seen_subcommand_from ws; and __fish_seen_subcommand_from add-repo") {
		t.Fatalf("missing fish ws add-repo rule: %q", text)
	}
	if !strings.Contains(text, " -l base-ref") || !strings.Contains(text, " -l sprint") {
		t.Fatalf("missing fish flag candidates: %q", text)
	}
}

func TestRenderShellCompletionScript_UnsupportedShell(t *testing.T) {
	_, err := renderShellCompletionScript("nushell")
	if err == nil {
		t.Fatal("expected unsupported shell error")
	}
}

func TestKraCompletionCommandFlags_WS_FocusTargetSelectors(t *testing.T) {
	want := []string{"--id", "--current", "--select", "--help"}
	got := completionRenderableFlags(kraCompletionCommandFlags["ws"])
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ws command flags = %v, want %v", got, want)
	}
}
