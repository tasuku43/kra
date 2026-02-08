---
title: "`gionx ws --act go`"
status: implemented
---

# `gionx ws --act go [--archived] [--id <id>] [--ui] [--format human|json] [<id>]`

## Purpose

Jump to a workspace directory as a "start work" action.

## Inputs

- `<id>` (required): workspace id for direct mode
- `--id <id>` (optional): explicit workspace id flag (cannot be combined with positional `<id>`)
- `--archived` (optional): target archived workspaces instead of active workspaces
- `--ui` (optional): print human-readable `Result:` section

## Behavior

### Mode selection

- Action is routed via `ws --act go`.
- This command path is explicit-id mode only (`--id` or positional `<id>`).
- For interactive selection, use `gionx ws select --act go`.

### Target path

- active target: `GIONX_ROOT/workspaces/<id>/`
- archived target (with `--archived`): `GIONX_ROOT/archive/<id>/`

### UX detail

- In standard mode, do not print shell command snippets to stdout.
- In `--ui` mode, print `Result:` and destination path (human-readable).
- Non-TTY constraints for selection are handled by `ws select`.

### Shell integration

- `gionx` cannot mutate the parent shell cwd directly.
- For practical navigation, shell wrappers execute action-file entries after command completion.
- Shell-wide wrapper integration is provided via `gionx shell init <shell>`.
- Planned extension:
  - when routed from unified launcher flow, `go` semantics remain identical to direct `ws --act go`.
  - shell integration uses post-exec action protocol (`GIONX_SHELL_ACTION_FILE`) for launcher-routed go actions.

## Errors

- no matching workspace in selected scope
- invalid mixed selection (more than one selected in selector mode)
- target directory does not exist
- non-TTY invocation in selector mode

## Non-interactive JSON contract

- `--format json` enables machine-readable output.
- In JSON mode, success and failure responses share envelope fields:
  - `ok` (bool)
  - `action` (`go`)
  - `workspace_id`
  - `result` (success only)
  - `error.code` and `error.message` (failure only)
- Failure keeps non-zero exit code (usage/internal/business failure mapping is preserved).

## Logical work-state behavior

- Candidate discovery for `ws go` should align with FS-first workspace discovery and `.gionx.meta.json`.
- `active` scope candidate rows should expose the same logical work-state semantics as `ws list`
  (`todo` / `in-progress`, runtime-derived).
- No logical work-state persistence is allowed.
