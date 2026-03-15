---
title: "`kra ws doc open`"
status: implemented
---

# `kra ws doc open [--id <workspace-id> | --current | --select] [<path>] [--surface <id|ref|index>] [--no-focus] [--format human|json]`

## Purpose

Open one Markdown document for a workspace in the `cmux` Markdown viewer while keeping viewer tabs collected into one docs pane slot per workspace.

## Target resolution

- Resolve workspace from exactly one of:
  - `--id <workspace-id>`
  - `--current`
  - `--select`
- Human mode may infer current workspace from `cwd` when no target flag is provided and `cwd` is under:
  - `workspaces/<id>/...`
  - `archive/<id>/...`
- JSON mode must resolve one workspace deterministically; interactive selection is not allowed.

## Path resolution

- `<path>` is optional.
- When omitted:
  - search from workspace root.
  - prompt for one Markdown file in human mode.
- When `<path>` is a directory:
  - search only under that directory.
  - prompt for one Markdown file in human mode.
- When `<path>` is a file:
  - open that file directly.
- Reject path when:
  - it resolves outside the target workspace
  - it resolves under `repos/`
  - it is not a Markdown file (`.md`, `.markdown`)

## cmux routing

- Require `cmux` capabilities:
  - `markdown.open`
  - `workspace.create`
  - `workspace.rename`
  - `pane.create`
  - `pane.list`
  - `pane.surfaces`
  - `surface.move`
  - `surface.close`
- Resolve target `cmux` workspace from existing `kra` workspace mapping.
- If no mapped `cmux` workspace exists, fail with `cmux_workspace_not_found`.

## Stage workspace and docs pane slot

- Use the `KRA_ROOT` `cmux` workspace as the Markdown staging workspace.
- Persist best-effort slot state under:
  - `.kra/state/cmux-docs.json`
- Stored state is advisory only.
- For the staging workspace:
  - prefer stored pane/surface when still reachable inside the current `KRA_ROOT` workspace
  - otherwise inspect the current `KRA_ROOT` workspace and pick one existing surface
  - if the `KRA_ROOT` workspace mapping is missing or stale, ensure it using the same runtime creation logic as `kra root open`, but do not select it
- Maintain one logical docs pane slot per target workspace:
  - prefer stored pane when still reachable
  - otherwise rediscover by scanning panes for `docs:*` viewer tabs
  - otherwise create a new right-side docs pane in the target workspace

## Viewer open flow

- Open Markdown via `cmux markdown open <abs-path> --workspace <stage-workspace> --surface <stage-surface>`.
- Move the created viewer surface into the target workspace docs pane slot.
- Append after the current last docs viewer if one exists; otherwise append after the last surface in that pane.
- When a docs pane was newly created for this open:
  - close the initial bootstrap terminal surface after moving the viewer in
- Rename viewer tab to:
  - `docs:<basename>`

## Focus

- Default:
  - focus the newly opened viewer surface after move.
- With `--no-focus`:
  - keep current focus unchanged.

## Output

- Human:
  - print opened workspace id, file path, docs pane ref, and surface ref.
- JSON envelope:
  - `ok`
  - `action=ws.doc.open`
  - `workspace_id`
  - `result`:
    - `path`
    - `scope_path`
    - `cmux_workspace_id`
    - `docs_pane_ref`
    - `viewer_surface_ref`
    - `focused`

## Errors

- `not_found`
  - workspace missing
  - Markdown file missing
  - no Markdown candidates in requested scope
- `invalid_argument`
  - conflicting target flags
  - path escapes workspace
  - path points under `repos/`
  - non-Markdown file requested
- `non_interactive_selection_required`
  - picker-required case in JSON mode
- `cmux_workspace_not_found`
  - no mapped runtime workspace
- `cmux_capability_missing`
  - required `cmux` method missing
- `cmux_stage_workspace_failed`
  - root staging workspace could not be ensured or inspected
