package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/testutil"
)

func TestCLI_Doctor_Help(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"doctor", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra doctor") {
		t.Fatalf("stdout missing doctor usage: %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Fatalf("stderr not empty: %q", errBuf.String())
	}
}

func TestCLI_Doctor_HealthyRoot(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsPath := filepath.Join(env.Root, "workspaces", "WS-1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := writeWorkspaceMetaFile(wsPath, newWorkspaceMetaFileForCreate("WS-1", "title", "", 100)); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	if err := c.touchStateRegistry(env.Root); err != nil {
		t.Fatalf("touchStateRegistry: %v", err)
	}

	code := c.Run([]string{"doctor"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	got := out.String()
	if !strings.Contains(got, "Result:") || !strings.Contains(got, "warn: 0") || !strings.Contains(got, "error: 0") {
		t.Fatalf("stdout missing healthy summary: %q", got)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("stderr not empty: %q", errBuf.String())
	}
}

func TestCLI_Doctor_DetectsInvalidWorkspaceMeta(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsPath := filepath.Join(env.Root, "workspaces", "WS-1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	metaPath := filepath.Join(wsPath, workspaceMetaFilename)
	if err := os.WriteFile(metaPath, []byte("{broken-json"), 0o644); err != nil {
		t.Fatalf("write broken meta: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	if err := c.touchStateRegistry(env.Root); err != nil {
		t.Fatalf("touchStateRegistry: %v", err)
	}

	code := c.Run([]string{"doctor"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	got := out.String()
	if !strings.Contains(got, "[workspace_meta_invalid]") {
		t.Fatalf("stdout missing invalid meta finding: %q", got)
	}
}

func TestCLI_Doctor_DetectsBindingMissingWorktree(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsPath := filepath.Join(env.Root, "workspaces", "WS-1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	meta := newWorkspaceMetaFileForCreate("WS-1", "title", "", 100)
	meta.ReposRestore = []workspaceMetaRepoRestore{{
		RepoUID: "github.com/example/repo",
		RepoKey: "example/repo",
		Alias:   "repo",
		Branch:  "WS-1",
		BaseRef: "origin/main",
	}}
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	if err := c.touchStateRegistry(env.Root); err != nil {
		t.Fatalf("touchStateRegistry: %v", err)
	}

	code := c.Run([]string{"doctor"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout=%q)", code, exitOK, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "[binding_missing_worktree]") || !strings.Contains(got, "warn: 1") {
		t.Fatalf("stdout missing warning finding: %q", got)
	}
}

func TestCLI_Doctor_DetectsLegacyWorkstateFile(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	workstatePath := filepath.Join(env.Root, ".kra", "state", "workspace-workstate.json")
	if err := os.MkdirAll(filepath.Dir(workstatePath), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(workstatePath, []byte("{\"WS-1\":\"todo\"}\n"), 0o644); err != nil {
		t.Fatalf("write legacy workstate file: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	if err := c.touchStateRegistry(env.Root); err != nil {
		t.Fatalf("touchStateRegistry: %v", err)
	}

	code := c.Run([]string{"doctor", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout=%q stderr=%q)", code, exitOK, out.String(), errBuf.String())
	}
	var resp struct {
		OK     bool         `json:"ok"`
		Action string       `json:"action"`
		Result doctorReport `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v (raw=%q)", err, out.String())
	}
	if !resp.OK || resp.Action != "doctor" {
		t.Fatalf("unexpected response head: %+v", resp)
	}
	if !hasDoctorFinding(resp.Result.Findings, "legacy_workstate_file", workstatePath) {
		t.Fatalf("legacy workstate finding missing: %+v", resp.Result.Findings)
	}
}

func TestCLI_Doctor_DetectsLegacyBaselineInUse(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsPath := filepath.Join(env.Root, "workspaces", "WS-1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := writeWorkspaceMetaFile(wsPath, newWorkspaceMetaFileForCreate("WS-1", "title", "", 100)); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}
	legacyBaselinePath := writeLegacyBaselineFile(t, env.Root, "WS-1")

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	if err := c.touchStateRegistry(env.Root); err != nil {
		t.Fatalf("touchStateRegistry: %v", err)
	}

	code := c.Run([]string{"doctor", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout=%q stderr=%q)", code, exitOK, out.String(), errBuf.String())
	}
	var resp struct {
		Result doctorReport `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v (raw=%q)", err, out.String())
	}
	if !hasDoctorFinding(resp.Result.Findings, "legacy_baseline_in_use", legacyBaselinePath) {
		t.Fatalf("legacy baseline in use finding missing: %+v", resp.Result.Findings)
	}
}

func TestCLI_Doctor_DetectsLegacyBaselineCheckFailed(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsPath := filepath.Join(env.Root, "workspaces", "WS-1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsPath, workspaceMetaFilename), []byte("{broken-json"), 0o644); err != nil {
		t.Fatalf("write broken meta: %v", err)
	}
	legacyBaselinePath := writeLegacyBaselineFile(t, env.Root, "WS-1")

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	if err := c.touchStateRegistry(env.Root); err != nil {
		t.Fatalf("touchStateRegistry: %v", err)
	}

	code := c.Run([]string{"doctor", "--format", "json"})
	if code != exitError {
		t.Fatalf("exit code = %d, want %d (stdout=%q stderr=%q)", code, exitError, out.String(), errBuf.String())
	}
	var resp struct {
		Result doctorReport `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v (raw=%q)", err, out.String())
	}
	if !hasDoctorFinding(resp.Result.Findings, "legacy_baseline_check_failed", legacyBaselinePath) {
		t.Fatalf("legacy baseline check failed finding missing: %+v", resp.Result.Findings)
	}
}

func TestCLI_Doctor_JSONAndFixValidation(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"doctor", "--format", "json", "--fix"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	var resp cliJSONResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v (raw=%q)", err, out.String())
	}
	if resp.OK || resp.Action != "doctor.fix" || resp.Error == nil || resp.Error.Code != "invalid_argument" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("stderr should be empty in json mode, got %q", errBuf.String())
	}
}

func TestCLI_Doctor_FixPlan_JSON(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	lockDir := filepath.Join(env.Root, ".kra", "locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	lockPath := filepath.Join(lockDir, "WS-1.lock")
	if err := os.WriteFile(lockPath, []byte("pid=999999\n"), 0o644); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"doctor", "--fix", "--plan", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout=%q stderr=%q)", code, exitOK, out.String(), errBuf.String())
	}
	var resp struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
		Result struct {
			Mode    string `json:"mode"`
			Summary struct {
				Planned int `json:"planned"`
				Applied int `json:"applied"`
				Skipped int `json:"skipped"`
				Failed  int `json:"failed"`
			} `json:"summary"`
			Actions []doctorFixAction `json:"actions"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v (raw=%q)", err, out.String())
	}
	if !resp.OK || resp.Action != "doctor.fix" {
		t.Fatalf("unexpected response head: %+v", resp)
	}
	if resp.Result.Mode != "plan" || resp.Result.Summary.Planned == 0 {
		t.Fatalf("unexpected fix plan result: %+v", resp.Result)
	}
}

func TestCLI_Doctor_FixPlan_JSON_SeparatesMigrationFromCleanup(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsKeepPath := filepath.Join(env.Root, "workspaces", "WS-KEEP")
	if err := os.MkdirAll(wsKeepPath, 0o755); err != nil {
		t.Fatalf("mkdir keep workspace: %v", err)
	}
	if err := writeWorkspaceMetaFile(wsKeepPath, newWorkspaceMetaFileForCreate("WS-KEEP", "keep", "", 100)); err != nil {
		t.Fatalf("write keep workspace meta: %v", err)
	}
	keepBaselinePath := writeLegacyBaselineFile(t, env.Root, "WS-KEEP")

	wsDropPath := filepath.Join(env.Root, "workspaces", "WS-DROP")
	if err := os.MkdirAll(wsDropPath, 0o755); err != nil {
		t.Fatalf("mkdir drop workspace: %v", err)
	}
	metaDrop := newWorkspaceMetaFileForCreate("WS-DROP", "drop", "", 100)
	metaDrop.Baseline = &workspaceBaseline{Version: 1, CreatedAt: 100}
	if err := writeWorkspaceMetaFile(wsDropPath, metaDrop); err != nil {
		t.Fatalf("write drop workspace meta: %v", err)
	}
	dropBaselinePath := writeLegacyBaselineFile(t, env.Root, "WS-DROP")

	workstatePath := filepath.Join(env.Root, ".kra", "state", "workspace-workstate.json")
	if err := os.MkdirAll(filepath.Dir(workstatePath), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(workstatePath, []byte("{\"WS-DROP\":\"in-progress\"}\n"), 0o644); err != nil {
		t.Fatalf("write legacy workstate file: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	if err := c.touchStateRegistry(env.Root); err != nil {
		t.Fatalf("touchStateRegistry: %v", err)
	}

	code := c.Run([]string{"doctor", "--fix", "--plan", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout=%q stderr=%q)", code, exitOK, out.String(), errBuf.String())
	}
	var resp struct {
		OK     bool            `json:"ok"`
		Action string          `json:"action"`
		Result doctorFixResult `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v (raw=%q)", err, out.String())
	}
	if !resp.OK || resp.Action != "doctor.fix" {
		t.Fatalf("unexpected response head: %+v", resp)
	}
	if !hasDoctorAction(resp.Result.Actions, "remove_legacy_workstate_file", workstatePath) {
		t.Fatalf("legacy workstate cleanup action missing: %+v", resp.Result.Actions)
	}
	if !hasDoctorAction(resp.Result.Actions, "remove_legacy_baseline_file", dropBaselinePath) {
		t.Fatalf("safe legacy baseline cleanup action missing: %+v", resp.Result.Actions)
	}
	if !hasDoctorAction(resp.Result.Actions, "migrate_legacy_baseline_file", keepBaselinePath) {
		t.Fatalf("legacy baseline in use should be migrated explicitly: %+v", resp.Result.Actions)
	}
}

func TestCLI_Doctor_FixApply_RemovesStaleLock(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	lockDir := filepath.Join(env.Root, ".kra", "locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	lockPath := filepath.Join(lockDir, "WS-1.lock")
	if err := os.WriteFile(lockPath, []byte("pid=999999\n"), 0o644); err != nil {
		t.Fatalf("write lock file: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"doctor", "--fix", "--apply", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout=%q stderr=%q)", code, exitOK, out.String(), errBuf.String())
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock file should be removed, stat err=%v", err)
	}
}

func TestCLI_Doctor_FixApply_RemovesLegacyStateFiles(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsPath := filepath.Join(env.Root, "workspaces", "WS-1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	meta := newWorkspaceMetaFileForCreate("WS-1", "title", "", 100)
	meta.Baseline = &workspaceBaseline{Version: 1, CreatedAt: 100}
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}
	legacyBaselinePath := writeLegacyBaselineFile(t, env.Root, "WS-1")

	workstatePath := filepath.Join(env.Root, ".kra", "state", "workspace-workstate.json")
	if err := os.MkdirAll(filepath.Dir(workstatePath), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(workstatePath, []byte("{\"WS-1\":\"todo\"}\n"), 0o644); err != nil {
		t.Fatalf("write legacy workstate file: %v", err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	if err := c.touchStateRegistry(env.Root); err != nil {
		t.Fatalf("touchStateRegistry: %v", err)
	}

	code := c.Run([]string{"doctor", "--fix", "--apply", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout=%q stderr=%q)", code, exitOK, out.String(), errBuf.String())
	}

	if _, err := os.Stat(workstatePath); !os.IsNotExist(err) {
		t.Fatalf("legacy workstate file should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(legacyBaselinePath); !os.IsNotExist(err) {
		t.Fatalf("legacy baseline file should be removed, stat err=%v", err)
	}
}

func TestCLI_Doctor_FixApply_MigratesLegacyBaselineIntoMeta(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	wsPath := filepath.Join(env.Root, "workspaces", "WS-1")
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := writeWorkspaceMetaFile(wsPath, newWorkspaceMetaFileForCreate("WS-1", "title", "", 100)); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}
	legacyBaselinePath := writeLegacyBaselineFile(t, env.Root, "WS-1")

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	if err := c.touchStateRegistry(env.Root); err != nil {
		t.Fatalf("touchStateRegistry: %v", err)
	}

	code := c.Run([]string{"doctor", "--fix", "--apply", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout=%q stderr=%q)", code, exitOK, out.String(), errBuf.String())
	}

	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		t.Fatalf("load workspace meta: %v", err)
	}
	if meta.Baseline == nil {
		t.Fatalf("baseline should be migrated into workspace meta")
	}
	if meta.Baseline.Version != 1 || meta.Baseline.CreatedAt != 100 {
		t.Fatalf("unexpected migrated baseline: %+v", *meta.Baseline)
	}
	if _, err := os.Stat(legacyBaselinePath); !os.IsNotExist(err) {
		t.Fatalf("legacy baseline file should be removed after migration, stat err=%v", err)
	}
}

func TestCLI_Doctor_FixApply_RegistersCurrentRoot(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"doctor", "--fix", "--apply", "--format", "json"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout=%q stderr=%q)", code, exitOK, out.String(), errBuf.String())
	}

	out.Reset()
	errBuf.Reset()
	code = c.Run([]string{"doctor", "--format", "json"})
	if code != exitOK {
		t.Fatalf("doctor after fix exit code = %d, want %d (stdout=%q stderr=%q)", code, exitOK, out.String(), errBuf.String())
	}
	var resp struct {
		Result doctorReport `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v (raw=%q)", err, out.String())
	}
	for _, f := range resp.Result.Findings {
		if f.Code == "root_not_registered" {
			t.Fatalf("root_not_registered should be repaired, findings=%+v", resp.Result.Findings)
		}
	}
}

func TestCLI_Doctor_FixFlagValidation(t *testing.T) {
	env := testutil.NewEnv(t)
	env.EnsureRootLayout(t)

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)

	code := c.Run([]string{"doctor", "--format", "json", "--plan"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}

	code = c.Run([]string{"doctor", "--format", "json", "--fix", "--plan", "--apply"})
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
}

func writeLegacyBaselineFile(t *testing.T, root string, workspaceID string) string {
	t.Helper()

	path := workspaceBaselinePath(root, workspaceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir baseline dir: %v", err)
	}
	baseline := workspaceBaseline{
		Version:   1,
		CreatedAt: 100,
		Repos:     map[string]workspaceBaselineRepo{},
		FS:        map[string]string{},
	}
	raw, err := json.Marshal(baseline)
	if err != nil {
		t.Fatalf("marshal legacy baseline: %v", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write legacy baseline: %v", err)
	}
	return path
}

func hasDoctorFinding(findings []doctorFinding, code string, target string) bool {
	for _, f := range findings {
		if f.Code == code && f.Target == target {
			return true
		}
	}
	return false
}

func hasDoctorAction(actions []doctorFixAction, kind string, target string) bool {
	for _, a := range actions {
		if a.Kind == kind && a.Target == target {
			return true
		}
	}
	return false
}
