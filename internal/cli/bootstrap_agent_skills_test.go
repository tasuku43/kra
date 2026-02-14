package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/paths"
)

func TestCLI_BootstrapAgentSkills_Help(t *testing.T) {
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"bootstrap", "agent-skills", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra bootstrap agent-skills") {
		t.Fatalf("stdout missing bootstrap usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_BootstrapAgentSkills_RequiresCurrentContext(t *testing.T) {
	setKraHomeForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"bootstrap", "agent-skills"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "current context is not set") {
		t.Fatalf("stderr missing current context guidance: %q", err.String())
	}
}

func TestCLI_BootstrapAgentSkills_CreatesDirectoryAndSymlinks(t *testing.T) {
	root := prepareCurrentRootForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"bootstrap", "agent-skills"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "Result:") {
		t.Fatalf("stdout missing result section: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}

	skillsRoot := filepath.Join(root, ".agent", "skills")
	if info, statErr := os.Stat(skillsRoot); statErr != nil {
		t.Fatalf("stat %s: %v", skillsRoot, statErr)
	} else if !info.IsDir() {
		t.Fatalf("%s is not dir", skillsRoot)
	}
	assertSkillpackSeeded(t, skillsRoot)
	assertSymlinkTo(t, filepath.Join(root, ".codex", "skills"), skillsRoot)
	assertSymlinkTo(t, filepath.Join(root, ".claude", "skills"), skillsRoot)
}

func TestCLI_BootstrapAgentSkills_Idempotent(t *testing.T) {
	root := prepareCurrentRootForTest(t)

	var out1 bytes.Buffer
	var err1 bytes.Buffer
	c1 := New(&out1, &err1)
	code := c1.Run([]string{"bootstrap", "agent-skills", "--format", "json"})
	if code != exitOK {
		t.Fatalf("first run exit code = %d, want %d (stderr=%q)", code, exitOK, err1.String())
	}
	first := decodeJSONResponse(t, out1.String())
	if !first.OK || first.Action != "bootstrap.agent-skills" {
		t.Fatalf("unexpected first json response: %+v", first)
	}

	var out2 bytes.Buffer
	var err2 bytes.Buffer
	c2 := New(&out2, &err2)
	code = c2.Run([]string{"bootstrap", "agent-skills", "--format", "json"})
	if code != exitOK {
		t.Fatalf("second run exit code = %d, want %d (stderr=%q)", code, exitOK, err2.String())
	}
	second := decodeJSONResponse(t, out2.String())
	if !second.OK || second.Action != "bootstrap.agent-skills" {
		t.Fatalf("unexpected second json response: %+v", second)
	}

	if got := len(jsonAnyArray(second.Result["linked"])); got != 0 {
		t.Fatalf("second result.linked length = %d, want 0", got)
	}
	if got := len(jsonAnyArray(second.Result["conflicts"])); got != 0 {
		t.Fatalf("second result.conflicts length = %d, want 0", got)
	}
	if got := len(jsonAnyArray(second.Result["skipped"])); got < 3 {
		t.Fatalf("second result.skipped length = %d, want >= 3", got)
	}

	skillsRoot := filepath.Join(root, ".agent", "skills")
	assertSkillpackSeeded(t, skillsRoot)
	assertSymlinkTo(t, filepath.Join(root, ".codex", "skills"), skillsRoot)
	assertSymlinkTo(t, filepath.Join(root, ".claude", "skills"), skillsRoot)
}

func TestCLI_BootstrapAgentSkills_Conflict_NoOverwrite(t *testing.T) {
	root := prepareCurrentRootForTest(t)
	conflictPath := filepath.Join(root, ".codex", "skills")
	if err := os.MkdirAll(filepath.Dir(conflictPath), 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	if err := os.WriteFile(conflictPath, []byte("conflict\n"), 0o644); err != nil {
		t.Fatalf("write conflict file: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"bootstrap", "agent-skills", "--format", "json"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "bootstrap.agent-skills" || resp.Error.Code != "conflict" {
		t.Fatalf("unexpected json response: %+v", resp)
	}

	conflicts := jsonAnyArray(resp.Result["conflicts"])
	if len(conflicts) == 0 {
		t.Fatalf("conflicts should not be empty: %+v", resp.Result)
	}
	if !strings.Contains(asJSONString(conflicts[0]), ".codex/skills") {
		t.Fatalf("first conflict should mention .codex/skills: %s", asJSONString(conflicts[0]))
	}

	info, statErr := os.Lstat(conflictPath)
	if statErr != nil {
		t.Fatalf("lstat conflict path: %v", statErr)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("conflict path should remain non-symlink")
	}
	if _, err := os.Lstat(filepath.Join(root, ".claude", "skills")); !os.IsNotExist(err) {
		t.Fatalf(".claude/skills should not be created on conflict, err=%v", err)
	}
}

func TestCLI_BootstrapAgentSkills_RejectsRootContextFlags(t *testing.T) {
	setKraHomeForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"bootstrap", "agent-skills", "--root", "/tmp/x"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "--root is not supported") {
		t.Fatalf("stderr missing unsupported flag message: %q", err.String())
	}
}

func TestCLI_Init_BootstrapAgentSkills_Integration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	setGitIdentity(t)

	root := t.TempDir()
	setKraHomeForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"init", "--root", root, "--context", "ctx", "--bootstrap", "agent-skills"})
	if code != exitOK {
		t.Fatalf("init exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "Bootstrap agent-skills") {
		t.Fatalf("stdout missing bootstrap summary: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}

	skillsRoot := filepath.Join(root, ".agent", "skills")
	assertSkillpackSeeded(t, skillsRoot)
	assertSymlinkTo(t, filepath.Join(root, ".codex", "skills"), skillsRoot)
	assertSymlinkTo(t, filepath.Join(root, ".claude", "skills"), skillsRoot)
}

func TestCLI_Init_BootstrapAgentSkills_InvalidValue(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	setGitIdentity(t)

	root := t.TempDir()
	setKraHomeForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"init", "--root", root, "--context", "ctx", "--bootstrap", "unknown"})
	if code != exitUsage {
		t.Fatalf("init exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(err.String(), "unsupported --bootstrap") {
		t.Fatalf("stderr missing invalid bootstrap message: %q", err.String())
	}
}

func TestCLI_Init_BootstrapAgentSkills_InvalidValue_JSON(t *testing.T) {
	root := t.TempDir()
	setKraHomeForTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"init", "--format", "json", "--root", root, "--context", "ctx", "--bootstrap", "unknown"})
	if code != exitUsage {
		t.Fatalf("init json exit code = %d, want %d", code, exitUsage)
	}
	resp := decodeJSONResponse(t, out.String())
	if resp.OK || resp.Action != "init" || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected json response: %+v", resp)
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty in json mode: %q", err.String())
	}
}

func TestCLI_BootstrapAgentSkills_FailsWhenCurrentContextIsNotRoot(t *testing.T) {
	setKraHomeForTest(t)
	notRoot := t.TempDir()
	if err := paths.WriteCurrentContext(notRoot); err != nil {
		t.Fatalf("WriteCurrentContext() error: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"bootstrap", "agent-skills"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(err.String(), "does not look like a KRA_ROOT") {
		t.Fatalf("stderr missing root shape error: %q", err.String())
	}
}

func assertSymlinkTo(t *testing.T, path string, wantTarget string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not symlink", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval symlink %s: %v", path, err)
	}
	resolvedCanonical, resolvedOK := canonicalPath(resolved)
	if !resolvedOK {
		t.Fatalf("canonicalize resolved path failed: %s", resolved)
	}
	wantCanonical, wantOK := canonicalPath(wantTarget)
	if !wantOK {
		t.Fatalf("canonicalize target path failed: %s", wantTarget)
	}
	if resolvedCanonical != wantCanonical {
		t.Fatalf("%s -> %s, want %s", path, resolvedCanonical, wantCanonical)
	}
}

func jsonAnyArray(v any) []any {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]any); ok {
		return arr
	}
	return nil
}

func asJSONString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return strings.TrimSpace(string(b))
}

func assertSkillpackSeeded(t *testing.T, skillsRoot string) {
	t.Helper()
	checks := []struct {
		path     string
		contains string
	}{
		{path: filepath.Join(skillsRoot, ".kra-skillpack.yaml"), contains: `pack: "kra-flow"`},
		{path: filepath.Join(skillsRoot, "flow-investigation", "SKILL.md"), contains: "name: flow-investigation"},
		{path: filepath.Join(skillsRoot, "flow-investigation", "SKILL.md"), contains: "description:"},
		{path: filepath.Join(skillsRoot, "flow-execution", "SKILL.md"), contains: "name: flow-execution"},
		{path: filepath.Join(skillsRoot, "flow-execution", "SKILL.md"), contains: "description:"},
		{path: filepath.Join(skillsRoot, "flow-insight-capture", "SKILL.md"), contains: "name: flow-insight-capture"},
		{path: filepath.Join(skillsRoot, "flow-insight-capture", "SKILL.md"), contains: "description:"},
	}
	for _, c := range checks {
		data, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatalf("read %s: %v", c.path, err)
		}
		if !strings.Contains(string(data), c.contains) {
			t.Fatalf("%s missing marker %q", c.path, c.contains)
		}
	}
}
