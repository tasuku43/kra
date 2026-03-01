---
title: "Integration testing (kra)"
status: implemented
---

# Integration testing (kra)

## Purpose

Define an expanded integration test plan for `kra` commands through the archive lifecycle (`MVP-042`).

This ticket focuses on "drift" and partial failure scenarios across:
- filesystem under `KRA_ROOT`
- root index cache
- git repo pool + worktrees

## Scope (initial)

Commands implemented through `MVP-042`:
- `kra init`
- `kra template create`
- `kra template remove`
- `kra template validate`
- `kra ws create`
- `kra ws add-repo`
- `kra ws close`
- `kra ws reopen`
- `kra ws purge`

## Done definition

- Add tests that run the CLI (`CLI.Run`) and verify side effects in both FS and index.
- Include at least some non-happy-path coverage for:
  - drift between index and filesystem
  - git worktree constraints / repo pool issues (as applicable)
- Keep tests isolated:
  - temp `KRA_ROOT` per test
  - isolated metadata/index files per test
  - avoid using the developer’s global git config/state

## Coverage map (spec -> tests)

This section maps the spec scenarios to concrete CLI integration tests.

### CLI UI regression matrix (cross-command)

- command-level human UI golden snapshots:
  - `internal/cli/ui_e2e_golden_test.go`: `TestGolden_UIE2E_CoreWorkspaceWorkflow`
- component-level UI golden snapshots:
  - `internal/cli/golden_ui_test.go`: selector/add-repo/ws-import-jira/ws-list render snapshots

### `init`

- settings drift by `root_path`:
  - `internal/cli/drift_test.go`: `TestCLI_Init_SettingsDrift_ErrorsOnDifferentRoot`
- settings drift by `repo_pool_path`:
  - `internal/cli/drift_test.go`: `TestCLI_Init_SettingsDrift_ErrorsOnDifferentRepoPool`

### `ws create`

- invalid root:
  - `internal/cli/drift_test.go`: `TestCLI_WS_Create_InvalidRoot_Errors`
- filesystem collision does not create index row:
  - `internal/cli/drift_test.go`: `TestCLI_WS_Create_FilesystemCollision_DoesNotInsertDBRow`
- recreate after `purged` increments generation:
  - `internal/cli/drift_test.go`: `TestCLI_WS_Create_Purged_AllowsNewGeneration`
- Jira single-issue create success:
  - `internal/cli/ws_create_jira_test.go`: `TestCLI_WS_Create_Jira_Success`
- Jira env missing fail-fast:
  - `internal/cli/ws_create_jira_test.go`: `TestCLI_WS_Create_Jira_MissingEnv_FailsFastWithoutWorkspaceCreation`
- Jira URL resolve failure (404) fail-fast:
  - `internal/cli/ws_create_jira_test.go`: `TestCLI_WS_Create_Jira_404_FailsFastWithoutStateMutation`
- Jira strict flag conflict (`--jira` with `--id/--title`) usage error:
  - `internal/cli/ws_create_jira_test.go`: `TestCLI_WS_Create_Jira_ConflictWithIDOrTitle_FailsUsage`
- default template missing should fail without workspace dir mutation:
  - `internal/cli/ws_create_template_test.go`: `TestCLI_WSCreate_DefaultTemplateMissing_Fails`
- reserved-path template should fail before workspace dir creation:
  - `internal/cli/ws_create_template_test.go`: `TestCLI_WSCreate_TemplateReservedPath_FailsBeforeCreate`
- `--template` should apply selected template content:
  - `internal/cli/ws_create_template_test.go`: `TestCLI_WSCreate_TemplateOption_CopiesSelectedTemplate`

### `template create`

- `--name` creates scaffold under `templates/<name>/`:
  - `internal/cli/template_create_test.go`: `TestCLI_TemplateCreate_NameFlag_CreatesScaffold`
- omitted name prompts and creates scaffold:
  - `internal/cli/template_create_test.go`: `TestCLI_TemplateCreate_PromptWhenNameOmitted`
- invalid template name returns usage:
  - `internal/cli/template_create_test.go`: `TestCLI_TemplateCreate_InvalidName_ReturnsUsage`
- existing template name fails:
  - `internal/cli/template_create_test.go`: `TestCLI_TemplateCreate_AlreadyExists_ReturnsError`
- staged path outside allowlist aborts commit:
  - `internal/cli/template_create_test.go`: `TestCLI_TemplateCreate_AbortWhenStagedOutsideAllowlist`

### `template remove`

- `--name` removes one template:
  - `internal/cli/template_remove_test.go`: `TestCLI_TemplateRemove_NameFlag_RemovesTemplate`
- `rm` alias works:
  - `internal/cli/template_remove_test.go`: `TestCLI_TemplateRemove_RMAlias_Works`
- omitted name prompts and removes:
  - `internal/cli/template_remove_test.go`: `TestCLI_TemplateRemove_PromptWhenNameOmitted`
- missing template returns structured error:
  - `internal/cli/template_remove_test.go`: `TestCLI_TemplateRemove_Missing_ReturnsStructuredError`
- staged path outside allowlist aborts commit:
  - `internal/cli/template_remove_test.go`: `TestCLI_TemplateRemove_AbortWhenStagedOutsideAllowlist`

### `template validate`

- validate all templates and aggregate violations:
  - `internal/cli/template_validate_test.go`: `TestCLI_TemplateValidate_AllTemplates_CollectsViolations`
- single template validation success:
  - `internal/cli/template_validate_test.go`: `TestCLI_TemplateValidate_Name_Success`
- missing named template shows available list:
  - `internal/cli/template_validate_test.go`: `TestCLI_TemplateValidate_Name_NotFound_ShowsAvailable`

### `ws import jira`

- plan-only with skip/fail classification:
  - `internal/cli/ws_import_jira_test.go`: `TestCLI_WS_Import_Jira_NoPromptWithoutApply_PrintsPlanWithSkipAndFail`
- JSON plan-only contract (`stdout` JSON only + reason/action classification):
  - `internal/cli/ws_import_jira_test.go`: `TestCLI_WS_Import_Jira_JSON_NoPromptWithoutApply_Contract`
- apply mode creates workspace from Jira issue list:
  - `internal/cli/ws_import_jira_test.go`: `TestCLI_WS_Import_Jira_NoPromptApply_CreatesWorkspace`
- JSON apply mode create failure classification:
  - `internal/cli/ws_import_jira_test.go`: `TestCLI_WS_Import_Jira_JSON_NoPromptApply_CreateFailureReason`
- prompt decline still respects exit-code contract (`failed > 0` => non-zero):
  - `internal/cli/ws_import_jira_test.go`: `TestCLI_WS_Import_Jira_PromptDecline_WithFailedPlan_ReturnsError`
- JSON prompt contract (prompts on `stderr`, JSON on `stdout`):
  - `internal/cli/ws_import_jira_test.go`: `TestCLI_WS_Import_Jira_JSON_Prompt_PrintsPromptToStderr`
- human plan layout contract (bulleted sections/tree rows):
  - `internal/cli/ws_import_jira_test.go`: `TestCLI_WS_Import_Jira_NoPromptWithoutApply_UsesBulletedPlanLayout`
- apply prompt phrasing contract:
  - `internal/cli/ws_import_jira_test.go`: `TestRenderWSImportJiraApplyPrompt_UsesBulletedPlanAlignment`
- prompt apply result contract (result summary + completion message):
  - `internal/cli/ws_import_jira_test.go`: `TestCLI_WS_Import_Jira_PromptAccept_PrintsResultSummary`
- `--sprint` without value interactive selection:
  - `internal/cli/ws_import_jira_test.go`: `TestCLI_WS_Import_Jira_SprintNoValue_PromptSelectsFromSpaceSprintList`
- sprint selector candidate filtering (`active`/`future` only):
  - `internal/cli/ws_import_jira_test.go`: `TestCLI_WS_Import_Jira_SprintNoValue_ShowsOnlyActiveFuture`
- numeric sprint ID resolution via JQL direct mode:
  - `internal/cli/ws_import_jira_test.go`: `TestCLI_WS_Import_Jira_SprintNumericID_UsesJQLDirect`

### `ws list`

- import workspace dir drift into index:
  - `internal/cli/ws_list_test.go`: `TestCLI_WS_List_ImportsWorkspaceDirAndPrintsIt`
- mark missing active repo worktree:
  - `internal/cli/ws_list_test.go`: `TestCLI_WS_List_MarksMissingRepoWorktree`
- do not mark missing for archived workspace:
  - `internal/cli/integration_comprehensive_test.go`: `TestCLI_WS_List_ArchivedWorkspace_DoesNotMarkRepoMissing`

### `ws add-repo`

- happy path (worktree + index):
  - `internal/cli/cli_test.go`: `TestCLI_WS_AddRepo_CreatesWorktreeAndRecordsState`
- repo pool corrupted:
  - `internal/cli/integration_lifecycle_test.go`: `TestCLI_WS_AddRepo_CorruptedRepoPool_FailsWithoutStateMutation`
- base_ref missing on remote:
  - `internal/cli/integration_comprehensive_test.go`: `TestCLI_WS_AddRepo_BaseRefNotFound_FailsWithoutMutatingState`

### `ws close`

- full archive lifecycle side effects (FS + index + git):
  - `internal/cli/ws_close_test.go`: `TestCLI_WS_Close_ArchivesWorkspaceRemovesWorktreesCommitsAndUpdatesDB`
- dirty worktree risk prompt and abort:
  - `internal/cli/ws_close_test.go`: `TestCLI_WS_Close_DirtyRepo_PromptsAndCanAbort`
- staged changes guard:
  - `internal/cli/integration_comprehensive_test.go`: `TestCLI_WS_Close_WithStagedChanges_FailsBeforeMutatingWorkspace`
- index drift (workspace repo binding metadata mismatch):
  - `internal/cli/integration_lifecycle_test.go`: `TestCLI_WS_Close_RepoMetadataDrift_FailsWithoutArchiving`

### `ws reopen`

- full reopen lifecycle side effects (FS + index + git):
  - `internal/cli/ws_reopen_test.go`: `TestCLI_WS_Reopen_RestoresWorkspaceRecreatesWorktreesCommitsAndUpdatesDB`
- branch checked out elsewhere:
  - `internal/cli/ws_reopen_test.go`: `TestCLI_WS_Reopen_ErrorsWhenBranchCheckedOutElsewhere`
- staged changes guard:
  - `internal/cli/integration_comprehensive_test.go`: `TestCLI_WS_Reopen_WithStagedChanges_FailsBeforeMutatingWorkspace`

### `ws purge`

- archived purge lifecycle side effects (FS + index + git):
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
- filesystem collision should not insert index rows
- allow re-create after `purged` (generation increments)
- `ws create --jira <ticket-url>` should resolve `id/title` from Jira issue
- missing Jira env vars should fail without mutating FS/state
- Jira issue fetch errors (e.g. 404/auth) should fail without mutating FS/state
- `--jira` + `--id/--title` should fail with usage
- missing `default` template should fail
- template validation violations should fail before workspace creation

### `template validate`

- create scaffold then validate should pass
- remove template then validate should fail for missing target when `--name` used
- validate all templates under root
- `--name` validates one template
- one or more violations should return non-zero

### `ws add-repo`

- repo pool missing/corrupted behavior
- worktree already checked out elsewhere (git worktree constraint)
- fetch failure (credentials/network) should have defined behavior (error vs continue)

### Archive lifecycle

- `ws close` removes worktrees and archives atomically (FS + index + git)
- `ws reopen` restores archived workspace and recreates worktrees (FS + index + git)
- `ws purge` removes workspace snapshot + files with confirmations (FS + index)

## Non-interactive contract tests (`UX-WS-026`)

- JSON success envelope for:
  - `ws close --format json`
- JSON validation failures:
  - `ws add-repo --format json` without required `--repo`/`--yes`
- Exit code policy:
  - JSON mode still returns non-zero exit on failures.
