---
title: "`kra ws --act close`"
status: implemented
---

# `kra ws --act close [--id <id>] [--force] [--format human|json] [--no-commit] [--commit] [<id>]`
# `kra ws --act close --dry-run --format json [--id <id>|<id>]`

## Purpose

Close a workspace:
- keep investigation notes and artifacts as an archive
- remove Git worktrees to keep the working area clean

This is the primary "task completed" flow in `kra`.

## Behavior (MVP)

### Preconditions

- Default mode (without `--no-commit`) requires `KRA_ROOT` to be a Git working tree (or `kra init` must have been run).
- Workspace `<id>` must exist as workspace metadata and be active.
- If current process cwd is inside the target workspace path (`workspaces/<id>/...`), the command must
  shift process cwd to `KRA_ROOT` before worktree removal starts.

### Steps

1) Inspect repo risk (live)

- For each repo under `KRA_ROOT/workspaces/<id>/repos/<alias>`:
  - compute risk similar to `gion` (dirty / unpushed / diverged / unknown / clean)
- If any repo is not clean, prompt for confirmation before continuing.

2) Commit pre-close snapshot (default; skipped by `--no-commit`)

- Commit message is fixed: `close-pre: <id>`
- Commit on the current branch.
- Stage only allowlisted paths:
  - `workspaces/<id>/`
- Preserve unrelated staged changes outside allowlist.
- `--commit` is accepted for backward compatibility and keeps default behavior.

3) Remove worktrees

- Remove each worktree under `workspaces/<id>/repos/<alias>`.
- Remove `workspaces/<id>/repos/` if it becomes empty.
- This step must run after process-cwd shift when current cwd is under target workspace.

4) Update workspace metadata/index

- Mark the workspace as `archived`.
- Update `updated_at`.

5) Archive the workspace contents

- Move `KRA_ROOT/workspaces/<id>/` to `KRA_ROOT/archive/<id>/` using an atomic rename.
- After this step, `KRA_ROOT/workspaces/<id>/` should not exist.

6) Commit the archive change (default; skipped by `--no-commit`)

- Commit message is fixed: `archive: <id>`
- Commit on the current branch.
- Stage only paths touched by this operation, at minimum:
  - `archive/<id>/`
  - removal of `workspaces/<id>/` (and any emptied parent folders as needed)
- After committing, store the commit SHA in metadata/index as `archived_commit_sha`.
- If post-archive commit fails, do not auto-rollback filesystem rename; keep archived state and return error.

7) Append an event

- Append `workspace_events(event_type='archived', workspace_id='<id>', at=...)` (this is the source of truth
  for the archive timestamp).

In default commit mode, unrelated changes must not be included in lifecycle commits.

### Shell synchronization for close

- When process cwd was shifted to `KRA_ROOT` due to target-workspace containment, successful close must emit
  shell action `cd '<KRA_ROOT>'` via action-file protocol.
- On failure, parent-shell cwd must not be modified.

## Notes

- `workspaces/**/repos/**` is ignored in `.gitignore`, but `ws close` must still delete worktrees
  (archives should not contain repos).

## Modes and selector behavior

- This command accepts explicit target by `--id` or positional `<id>`.
- If no id is provided, resolve from current path under `workspaces/<id>/...`.
- Interactive selection should use `kra ws select --act close`.
- Selector and follow-up output should use section headings:
  - `Workspaces(active):`
  - `Plan:`
  - `Result:`
- `ws close` user-facing wording uses `close` for actions/results:
  - selector footer action hint: `enter close`
  - risk confirmation prompt: `type yes to apply close on non-clean workspaces:`
  - result summary verb: `Closed n / m`
- Internal lifecycle/storage naming remains `archived` (status/event/commit message).
- Section spacing:
  - `Workspaces(active):` has one blank line after heading.
  - `Plan:` has no blank line after heading.
  - `Result:` has no blank line after heading.
- Section body indentation must use shared global UI indentation constants.

### Bulk close safety gate

- After selector confirmation, evaluate risk for all selected workspaces.
- `risky` is defined as `dirty` / `unpushed` / `diverged` (plus `unknown` as non-safe).
- If selected set is clean-only, proceed directly to close and print `Result:`.
- If any selected workspace is non-clean (`risky` or `unknown`), print `Plan:` section with risk details and
  require explicit `yes` confirmation before execution.
- If risk confirmation is declined/canceled, abort without side effects.
- Risk label semantics and severity follow `commands/ws/selector.md`.

### Non-interactive JSON safety gate

- `--format json` enables non-interactive execution contract.
- In JSON mode, cwd fallback is not allowed; target must be explicit (`--id` or positional id).
- If non-clean risk exists, execution requires `--force`; otherwise command returns non-zero with JSON error.
- `--dry-run --format json` must not mutate filesystem/git/state and should return executable/risk/planned-effects envelope.

### Commit strictness (non-repo files)

- Policy: non-`repos/` contents must be captured in the archive commit.
- Stage by allowlist only:
  - pre-close snapshot commit: `workspaces/<id>/`
  - archive commit: `workspaces/<id>/`, `archive/<id>/`
- Each lifecycle commit must be scoped by allowlist pathspec only so pre-existing staged changes outside the
  allowlist are preserved and must not be included.
- If `gitignore` causes any non-`repos/` files under selected workspace to be unstageable, abort.

## FS metadata behavior

- Before removing worktrees, refresh `workspaces/<id>/.kra.meta.json.repos_restore` from live repo state.
- `repos_restore` becomes the canonical reopen input after close.
- `workspace.status` in `.kra.meta.json` must be updated to `archived` before moving to `archive/<id>/`.
- Metadata updates must use atomic replace.
