---
title: "`kra ws list`"
status: implemented
---

# `kra ws list [--archived] [--tree] [--format human|tsv|json]`

Alias:
- `kra ws ls` (same semantics as `ws list`)

## Purpose

List workspaces with status and summary fields, similar in spirit to `gion manifest ls`.

`ws list` is a read-only listing command.
`ws list` is read-only. Interactive selection is provided by each workspace action command with `--select`.

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
- JSON envelope follows `docs/spec/concepts/output-contract.md`:
  - action: `ws.list`
  - result: `scope`, `tree`, `items[]`

## Display fields (MVP)

- `id`
- `status` (`active`/`archived`)
- `updated_at`
- `repo_count`
- `title` (stored as `title` for compatibility)

## Behavior (MVP)

- Filesystem metadata (`.kra.meta.json`) is the primary source of desired/current state.
- Directory existence under `KRA_ROOT/workspaces/` and `KRA_ROOT/archive/` is treated as physical truth.
- Logical work-state derivation (`active` scope):
  - runtime baseline file: `.kra/state/workspace-baselines/<id>.json`
  - repo signals under `repos/**`:
    - `dirty` -> `in-progress`
    - `baseline_head..HEAD` delta -> `in-progress`
  - non-repo FS signals:
    - compare current file hash map with baseline `fs` map (exclude `repos/**`, `.kra.meta.json`)
  - if any signal differs, classify as `in-progress`; otherwise `todo`
  - once a workspace is derived as `in-progress`, cache it in `.kra/state/workspace-workstate.json`
    and keep monotonic `todo -> in-progress` semantics.

### Drift repair (MVP)

- If a repo appears in metadata but its worktree is missing on disk:
  - mark/reconcile drift in index data without mutating canonical metadata unexpectedly
- If a workspace directory exists on disk but index/cached record is missing:
  - import it using directory and metadata signals.
