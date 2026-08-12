package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/tasuku43/kra/internal/app/cmuxtodo"
	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/testutil"
)

type fakeCMUXTodoReconciler struct {
	requests []cmuxtodo.Request
	run      func(cmuxTodoRequest cmuxtodo.Request) (cmuxtodo.Result, error)
}

func (f *fakeCMUXTodoReconciler) Reconcile(_ context.Context, request cmuxtodo.Request) (cmuxtodo.Result, error) {
	f.requests = append(f.requests, request)
	if f.run != nil {
		return f.run(request)
	}
	return cmuxtodo.Result{WorkspaceID: request.WorkspaceID}, nil
}

func installFakeCMUXTodoReconciler(t *testing.T, fake *fakeCMUXTodoReconciler) {
	t.Helper()
	original := newCMUXTodoReconciler
	newCMUXTodoReconciler = func() cmuxTodoReconciler { return fake }
	t.Cleanup(func() { newCMUXTodoReconciler = original })
}

func TestCLI_WSTaskSync_JSONProjectsNativeTodo(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")
	fake := &fakeCMUXTodoReconciler{run: func(request cmuxtodo.Request) (cmuxtodo.Result, error) {
		return cmuxtodo.Result{
			WorkspaceID:     request.WorkspaceID,
			CMUXWorkspaceID: "cmux-1",
			Changed:         true,
			ManagedCount:    2,
			ForeignCount:    1,
			Operations: []cmuxtodo.Operation{
				{Kind: cmuxtodo.OperationAdd, TaskID: "TASK-001"},
				{Kind: cmuxtodo.OperationSetState, TaskID: "TASK-002"},
				{Kind: cmuxtodo.OperationPullStatus, TaskID: "TASK-003"},
			},
		}, nil
	}}
	installFakeCMUXTodoReconciler(t, fake)

	var out bytes.Buffer
	var stderr bytes.Buffer
	code := New(&out, &stderr).Run([]string{"ws", "task", "sync", "--id", "WS1", "--prefer", "kra", "--format", "json"})
	if code != exitOK || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if len(fake.requests) != 1 {
		t.Fatalf("requests = %#v", fake.requests)
	}
	request := fake.requests[0]
	if request.Root != env.Root || request.WorkspaceID != "WS1" || request.Scope != "active" || request.Target != cmuxtodo.TargetWorkspace || request.Prefer != cmuxtodo.PreferKRA {
		t.Fatalf("request = %#v", request)
	}
	response := decodeJSONResponse(t, out.String())
	if !response.OK || response.Action != "ws.task.sync" || response.WorkspaceID != "WS1" {
		t.Fatalf("response = %#v", response)
	}
	sync, ok := response.Result["sync"].(map[string]any)
	if !ok || sync["state"] != "synced" || sync["created"] != float64(1) || sync["pushed"] != float64(1) || sync["pulled"] != float64(1) {
		t.Fatalf("sync = %#v", response.Result["sync"])
	}
}

func TestCLI_WSTaskSync_UnmappedIsSkipped(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")
	fake := &fakeCMUXTodoReconciler{run: func(request cmuxtodo.Request) (cmuxtodo.Result, error) {
		return cmuxtodo.Result{WorkspaceID: request.WorkspaceID}, cmuxtodo.ErrNotMapped
	}}
	installFakeCMUXTodoReconciler(t, fake)

	var out bytes.Buffer
	var stderr bytes.Buffer
	code := New(&out, &stderr).Run([]string{"ws", "task", "sync", "--id", "WS1", "--format", "json"})
	if code != exitOK || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	response := decodeJSONResponse(t, out.String())
	sync := response.Result["sync"].(map[string]any)
	if !response.OK || sync["state"] != "skipped" || sync["warning"] == "" {
		t.Fatalf("response = %#v", response)
	}
}

func TestCLI_WSTaskSync_ConflictRequiresExplicitPreference(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")
	fake := &fakeCMUXTodoReconciler{run: func(request cmuxtodo.Request) (cmuxtodo.Result, error) {
		return cmuxtodo.Result{
			WorkspaceID: request.WorkspaceID,
			Conflicts: []cmuxtodo.Conflict{{
				TaskID: "TASK-001",
				Kind:   cmuxtodo.ConflictState,
				Detail: "both sides changed",
			}},
		}, cmuxtodo.ErrConflict
	}}
	installFakeCMUXTodoReconciler(t, fake)

	var out bytes.Buffer
	var stderr bytes.Buffer
	code := New(&out, &stderr).Run([]string{"ws", "task", "sync", "--id", "WS1", "--format", "json"})
	if code != exitError || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	response := decodeJSONResponse(t, out.String())
	if response.OK || response.Error.Code != "conflict" {
		t.Fatalf("response = %#v", response)
	}
	sync := response.Result["sync"].(map[string]any)
	if sync["state"] != "conflict" || len(sync["conflicts"].([]any)) != 1 {
		t.Fatalf("sync = %#v", sync)
	}
}

func TestCLI_WSTaskSync_AllAggregatesPartialFailure(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")
	seedWorkspaceMeta(t, env.Root, "active", "WS2")
	fake := &fakeCMUXTodoReconciler{run: func(request cmuxtodo.Request) (cmuxtodo.Result, error) {
		if request.WorkspaceID == "WS2" {
			return cmuxtodo.Result{WorkspaceID: request.WorkspaceID}, cmuxtodo.ErrConflict
		}
		return cmuxtodo.Result{WorkspaceID: request.WorkspaceID, Changed: true}, nil
	}}
	installFakeCMUXTodoReconciler(t, fake)

	var out bytes.Buffer
	var stderr bytes.Buffer
	code := New(&out, &stderr).Run([]string{"ws", "task", "sync", "--all", "--dry-run", "--format", "json"})
	if code != exitError || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	response := decodeJSONResponse(t, out.String())
	if response.OK || response.Error.Code != "partial_failure" || response.Result["count"] != float64(2) || response.Result["failed"] != float64(1) || response.Result["succeeded"] != float64(1) {
		t.Fatalf("response = %#v", response)
	}
	if len(fake.requests) != 2 || !fake.requests[0].DryRun || !fake.requests[1].DryRun {
		t.Fatalf("requests = %#v", fake.requests)
	}
}

func TestCLI_WSTaskSync_CMUXCurrentRootAnchorTargetsAllWorkspaces(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	seedWorkspaceMeta(t, env.Root, "active", "WS1")
	seedWorkspaceMeta(t, env.Root, "active", "WS2")
	store := cmuxmap.NewStore(env.Root)
	if err := store.Save(cmuxmap.File{
		Version: cmuxmap.CurrentVersion,
		RootGroup: &cmuxmap.GroupMapping{
			CMUXWindowID:      "window-id",
			CMUXGroupID:       "group-id",
			AnchorWorkspaceID: "anchor-id",
		},
		// Deliberately omit the legacy KRA_ROOT workspace entry. RootGroup is
		// authoritative for native Workspace Group anchor routing.
		Workspaces: map[string]cmuxmap.WorkspaceMapping{},
	}); err != nil {
		t.Fatalf("save cmux mapping: %v", err)
	}
	t.Setenv("CMUX_WORKSPACE_ID", "anchor-id")
	originalGetwd := getwdForCMUXTodoSyncTest(t)
	if err := os.Chdir(env.Root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalGetwd) })

	fake := &fakeCMUXTodoReconciler{}
	installFakeCMUXTodoReconciler(t, fake)
	var out bytes.Buffer
	var stderr bytes.Buffer
	code := New(&out, &stderr).Run([]string{"ws", "task", "sync", "--cmux-current", "--dry-run", "--format", "json"})
	if code != exitOK || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if len(fake.requests) != 2 || fake.requests[0].WorkspaceID != "WS1" || fake.requests[1].WorkspaceID != "WS2" {
		t.Fatalf("requests = %#v", fake.requests)
	}
	for _, request := range fake.requests {
		if request.Root != env.Root || request.Scope != "active" || !request.DryRun {
			t.Fatalf("request = %#v", request)
		}
	}
}

func getwdForCMUXTodoSyncTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}

func TestCLI_TaskSyncTargetsRootGroupAnchor(t *testing.T) {
	env := testutil.NewEnv(t)
	initAndConfigureRootRepo(t, env.Root)
	fake := &fakeCMUXTodoReconciler{}
	installFakeCMUXTodoReconciler(t, fake)

	var out bytes.Buffer
	var stderr bytes.Buffer
	code := New(&out, &stderr).Run([]string{"task", "sync", "--format", "json"})
	if code != exitOK || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if len(fake.requests) != 1 {
		t.Fatalf("requests = %#v", fake.requests)
	}
	request := fake.requests[0]
	if request.Root != env.Root || request.WorkspaceID != cmuxtodo.RootWorkspaceID || request.Scope != "root" || request.Target != cmuxtodo.TargetRoot {
		t.Fatalf("request = %#v", request)
	}
	response := decodeJSONResponse(t, out.String())
	if !response.OK || response.Action != "task.sync" || response.WorkspaceID != "" {
		t.Fatalf("response = %#v", response)
	}
}

func TestParseWSTaskSyncOptionsRejectsUnsafeInputs(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--all", "--id", "WS1"},
		{"--id", "WS1", "--prefer", "newest"},
		{"--id", "WS1", "--format", "yaml"},
	} {
		if _, err := parseWSTaskSyncOptions(args); err == nil {
			t.Fatalf("parseWSTaskSyncOptions(%q) error = nil", args)
		}
	}
	if _, err := parseTaskSyncOptions([]string{"--prefer", "newest"}); err == nil {
		t.Fatal("parseTaskSyncOptions invalid preference error = nil")
	}
}

func TestCMUXTodoErrorCodeRecovery(t *testing.T) {
	for _, cause := range []error{errors.New("save failed"), cmuxtodo.ErrConflict, cmuxtodo.ErrConcurrentTodo} {
		err := &cmuxtodo.RecoveryError{Cause: cause, Repair: []error{errors.New("undo failed")}}
		if got := cmuxTodoErrorCode(err); got != "recovery_failed" {
			t.Fatalf("cause=%v code=%q", cause, got)
		}
	}
}
