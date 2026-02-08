---
title: "`gionx ws go`"
status: implemented
pending:
  - ws_select_launcher_integration
---

# `gionx ws go [--archived] [--select] [--ui] [--emit-cd] [<id>]`

## Purpose

Jump to a workspace directory as a "start work" action.

## Inputs

- `<id>` (optional): workspace id for direct mode
- `--archived` (optional): target archived workspaces instead of active workspaces
- `--select` (optional): force workspace selection UI before command execution
- `--ui` (optional): print human-readable `Result:` section
- `--emit-cd` (optional): backward-compatible alias of default shell snippet output

## Behavior

### Mode selection

- If `<id>` is provided:
  - resolve the target directly
- If `--select` is provided:
  - launch shared selector UI first
  - selector scope is `active` by default, `archived` when `--archived` is set
  - selected workspace id is then used as direct target
  - `--select` cannot be combined with `<id>`
- If `<id>` is omitted:
  - launch shared selector UI (`commands/ws/selector.md`)
  - default scope is `active`; use `--archived` to switch scope
  - selection cardinality is single-only
  - UI is single-select mode (no checkbox markers / no `selected: n/m` footer)

### Target path

- active target: `GIONX_ROOT/workspaces/<id>/`
- archived target (with `--archived`): `GIONX_ROOT/archive/<id>/`

### UX detail

- In standard mode, print only shell snippet (`cd '<path>'`) to stdout.
- In `--ui` mode, print `Result:` and destination path (human-readable).
- `--emit-cd` keeps backward compatibility and behaves the same as standard mode.
- In selector mode, non-TTY invocation must fail (no fallback).

### Shell integration

- `gionx` cannot mutate the parent shell cwd directly.
- For practical navigation, default output is shell-evaluable.
- Expected usage style: `eval "$(gionx ws go)"`.
- Shell-wide wrapper integration is provided via `gionx shell init <shell>`.
- Planned extension:
  - when routed from unified launcher flow, `go` semantics remain identical to direct `ws go`.
  - shell integration uses post-exec action protocol (`GIONX_SHELL_ACTION_FILE`) for launcher-routed go actions
    while preserving user-visible behavior.

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
