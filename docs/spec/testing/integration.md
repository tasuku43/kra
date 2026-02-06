---
title: "Integration testing (gionx)"
status: implemented
---

# Integration testing (gionx)

## Purpose

Define an expanded integration test plan for `gionx` commands through the archive lifecycle (`MVP-042`).

This ticket focuses on "drift" and partial failure scenarios across:
- filesystem under `GIONX_ROOT`
- SQLite state store
- git repo pool + worktrees

## Scope (initial)

Commands implemented through `MVP-042`:
- `gionx init`
- `gionx ws create`
- `gionx ws add-repo`
- `gionx ws close`
- `gionx ws reopen`
- `gionx ws purge`

## Done definition

- Add tests that run the CLI (`CLI.Run`) and verify side effects in both FS and DB.
- Include at least some non-happy-path coverage for:
  - drift between DB and filesystem
  - git worktree constraints / repo pool issues (as applicable)
- Keep tests isolated:
  - temp `GIONX_ROOT` per test
  - isolated sqlite file per test
  - avoid using the developer’s global git config/state

## Coverage map (spec -> tests)

This section maps the spec scenarios to concrete CLI integration tests.

### `init`

- settings drift by `root_path`:
  - `internal/cli/drift_test.go`: `TestCLI_Init_SettingsDrift_ErrorsOnDifferentRoot`
- settings drift by `repo_pool_path`:
  - `internal/cli/drift_test.go`: `TestCLI_Init_SettingsDrift_ErrorsOnDifferentRepoPool`

### `ws create`

- invalid root:
  - `internal/cli/drift_test.go`: `TestCLI_WS_Create_InvalidRoot_Errors`
- filesystem collision does not create DB row:
  - `internal/cli/drift_test.go`: `TestCLI_WS_Create_FilesystemCollision_DoesNotInsertDBRow`
- recreate after `purged` increments generation:
  - `internal/cli/drift_test.go`: `TestCLI_WS_Create_Purged_AllowsNewGeneration`

### `ws list`

- import workspace dir drift into DB:
  - `internal/cli/ws_list_test.go`: `TestCLI_WS_List_ImportsWorkspaceDirAndPrintsIt`
- mark missing active repo worktree:
  - `internal/cli/ws_list_test.go`: `TestCLI_WS_List_MarksMissingRepoWorktree`
- do not mark missing for archived workspace:
  - `internal/cli/integration_comprehensive_test.go`: `TestCLI_WS_List_ArchivedWorkspace_DoesNotMarkRepoMissing`

### `ws add-repo`

- happy path (worktree + DB):
  - `internal/cli/cli_test.go`: `TestCLI_WS_AddRepo_CreatesWorktreeAndRecordsState`
- repo pool corrupted:
  - `internal/cli/integration_lifecycle_test.go`: `TestCLI_WS_AddRepo_CorruptedRepoPool_FailsWithoutStateMutation`
- base_ref missing on remote:
  - `internal/cli/integration_comprehensive_test.go`: `TestCLI_WS_AddRepo_BaseRefNotFound_FailsWithoutMutatingState`

### `ws close`

- full archive lifecycle side effects (FS + DB + git):
  - `internal/cli/ws_close_test.go`: `TestCLI_WS_Close_ArchivesWorkspaceRemovesWorktreesCommitsAndUpdatesDB`
- dirty worktree risk prompt and abort:
  - `internal/cli/ws_close_test.go`: `TestCLI_WS_Close_DirtyRepo_PromptsAndCanAbort`
- staged changes guard:
  - `internal/cli/integration_comprehensive_test.go`: `TestCLI_WS_Close_WithStagedChanges_FailsBeforeMutatingWorkspace`
- DB drift (`workspace_repos` references missing `repos` row):
  - `internal/cli/integration_lifecycle_test.go`: `TestCLI_WS_Close_RepoMetadataDrift_FailsWithoutArchiving`

### `ws reopen`

- full reopen lifecycle side effects (FS + DB + git):
  - `internal/cli/ws_reopen_test.go`: `TestCLI_WS_Reopen_RestoresWorkspaceRecreatesWorktreesCommitsAndUpdatesDB`
- branch checked out elsewhere:
  - `internal/cli/ws_reopen_test.go`: `TestCLI_WS_Reopen_ErrorsWhenBranchCheckedOutElsewhere`
- staged changes guard:
  - `internal/cli/integration_comprehensive_test.go`: `TestCLI_WS_Reopen_WithStagedChanges_FailsBeforeMutatingWorkspace`

### `ws purge`

- archived purge lifecycle side effects (FS + DB + git):
  - `internal/cli/ws_purge_test.go`: `TestCLI_WS_Purge_ArchivedWorkspace_DeletesPathsCommitsAndUpdatesDB`
- confirmation/force behavior:
  - `internal/cli/ws_purge_test.go`: `TestCLI_WS_Purge_NoPromptWithoutForce_Refuses`
  - `internal/cli/ws_purge_test.go`: `TestCLI_WS_Purge_NoPromptForce_ActiveWorkspace_Succeeds`
- risk prompt on active dirty repo:
  - `internal/cli/ws_purge_test.go`: `TestCLI_WS_Purge_ActiveDirtyRepo_AsksSecondConfirmationAndCanAbort`
- staged changes guard:
  - `internal/cli/integration_comprehensive_test.go`: `TestCLI_WS_Purge_WithStagedChanges_FailsBeforeDeletingWorkspace`

## Candidate scenarios (non-exhaustive)

### `init`

- settings already initialized with different root/pool should error (drift protection)

### `ws create`

- invalid root should error
- filesystem collision should not insert DB rows
- allow re-create after `purged` (generation increments)

### `ws add-repo`

- repo pool missing/corrupted behavior
- worktree already checked out elsewhere (git worktree constraint)
- fetch failure (credentials/network) should have defined behavior (error vs continue)

### Archive lifecycle

- `ws close` removes worktrees and archives atomically (FS + DB + git)
- `ws reopen` restores archived workspace and recreates worktrees (FS + DB + git)
- `ws purge` removes workspace snapshot + files with confirmations (FS + DB)
