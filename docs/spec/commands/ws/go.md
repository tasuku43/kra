---
title: "`gionx ws go`"
status: implemented
pending:
  - ws_list_select_entrypoint_doc_sync
---

# `gionx ws go [--archived] [--id <id>] [--ui] [<id>]`

## Purpose

Jump to a workspace directory as a "start work" action.

## Inputs

- `<id>` (required): workspace id for direct mode
- `--id <id>` (optional): explicit workspace id flag (cannot be combined with positional `<id>`)
- `--archived` (optional): target archived workspaces instead of active workspaces
- `--ui` (optional): print human-readable `Result:` section

## Behavior

### Mode selection

- This command is explicit-id mode only (`--id` or positional `<id>`).
- For interactive selection, use `gionx ws list --select`.

### Target path

- active target: `GIONX_ROOT/workspaces/<id>/`
- archived target (with `--archived`): `GIONX_ROOT/archive/<id>/`

### UX detail

- In standard mode, do not print shell command snippets to stdout.
- In `--ui` mode, print `Result:` and destination path (human-readable).
- Non-TTY constraints for selection are handled by `ws list --select`.

### Shell integration

- `gionx` cannot mutate the parent shell cwd directly.
- For practical navigation, shell wrappers execute action-file entries after command completion.
- Shell-wide wrapper integration is provided via `gionx shell init <shell>`.
- Planned extension:
  - when routed from unified launcher flow, `go` semantics remain identical to direct `ws go`.
  - shell integration uses post-exec action protocol (`GIONX_SHELL_ACTION_FILE`) for launcher-routed go actions.

## Errors

- no matching workspace in selected scope
- invalid mixed selection (more than one selected in selector mode)
- target directory does not exist
- non-TTY invocation in selector mode

## Logical work-state behavior

- Candidate discovery for `ws go` should align with FS-first workspace discovery and `.gionx.meta.json`.
- `active` scope candidate rows should expose the same logical work-state semantics as `ws list`
  (`todo` / `in-progress`, runtime-derived).
- No logical work-state persistence is allowed.
