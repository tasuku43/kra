---
title: "`kra ws list`"
status: proposed
---

# `kra ws list [--archived] [--tree] [--format human|tsv|json]`

Alias:
- `kra ws ls` (same semantics as `ws list`)

## Purpose

List workspaces with status and summary fields, similar in spirit to `gion manifest ls`.

`ws list` is a read-only listing command. Interactive selection is provided by each workspace action command
with `--select`.

## Role boundary

- Workspace action commands (`ws close`, `ws reopen`, etc.) handle interactive selection via `--select`.
- Operation commands run with explicit `<id>` after selection.

## Default display

- One row per workspace (summary view) using selector-parity visual hierarchy.
- Row content is marker-list style and summary-first:
  - `id`
  - `title` (stored as `title` for compatibility)
- Canonical row shape:
  - `• WS-101: login flow`
- Summary row order is fixed as `ID | title`.
- Marker is display-only and not used to distinguish work-state.
- Ellipsis policy:
  - only `title` is truncated with `…` when terminal width is tight.
  - row output width must not exceed the selected terminal width.
- Header shows scope only:
  - default: `Workspaces(active):`
  - `--archived`: `Workspaces(archived):`
- `Workspaces(...)` section follows shared section atom contract and ends with exactly one trailing blank line.
- `status` is represented by scope, not repeated per row.
- Active list sort order is fixed:
  - work-state priority (`in-progress` first)
  - then `updated_at` descending
  - then `id` ascending
- Archived list keeps existing `updated_at`/`id` order (no work-state priority).
- Summary output should follow the same shared row rendering semantics as selector flows
  (`commands/ws/selector.md`), while remaining non-interactive.
- Status label coloring must follow shared semantics from selector UI:
  - `active`: active accent color
  - `archived`: muted color (`text.muted`)
  - no-color terminals: plain text fallback
- Selector markers (`[ ]`, `[x]`) are not used in `ws list`.

## Expanded display

- `--tree` shows repo-level detail under each workspace row.
- Default output remains summary-first to keep task-list UX and scripting usage simple.
- Repo tree lines are supplemental information and should use muted/low-contrast styling consistent with
  `commands/ws/selector.md` visual rules.

## Machine-readable output policy

- default format is `human`.
- machine-readable output is available via `--format tsv` and `--format json`.
- TSV columns are fixed as:
  - `id`
  - `status`
  - `updated_at`
  - `repo_count`
  - `title`
- JSON envelope follows `docs/dev/spec/concepts/output-contract.md`:
  - action: `ws.list`
  - result: `scope`, `tree`, `items[]`

## Display fields (MVP)

- `id`
- `status` (`active`/`archived`)
- `updated_at`
- `repo_count`
- `title` (stored as `title` for compatibility)

## Behavior

- Filesystem metadata (`.kra.meta.json`) is the primary source of desired/current state.
- Directory existence under `KRA_ROOT/workspaces/` and `KRA_ROOT/archive/` is treated as physical truth.
- with `--debug`, emit phase timing entries to debug log for registry touch, row scan/build, per-workspace
  repo/work-state derivation, and render steps
- `ws list` is strictly read-only:
  - must not create or refresh `.kra.meta.json.baseline`
  - must not rewrite `.kra.meta.json.workspace.work_state`
  - must not migrate legacy files
- Logical work-state handling (`active` scope):
  - preferred source is `.kra.meta.json.workspace.work_state`
  - if stored state is missing/blank, command may derive a transient display value from existing
    `.kra.meta.json.baseline`
  - legacy `.kra/state/workspace-baselines/<id>.json` is not read by `ws list`
  - if safe derivation is not possible, use the closest non-mutating fallback for ordering and report degraded state
    in warnings
- Drift or metadata normalization is reported, not repaired:
  - broken repo links
  - missing baseline
  - missing work-state
  - legacy state files
  are `doctor` responsibilities

## JSON contract additions

- `result.warnings[]` may be returned when row data is degraded but listing can still succeed
- warnings must be stable, user-actionable strings rather than raw stack messages
