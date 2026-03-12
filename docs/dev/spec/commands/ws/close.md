---
title: "`kra ws close`"
status: implemented
---

# `kra ws close [--id <id>] [--current] [--select] [--force] [--format human|json] [--no-commit] [--commit]`
# `kra ws close --dry-run --format json --id <id>`

## Purpose

Close a workspace:
- keep investigation notes and artifacts as an archive
- remove Git worktrees to keep the working area clean

This is the primary "task completed" flow in `kra`.
This flow must be recoverable when interrupted after irreversible filesystem phases.

## Behavior (MVP)

### Preconditions

- Default mode (without `--no-commit`) requires `KRA_ROOT` to be a Git working tree (or `kra init` must have been run).
- Workspace `<id>` must exist as workspace metadata and be active.
- If current process cwd is inside the target workspace path (`workspaces/<id>/...`), the command must
  shift process cwd to `KRA_ROOT` before worktree removal starts.
- If `KRA_ROOT/.kra/state/operations/ws-close/<id>.json` already exists and is unfinished, `ws close` must fail
  with recovery guidance instead of starting a second close flow.

### Steps

1) Inspect repo risk (live)

- For each repo under `KRA_ROOT/workspaces/<id>/repos/<alias>`:
  - compute risk similar to `gion` (dirty / unpushed / diverged / unknown / clean)
- If any repo is not clean, prompt for confirmation before continuing.

2) Inspect output coverage

- Derive output coverage from non-scaffolding files under `notes/` and `artifacts/`.
- Coverage states are:
  - `empty`
  - `notes-only`
  - `artifacts-only`
  - `documented`
- Scaffolding-only files do not count toward coverage:
  - `AGENTS.md`
  - `CLAUDE.md`
  - `.kra.meta.json`
  - `.claude/settings.local.json`
- `README.md` may be shown as a summary artifact but does not by itself satisfy `documented`.
- Root config key `workspace.close.empty_record_policy` controls handling when coverage is `empty`:
  - `warn`
  - `require-confirmation`

3) Initialize lifecycle journal

- Create/update `KRA_ROOT/.kra/state/operations/ws-close/<id>.json`.
- Journal schema is defined in `docs/dev/spec/concepts/lifecycle-journal.md`.
- Persist phase `risk_checked` after safety evaluation and before irreversible work begins.
- If pre-close snapshot fails before any irreversible workspace mutation, remove the just-created journal so
  a subsequent `ws close` retry is not blocked.

4) Commit pre-close snapshot (default; skipped by `--no-commit`)

- Commit message is fixed: `close-pre: <id>`
- Commit on the current branch.
- Stage only allowlisted paths:
  - `workspaces/<id>/`
- Preserve unrelated staged changes outside allowlist.
- `--commit` is accepted for backward compatibility and keeps default behavior.

5) Remove worktrees

- Remove each worktree under `workspaces/<id>/repos/<alias>`.
- Delete each corresponding local branch from the repo pool bare repository (`refs/heads/<branch>`).
  - If branch deletion cannot be completed (for example, branch already in use elsewhere), continue close as best-effort.
- Remove `workspaces/<id>/repos/` if it becomes empty.
- This step must run after process-cwd shift when current cwd is under target workspace.

6) Update workspace metadata/index

- Mark the workspace as `archived`.
- Update `updated_at`.

7) Archive the workspace contents

- Move `KRA_ROOT/workspaces/<id>/` to `KRA_ROOT/archive/<id>/` using an atomic rename.
- After this step, `KRA_ROOT/workspaces/<id>/` should not exist.

8) Commit the archive change (default; skipped by `--no-commit`)

- Commit message is fixed: `archive: <id>`
- Commit on the current branch.
- Stage only paths touched by this operation, at minimum:
  - `archive/<id>/`
  - removal of `workspaces/<id>/` (and any emptied parent folders as needed)
- After committing, store the commit SHA in metadata/index as `archived_commit_sha`.
- If post-archive commit fails, do not auto-rollback filesystem rename; keep archived state, preserve the
  lifecycle journal, and return error.

9) Append an event

- Append `workspace_events(event_type='archived', workspace_id='<id>', at=...)` (this is the source of truth
  for the archive timestamp).

10) Close mapped cmux workspace(s) (best-effort)

- If `.kra/state/cmux-workspaces.json` has mapping entries for `<id>`, call `cmux close-workspace --workspace <cmux-id>`
  for each mapped entry.
- `not_found`/`unknown workspace` is treated as already-closed and does not fail `ws close`.
- When all mapped entries are closed (or already absent), remove the workspace mapping entry from
  `.kra/state/cmux-workspaces.json`.
- cmux close failures must not rollback archive results.

11) Finalize lifecycle journal

- Mark the operation `completed`.
- Remove the journal file after successful completion.

In default commit mode, unrelated changes must not be included in lifecycle commits.

### Shell synchronization for close

- When process cwd was shifted to `KRA_ROOT` due to target-workspace containment, successful close must emit
  shell action `cd '<KRA_ROOT>'` via action-file protocol.
- On failure, parent-shell cwd must not be modified.

## Notes

- `workspaces/**/repos/**` is ignored in `.gitignore`, but `ws close` must still delete worktrees
  (archives should not contain repos).

## Modes and selector behavior

- This command accepts explicit target by `--id`.
- If no id is provided, resolve from current path under `workspaces/<id>/...` or use `--current`.
- Interactive selection should use `kra ws close --select`.
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
- `empty` output coverage is also a confirmation trigger when `workspace.close.empty_record_policy=require-confirmation`.
- If selected set is clean-only, proceed directly to close and print `Result:`.
- If any selected workspace is non-clean (`risky` or `unknown`), print `Plan:` section with risk details and
  require explicit `yes` confirmation before execution.
- If output coverage requires confirmation, include that reason in the same `Plan:` section.
- If risk confirmation is declined/canceled, abort without side effects.
- Risk label semantics and severity follow `commands/ws/selector.md`.

### Non-interactive JSON safety gate

- `--format json` enables non-interactive execution contract.
- In JSON mode, cwd fallback is not allowed; target must be explicit (`--id`).
- If non-clean risk exists, or empty coverage requires confirmation, execution requires `--force`;
  otherwise command returns non-zero with JSON error.
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
- On successful close, archived `.kra.meta.json` keeps the workspace baseline.
- Legacy baseline cleanup is not part of `ws close`; use `doctor --fix` for old state files.
- Read-only commands must not be relied on to finish or repair interrupted close state; recovery belongs to
  lifecycle journal handling and `doctor --fix`.
