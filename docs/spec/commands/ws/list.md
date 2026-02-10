---
title: "`gionx ws list`"
status: implemented
---

# `gionx ws list [--archived] [--tree] [--format human|tsv]`

Alias:
- `gionx ws ls` (same semantics as `ws list`)

## Purpose

List workspaces with status and summary fields, similar in spirit to `gion manifest ls`.

`ws list` is a read-only listing command.
`ws list` is read-only. Interactive selection entrypoint is `ws select`.

## Role boundary

- `ws select` handles interactive workspace/action selection.
- Operation commands run with explicit `<id>` after selection.

## Default display

- One row per workspace (summary view) using selector-parity visual hierarchy.
- Row content is bullet-list style and summary-first:
  - `id`
  - `title` (stored as `title` for compatibility)
- Canonical row shape:
  - `• WS-101: login flow`
- Summary row order is fixed as `ID | title`.
- Ellipsis policy:
  - only `title` is truncated with `…` when terminal width is tight.
  - row output width must not exceed the selected terminal width.
- Header shows scope only:
  - default: `Workspaces(active):`
  - `--archived`: `Workspaces(archived):`
- `Workspaces(...)` section follows shared section atom contract and ends with exactly one trailing blank line.
- `status` is represented by scope, not repeated per row.
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

- `ws list` is specified as human-oriented task-list output in this UX phase.
- Machine-readable output is explicit via `--format tsv`.
- Default format is `human`.
- TSV columns are fixed as:
  - `id`
  - `status`
  - `updated_at`
  - `repo_count`
  - `title`

## Display fields (MVP)

- `id`
- `status` (`active`/`archived`)
- `updated_at`
- `repo_count`
- `title` (stored as `title` for compatibility)

## Behavior (MVP)

- Filesystem metadata (`.gionx.meta.json`) is the primary source of desired/current state.
- Directory existence under `GIONX_ROOT/workspaces/` and `GIONX_ROOT/archive/` is treated as physical truth.

### Drift repair (MVP)

- If a repo appears in metadata but its worktree is missing on disk:
  - mark/reconcile drift in index data without mutating canonical metadata unexpectedly
- If a workspace directory exists on disk but index/cached record is missing:
  - import it using directory and metadata signals.
