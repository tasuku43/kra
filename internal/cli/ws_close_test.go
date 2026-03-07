package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/core/repospec"
	"github.com/tasuku43/kra/internal/core/repostore"
	"github.com/tasuku43/kra/internal/core/workspacerisk"
	"github.com/tasuku43/kra/internal/testutil"
)

type fakeCMUXCloseClient struct {
	closeErrByWorkspace map[string]error
	closedWorkspaceIDs  []string
}

func (f *fakeCMUXCloseClient) CloseWorkspace(_ context.Context, workspace string) error {
	f.closedWorkspaceIDs = append(f.closedWorkspaceIDs, workspace)
	if err := f.closeErrByWorkspace[workspace]; err != nil {
		return err
	}
	return nil
}

func TestCLI_WS_Close_Help_ShowsUsage(t *testing.T) {
	prepareCurrentRootForTest(t)
	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)

	code := c.Run([]string{"ws", "close", "--help"})
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "kra ws close") {
		t.Fatalf("stdout missing ws close usage: %q", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr not empty: %q", err.String())
	}
}

func TestCLI_WS_Close_ClosesMappedCMUXWorkspaceAndPrunesMapping(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")
	store := cmuxmap.NewStore(env.Root)
	if err := store.Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-WS-1", Ordinal: 1},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save cmux mapping: %v", err)
	}

	fake := &fakeCMUXCloseClient{}
	prev := newCMUXCloseClient
	newCMUXCloseClient = func() cmuxCloseClient { return fake }
	t.Cleanup(func() { newCMUXCloseClient = prev })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "close", "--id", "WS1", "--no-commit"})
	if code != exitOK {
		t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if got := fake.closedWorkspaceIDs; len(got) != 1 || got[0] != "CMUX-WS-1" {
		t.Fatalf("closed workspace ids = %v, want [CMUX-WS-1]", got)
	}
	mapping, lerr := store.Load()
	if lerr != nil {
		t.Fatalf("load cmux mapping: %v", lerr)
	}
	if _, ok := mapping.Workspaces["WS1"]; ok {
		t.Fatalf("cmux mapping should be removed for WS1: %+v", mapping.Workspaces["WS1"])
	}
}

func TestCLI_WS_Close_CMUXCloseFailure_DoesNotFailWorkspaceClose(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")
	store := cmuxmap.NewStore(env.Root)
	if err := store.Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		Workspaces: map[string]cmuxmap.WorkspaceMapping{
			"WS1": {
				NextOrdinal: 2,
				Entries: []cmuxmap.Entry{
					{CMUXWorkspaceID: "CMUX-WS-1", Ordinal: 1},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save cmux mapping: %v", err)
	}

	fake := &fakeCMUXCloseClient{
		closeErrByWorkspace: map[string]error{
			"CMUX-WS-1": errors.New("cmux close-workspace: boom"),
		},
	}
	prev := newCMUXCloseClient
	newCMUXCloseClient = func() cmuxCloseClient { return fake }
	t.Cleanup(func() { newCMUXCloseClient = prev })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "close", "--id", "WS1", "--no-commit"})
	if code != exitOK {
		t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "archive", "WS1")); statErr != nil {
		t.Fatalf("archive/WS1 should exist after close even when cmux close fails: %v", statErr)
	}
	mapping, lerr := store.Load()
	if lerr != nil {
		t.Fatalf("load cmux mapping: %v", lerr)
	}
	if _, ok := mapping.Workspaces["WS1"]; !ok {
		t.Fatalf("cmux mapping should remain when close fails")
	}
}

func TestCLI_WS_Close_ArchivesWorkspaceRemovesWorktreesCommitsAndUpdatesDB(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env, repoSpec := prepareActiveWorkspaceForCloseTest(t)

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "close", "--commit", "--id", "WS1"})
		if code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
		if strings.Contains(err.String(), "type yes to apply close on non-clean workspaces:") {
			t.Fatalf("clean close should not require confirmation: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err == nil {
		t.Fatalf("workspaces/WS1 should not exist after close")
	}
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err != nil {
		t.Fatalf("archive/WS1 should exist after close: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1", "repos")); err == nil {
		t.Fatalf("archive/WS1/repos should not exist after close")
	}
	metaBytes, readErr := os.ReadFile(filepath.Join(env.Root, "archive", "WS1", workspaceMetaFilename))
	if readErr != nil {
		t.Fatalf("read %s: %v", workspaceMetaFilename, readErr)
	}
	var meta workspaceMetaFile
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal %s: %v", workspaceMetaFilename, err)
	}
	if meta.Workspace.Status != "archived" {
		t.Fatalf("workspace meta status = %q, want %q", meta.Workspace.Status, "archived")
	}
	if len(meta.ReposRestore) != 1 {
		t.Fatalf("repos_restore length = %d, want %d", len(meta.ReposRestore), 1)
	}
	if got := meta.ReposRestore[0]; got.Alias != "r" || got.Branch != "WS1/test" {
		t.Fatalf("repos_restore[0] = %+v, want alias=%q branch=%q", got, "r", "WS1/test")
	}
	spec, err := repospec.Normalize(repoSpec)
	if err != nil {
		t.Fatalf("Normalize(repoSpec): %v", err)
	}
	barePath := repostore.StorePath(env.RepoPoolPath(), spec)
	cmd := exec.Command("git", "--git-dir", barePath, "show-ref", "--verify", "--quiet", "refs/heads/WS1/test")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("local branch should be deleted after close: refs/heads/WS1/test")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("git show-ref unexpected error: %v (output=%s)", err, strings.TrimSpace(string(out)))
	}

	subj := strings.TrimSpace(mustGitOutput(t, env.Root, "log", "-1", "--pretty=%s"))
	if subj != "archive: WS1" {
		t.Fatalf("commit subject = %q, want %q", subj, "archive: WS1")
	}
	wsStatus := strings.TrimSpace(mustGitOutput(t, env.Root, "status", "--short", "--", filepath.ToSlash(filepath.Join("workspaces", "WS1"))))
	if wsStatus != "" {
		t.Fatalf("workspaces/WS1 should not remain as unstaged deletion after close commit: %q", wsStatus)
	}

}

func TestCLI_WS_Close_DirtyRepo_PromptsAndCanAbort(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env, _ := prepareActiveWorkspaceForCloseTest(t)

	worktreePath := filepath.Join(env.Root, "workspaces", "WS1", "repos", "r")
	if err := os.WriteFile(filepath.Join(worktreePath, "DIRTY.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("n\n")

		code := c.Run([]string{"ws", "close", "--id", "WS1"})
		if code != exitError {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
		}
		if !strings.Contains(err.String(), "type yes to apply close on non-clean workspaces:") {
			t.Fatalf("stderr missing confirmation prompt: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err != nil {
		t.Fatalf("workspaces/WS1 should still exist after abort: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err == nil {
		t.Fatalf("archive/WS1 should not exist after abort")
	}
}

func TestCLI_WS_Close_EmptyRecordWarnPolicy_ContinuesAndWarns(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env, _ := prepareActiveWorkspaceForCloseTest(t)
	writeWSCloseEmptyRecordPolicy(t, env.Root, "warn")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "close", "--no-commit", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if !strings.Contains(out.String(), "Warnings:") || !strings.Contains(out.String(), "coverage: empty") {
		t.Fatalf("stdout should include empty-record warning: %q", out.String())
	}
	if strings.Contains(err.String(), "type yes to apply close on non-clean workspaces:") {
		t.Fatalf("warn policy should not require confirmation: %q", err.String())
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "archive", "WS1")); statErr != nil {
		t.Fatalf("archive/WS1 should exist after close: %v", statErr)
	}
}

func TestCLI_WS_Close_EmptyRecordRequireConfirmation_PromptsAndCanAbort(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env, _ := prepareActiveWorkspaceForCloseTest(t)
	writeWSCloseEmptyRecordPolicy(t, env.Root, "require-confirmation")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	c.In = strings.NewReader("n\n")

	code := c.Run([]string{"ws", "close", "--no-commit", "--id", "WS1"})
	if code != exitError {
		t.Fatalf("ws close exit code = %d, want %d (stderr=%q stdout=%q)", code, exitError, err.String(), out.String())
	}
	if !strings.Contains(out.String(), "Plan:") || !strings.Contains(out.String(), "coverage: empty") {
		t.Fatalf("stdout should include empty-record plan reason: %q", out.String())
	}
	if !strings.Contains(err.String(), "type yes to apply close on non-clean workspaces:") {
		t.Fatalf("stderr should include confirmation prompt: %q", err.String())
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); statErr != nil {
		t.Fatalf("workspace should remain after declined confirmation: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "archive", "WS1")); !os.IsNotExist(statErr) {
		t.Fatalf("archive should not exist after declined confirmation, stat err=%v", statErr)
	}
}

func TestCLI_WS_Close_JSON_DryRun_EmptyRecordRequireConfirmationRequiresForce(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env, _ := prepareActiveWorkspaceForCloseTest(t)
	writeWSCloseEmptyRecordPolicy(t, env.Root, "require-confirmation")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "close", "--dry-run", "--format", "json", "--id", "WS1"})
	if code != exitError {
		t.Fatalf("ws close dry-run exit code = %d, want %d (stderr=%q stdout=%q)", code, exitError, err.String(), out.String())
	}

	var resp struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
		Result struct {
			Executable           bool              `json:"executable"`
			RequiresConfirmation bool              `json:"requires_confirmation"`
			RequiresForce        bool              `json:"requires_force"`
			Checks               []jsonDryRunCheck `json:"checks"`
			Coverage             map[string]any    `json:"coverage"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v (raw=%q)", err, out.String())
	}
	if resp.OK {
		t.Fatalf("dry-run response should not be ok when force is required: %+v", resp)
	}
	if resp.Action != "ws.close.dry-run" {
		t.Fatalf("action = %q, want %q", resp.Action, "ws.close.dry-run")
	}
	if resp.Result.Executable {
		t.Fatalf("dry-run should not be executable without force: %+v", resp.Result)
	}
	if !resp.Result.RequiresConfirmation || !resp.Result.RequiresForce {
		t.Fatalf("dry-run should require confirmation/force: %+v", resp.Result)
	}
	if got := resp.Result.Coverage["state"]; got != string(workspaceOutputCoverageEmpty) {
		t.Fatalf("coverage.state = %v, want %q", got, workspaceOutputCoverageEmpty)
	}
	if !jsonChecksContainMessage(resp.Result.Checks, "output_coverage_gate", "requires --force") {
		t.Fatalf("checks should mention output coverage force gate: %+v", resp.Result.Checks)
	}
}

func TestCLI_WS_Close_JSON_DryRun_Force_AllowsEmptyRecordRequireConfirmation(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env, _ := prepareActiveWorkspaceForCloseTest(t)
	writeWSCloseEmptyRecordPolicy(t, env.Root, "require-confirmation")

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "close", "--dry-run", "--format", "json", "--force", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws close dry-run --force exit code = %d, want %d (stderr=%q stdout=%q)", code, exitOK, err.String(), out.String())
	}

	var resp struct {
		OK     bool   `json:"ok"`
		Action string `json:"action"`
		Result struct {
			Executable           bool              `json:"executable"`
			RequiresConfirmation bool              `json:"requires_confirmation"`
			RequiresForce        bool              `json:"requires_force"`
			Checks               []jsonDryRunCheck `json:"checks"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal error: %v (raw=%q)", err, out.String())
	}
	if !resp.OK || resp.Action != "ws.close.dry-run" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if !resp.Result.Executable {
		t.Fatalf("dry-run should be executable with force: %+v", resp.Result)
	}
	if !jsonChecksContainMessage(resp.Result.Checks, "output_coverage_gate", "accepted") {
		t.Fatalf("checks should mention accepted empty coverage: %+v", resp.Result.Checks)
	}
}

func TestCLI_WS_Close_SelectorModeWithoutTTY_Errors(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		c.In = strings.NewReader("")

		code := c.Run([]string{"ws", "close"})
		if code != exitUsage {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitUsage, err.String())
		}
		if !strings.Contains(err.String(), "ws close requires --id <id> or active workspace context") && !strings.Contains(err.String(), "Usage:") {
			t.Fatalf("stderr missing id requirement: %q", err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "workspaces", "WS1")); err != nil {
		t.Fatalf("workspace should remain: %v", err)
	}
}

func TestCLI_WS_Close_ShiftsProcessCWDWhenInsideTargetWorkspace(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	wsDir := filepath.Join(env.Root, "workspaces", "WS1")
	if err := os.Chdir(wsDir); err != nil {
		t.Fatalf("Chdir(%s) error: %v", wsDir, err)
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	c := New(&out, &errBuf)
	code := c.Run([]string{"ws", "close", "--no-commit", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, errBuf.String())
	}
	afterWD, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatalf("Getwd() after close error: %v", wdErr)
	}
	afterResolved := afterWD
	if resolved, err := filepath.EvalSymlinks(afterWD); err == nil {
		afterResolved = resolved
	}
	rootResolved := env.Root
	if resolved, err := filepath.EvalSymlinks(env.Root); err == nil {
		rootResolved = resolved
	}
	if afterResolved != rootResolved {
		t.Fatalf("process cwd = %q (resolved=%q), want %q (resolved=%q)", afterWD, afterResolved, env.Root, rootResolved)
	}
}

func TestCLI_WS_Close_AllowsUnrelatedPreStagedChangesOutsideWorkspaceAllowlist(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")
	unrelated := filepath.Join(env.Root, "UNRELATED.md")
	if err := os.WriteFile(unrelated, []byte("keep staged\n"), 0o644); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}
	runGit(t, env.Root, "add", "UNRELATED.md")

	{
		var out bytes.Buffer
		var err bytes.Buffer
		c := New(&out, &err)
		code := c.Run([]string{"ws", "close", "--commit", "--id", "WS1"})
		if code != exitOK {
			t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
		}
	}

	if _, err := os.Stat(filepath.Join(env.Root, "archive", "WS1")); err != nil {
		t.Fatalf("archive/WS1 should exist after close: %v", err)
	}

	staged := strings.TrimSpace(mustGitOutput(t, env.Root, "diff", "--cached", "--name-only"))
	if !strings.Contains(staged, "UNRELATED.md") {
		t.Fatalf("unrelated staged file should remain staged: %q", staged)
	}

	subj := strings.TrimSpace(mustGitOutput(t, env.Root, "log", "-1", "--pretty=%s"))
	if subj != "archive: WS1" {
		t.Fatalf("commit subject = %q, want %q", subj, "archive: WS1")
	}
}

func TestCLI_WS_Close_RemovesLifecycleJournalOnSuccess(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env, _ := prepareActiveWorkspaceForCloseTest(t)

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "close", "--commit", "--id", "WS1"})
	if code != exitOK {
		t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitOK, err.String())
	}
	if _, statErr := os.Stat(wsCloseJournalPath(env.Root, "WS1")); !os.IsNotExist(statErr) {
		t.Fatalf("ws close journal should be removed on success, stat err=%v", statErr)
	}
}

func TestCLI_WS_Close_PostRenameFailure_PreservesLifecycleJournal(t *testing.T) {
	testutil.RequireCommand(t, "git")

	env, _ := prepareActiveWorkspaceForCloseTest(t)

	prev := commitArchiveChangeFn
	commitArchiveChangeFn = func(ctx context.Context, root string, workspaceID string, expectedArchiveFiles []string) (string, error) {
		return "", errors.New("boom archive commit")
	}
	t.Cleanup(func() { commitArchiveChangeFn = prev })

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "close", "--commit", "--id", "WS1"})
	if code != exitError {
		t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
	}
	if _, statErr := os.Stat(filepath.Join(env.Root, "archive", "WS1")); statErr != nil {
		t.Fatalf("archive/WS1 should remain after post-rename failure: %v", statErr)
	}
	journal, loadErr := loadWSCloseLifecycleJournal(env.Root, "WS1")
	if loadErr != nil {
		t.Fatalf("load ws close journal: %v", loadErr)
	}
	if journal.Phase != wsClosePhaseWorkspaceRenamed {
		t.Fatalf("journal phase = %q, want %q", journal.Phase, wsClosePhaseWorkspaceRenamed)
	}
	if journal.ClosePreCommitSHA == "" {
		t.Fatalf("close_pre_commit_sha should be recorded")
	}
	if journal.ArchiveCommitSHA != "" {
		t.Fatalf("archive_commit_sha should be empty before resume, got %q", journal.ArchiveCommitSHA)
	}
}

func TestCLI_WS_Close_RefusesWhenUnfinishedLifecycleJournalExists(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")

	journal := newWSCloseLifecycleJournal("WS1", true, time.Now().Unix())
	if err := saveWSCloseLifecycleJournal(env.Root, journal); err != nil {
		t.Fatalf("save ws close journal: %v", err)
	}

	var out bytes.Buffer
	var err bytes.Buffer
	c := New(&out, &err)
	code := c.Run([]string{"ws", "close", "--no-commit", "--id", "WS1"})
	if code != exitError {
		t.Fatalf("ws close exit code = %d, want %d (stderr=%q)", code, exitError, err.String())
	}
	if !strings.Contains(err.String(), "unfinished ws close recovery exists") {
		t.Fatalf("stderr should mention unfinished journal: %q", err.String())
	}
}

func TestPrintCloseRiskSection_UsesSharedSpacingAndIndent(t *testing.T) {
	var out bytes.Buffer
	items := []workspaceRiskDetail{
		{
			id:   "WS1",
			risk: workspacerisk.WorkspaceRiskDirty,
			perRepo: []repoRiskItem{
				{alias: "repo-a", state: workspacerisk.RepoStateDirty},
			},
		},
		{
			id:      "WS2",
			risk:    workspacerisk.WorkspaceRiskClean,
			perRepo: []repoRiskItem{{alias: "repo-b", state: workspacerisk.RepoStateClean}},
		},
	}

	printRiskSection(&out, items, false)
	got := out.String()

	if !strings.HasPrefix(got, "Plan:\n") {
		t.Fatalf("close plan section should start with plan heading: %q", got)
	}
	if !strings.Contains(got, "\n  • close 2 workspaces\n") {
		t.Fatalf("plan heading row mismatch: %q", got)
	}
	if !strings.Contains(got, "\n  • workspace WS1\n") {
		t.Fatalf("workspace label row mismatch: %q", got)
	}
	if !strings.Contains(got, "\n    └─ repo-a\n") {
		t.Fatalf("repo tree row mismatch: %q", got)
	}
	if !strings.Contains(got, "\n       risk: dirty\n") {
		t.Fatalf("repo risk row mismatch: %q", got)
	}
	if !strings.Contains(got, "\n       sync: upstream=(none) ahead=0 behind=0\n") {
		t.Fatalf("sync row mismatch: %q", got)
	}
	if !strings.Contains(got, "\n  • workspace WS2\n") {
		t.Fatalf("second workspace label row mismatch: %q", got)
	}
	if !strings.Contains(got, "\n    └─ repo-b\n") {
		t.Fatalf("repo risk detail indentation mismatch: %q", got)
	}
}

func TestEnsureRootGitWorktree_AllowsNestedRootInSameGitWorktree(t *testing.T) {
	testutil.RequireCommand(t, "git")

	parent := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	run(parent, "init", "-b", "main")

	root := filepath.Join(parent, "nested", "work")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir nested root: %v", err)
	}
	if err := ensureRootGitWorktree(context.Background(), root); err != nil {
		t.Fatalf("ensureRootGitWorktree() error = %v, want nil", err)
	}
}

func TestEnsureRootGitWorktree_RejectsRootOutsideGitWorktree(t *testing.T) {
	testutil.RequireCommand(t, "git")

	repo := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	run(repo, "init", "-b", "main")

	outside := t.TempDir()
	err := ensureRootGitWorktree(context.Background(), outside)
	if err == nil {
		t.Fatalf("ensureRootGitWorktree() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "KRA_ROOT must be a git working tree") {
		t.Fatalf("error = %q, want git working tree guidance", err.Error())
	}
}

func TestCommitArchiveChange_AllowlistsNestedRootRelativePaths(t *testing.T) {
	testutil.RequireCommand(t, "git")

	parent := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	run(parent, "init", "-b", "main")
	run(parent, "config", "user.email", "test@example.com")
	run(parent, "config", "user.name", "test")

	root := filepath.Join(parent, "tasuku-yamashita", "work")
	wsID := "DEMO-0000"
	archiveDir := filepath.Join(root, "archive", wsID)
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatalf("mkdir archive dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte(".claude/settings.local.json\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	metaPath := filepath.Join(archiveDir, workspaceMetaFilename)
	if err := os.WriteFile(metaPath, []byte(`{"workspace":{"id":"DEMO-0000","status":"archived"}}`), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	ignoredPath := filepath.Join(archiveDir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(ignoredPath), 0o755); err != nil {
		t.Fatalf("mkdir ignored dir: %v", err)
	}
	if err := os.WriteFile(ignoredPath, []byte("{\"local\":true}\n"), 0o644); err != nil {
		t.Fatalf("write ignored settings: %v", err)
	}

	sha, err := commitArchiveChange(context.Background(), root, wsID, []string{
		workspaceMetaFilename,
		".claude/settings.local.json",
	})
	if err != nil {
		t.Fatalf("commitArchiveChange() error = %v, want nil", err)
	}
	if strings.TrimSpace(sha) == "" {
		t.Fatalf("commitArchiveChange() sha is empty")
	}
}

func mustGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v (output=%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out)
}

func prepareActiveWorkspaceForCloseTest(t *testing.T) (testutil.Env, string) {
	t.Helper()

	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)

	repoSpec := prepareRemoteRepoSpec(t, func(dir string, args ...string) {
		runGit(t, dir, args...)
	})
	spec, err := repospec.Normalize(repoSpec)
	if err != nil {
		t.Fatalf("Normalize(repoSpec): %v", err)
	}
	barePath := repostore.StorePath(env.RepoPoolPath(), spec)
	seedRepoPoolBareFixtureOrFetch(t, repoSpec, barePath, fixtureRemoteDefaultBranch)

	wsPath := filepath.Join(env.Root, "workspaces", "WS1")
	if err := os.MkdirAll(filepath.Join(wsPath, "repos"), 0o755); err != nil {
		t.Fatalf("mkdir workspace repos: %v", err)
	}
	now := time.Now().Unix()
	meta := newWorkspaceMetaFileForCreate("WS1", "WS1", "", now)
	meta.ReposRestore = []workspaceMetaRepoRestore{{
		RepoUID:   spec.Host + "/" + spec.Owner + "/" + spec.Repo,
		RepoKey:   spec.Owner + "/" + spec.Repo,
		RemoteURL: repoSpec,
		Alias:     spec.Repo,
		Branch:    "WS1/test",
		BaseRef:   "origin/main",
	}}
	if err := writeWorkspaceMetaFile(wsPath, meta); err != nil {
		t.Fatalf("write workspace meta: %v", err)
	}

	runGit(t, "", "--git-dir", barePath, "branch", "WS1/test", "origin/main")
	runGit(t, "", "--git-dir", barePath, "worktree", "add", filepath.Join(wsPath, "repos", spec.Repo), "WS1/test")

	return env, repoSpec
}

func writeWSCloseEmptyRecordPolicy(t *testing.T, root string, policy string) {
	t.Helper()

	rootConfigPath := filepath.Join(root, ".kra", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(rootConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir root config dir: %v", err)
	}
	content := "workspace:\n  defaults:\n    template: default\n  close:\n    empty_record_policy: " + policy + "\n"
	if err := os.WriteFile(rootConfigPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write root config: %v", err)
	}
}

type jsonDryRunCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func jsonChecksContainMessage(checks []jsonDryRunCheck, name string, wantSubstring string) bool {
	for _, check := range checks {
		if check.Name == name && strings.Contains(check.Message, wantSubstring) {
			return true
		}
	}
	return false
}
