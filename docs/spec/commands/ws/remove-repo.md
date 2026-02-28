---
title: "`kra ws remove-repo`"
status: implemented
---

# `kra ws remove-repo [--id <workspace-id>] [<workspace-id>] [--format human|json]`

## Purpose

Remove repository bindings from a workspace and delete corresponding workspace worktrees.

This command is the operational counterpart of `kra ws add-repo`.

## Inputs

- `workspace-id` (optional): existing active workspace ID
- `--id <workspace-id>` (optional): explicit workspace ID flag
  - cannot be combined with positional `workspace-id`
  - if omitted, current working directory must be under `KRA_ROOT/workspaces/<id>/`
  - otherwise the command fails fast
- interactive selection is handled by `kra ws remove-repo --select`.
- JSON mode (`--format json`) is non-interactive and accepts:
  - `--repo <repo-key>` (repeatable, required)
  - `--yes` (required)
  - `--force` (optional; bypass dirty/unpushed safety gate)

## Selection source

- Candidates are repos currently bound to the target workspace.
- Candidate rows must show `repo_key` (and optionally `alias` when needed for disambiguation).
- Candidate ordering:
  1. `repo_key` ascending
  2. `alias` ascending
- Candidate filter target is `repo_key` and `alias`.

## Behavior

1. Select repos to remove (multi-select)
  - section heading: `Repos(workspace):`
  - TTY: shared inline selector (`space` toggle, `enter` confirm, text filter input)
  - non-TTY: fail fast (interactive selection requires TTY)

2. Preflight safety checks
  - verify each selected binding exists in target workspace
  - verify selected worktree path is under `workspaces/<id>/repos/<alias>`
  - dirty/unpushed worktree policy:
    - human mode:
      - `Plan:` must include per-repo `risk` / `sync` / `files` details before apply
      - if any selected worktree has non-clean risk, apply confirmation requires explicit `yes`
    - JSON mode:
      - non-clean risk requires `--force`; otherwise fail with `error.code=conflict`

3. Plan and confirmation
  - print `Plan:` with selected repo list and per-repo details:
    - `risk:`
    - `sync: upstream=<...> ahead=<n> behind=<n>`
    - `files:` (`git status --short` style lines) when there are changes
  - final prompt:
    - default: `apply this plan? [Enter=yes / n=no]: `
    - with non-clean risk: require explicit `yes` confirmation

4. Apply (all-or-nothing)
  - remove workspace repo bindings from state/index
  - delete selected worktree directories under `workspaces/<id>/repos/`
  - if current process cwd is inside target `workspaces/<id>/`, shift cwd to `workspaces/<id>/` before destructive removal
  - when cwd shift happened, emit shell action `cd <workspaces/<id>>` after successful apply
  - keep repo pool entries and bare repos untouched
  - on failure, abort with error

5. FS metadata behavior
  - on success, remove corresponding entries from `workspaces/<id>/.kra.meta.json` `repos_restore`
  - metadata update must be atomic (`temp + rename`)

6. Result
  - print `Result:`
  - summary: `Removed <n> / <m>`
  - per repo success lines: `✔ <repo-key>`
  - visual emphasis policy:
    - `Removed <n> / <m>` stays primary
    - success color is used on `✔` only
    - repo key text stays primary

## Non-interactive JSON contract

- `--format json` enables machine-readable output.
- In JSON mode, command must not prompt.
- Missing required inputs (`--repo`, `--yes`) must fail with `error.code=invalid_argument` and non-zero exit.
- Error payload must clearly indicate whether failure happened at preflight or apply stage.

## Scope and launcher integration

- Action is valid only for active workspaces.
- `kra ws remove-repo --select` is supported and skips action menu.
- Action menu (`kra ws` in active workspace context) should include `remove-repo` near `add-repo`.
