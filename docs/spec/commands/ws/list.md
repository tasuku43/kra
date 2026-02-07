---
title: "`gionx ws list`"
status: implemented
---

# `gionx ws list [--archived] [--tree] [--format human|tsv]`

## Purpose

List workspaces with status and risk, similar in spirit to `gion manifest ls`.

`ws list` is a read-only listing command. Interactive selection flows are specified in `selector.md` and
action commands (`ws close`, `ws go`, `ws reopen`, `ws purge`).

## Role boundary

- `ws list` only shows current state and exits (non-interactive).
- Action execution belongs to `ws close/go/reopen/purge` selector flows.

## Default display

- One row per workspace (summary view) using selector-parity visual hierarchy.
- Row content is single-line and summary-first:
  - `id`
  - `risk` (color-only indicator; no textual tag)
  - `repo_count`
  - `description`
- Summary row order is fixed as `ID | risk | repos | description`.
- Column alignment rules:
  - `ID`, `risk`, and `repos` columns use fixed widths computed from the visible row set.
  - `description` starts at a stable column for all rows.
- Ellipsis policy:
  - only `description` is truncated with `…` when terminal width is tight.
  - row output width must not exceed the selected terminal width.
- Header shows scope only:
  - default: `Workspaces(active):`
  - `--archived`: `Workspaces(archived):`
- `status` is represented by scope, not repeated per row.
- Summary output should follow the same shared row rendering semantics as selector flows
  (`commands/ws/selector.md`), while remaining non-interactive.
- Status label coloring must follow shared semantics from selector UI:
  - `active`: active accent color
  - `archived`: muted color (`text.muted`)
  - no-color terminals: plain text fallback
- Selector markers (`[ ]`, `[x]`) are not used in `ws list`.
- Textual risk tags (`[clean]`, `[dirty]`, etc.) are not used in summary rows.

## Expanded display

- `--tree` shows repo-level detail under each workspace row.
- Default output remains summary-first to keep task-list UX and scripting usage simple.
- Repo tree lines are supplemental information and should use muted/low-contrast styling consistent with
  `commands/ws/selector.md` visual rules.

## Machine-readable output policy

- `ws list` is specified as human-oriented task-list output in this UX phase.
- Machine-readable output is explicit via `--format tsv`.
- Default format is `human`.

## Display fields (MVP)

- `id`
- `status` (`active`/`archived`)
- `updated_at`
- `repo_count`
- `risk` (live)
- `description`

## Behavior (MVP)

- The global state store is the primary source of desired state.
- The filesystem under `GIONX_ROOT/workspaces/` is treated as actual state.
- `risk` is reported as:
  - `clean` when `repo_count == 0`
  - `unknown` otherwise (repo-level risk computation is implemented later)

### Drift repair (MVP)

- If a repo is present in the state store but its worktree is missing on disk:
  - mark it as `missing` in the state store (do not delete)
- If a workspace directory exists on disk but is missing in the state store:
  - import it automatically using the directory name as the workspace ID
