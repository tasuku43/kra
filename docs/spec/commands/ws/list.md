---
title: "`gionx ws list`"
status: implemented
pending:
  - UX-WS-001-selector-foundation
  - UX-WS-001-list-role-clarification
---

# `gionx ws list`

## Purpose

List workspaces with status and risk, similar in spirit to `gion manifest ls`.

`ws list` is a read-only listing command. Interactive selection flows are specified in `selector.md` and
action commands (`ws close`, `ws go`, `ws reopen`, `ws purge`).

## Role boundary

- `ws list` only shows current state and exits (non-interactive).
- Action execution belongs to `ws close/go/reopen/purge` selector flows.

## Default display (planned refinement)

- One row per workspace (summary view), with at least:
  - `id`
  - `status`
  - `risk` (badge-like token, e.g. `[clean]`, `[dirty]`, `[unpushed]`, `[unknown]`)
  - `repo_count`
  - `updated_at`
  - `description`
- Summary output should follow the same shared row rendering semantics as selector flows
  (`commands/ws/selector.md`), while remaining non-interactive.

## Expanded display (planned refinement)

- `--tree` shows repo-level detail under each workspace row.
- Default output remains summary-first to keep task-list UX and scripting usage simple.
- Repo tree lines are supplemental information and should use muted/low-contrast styling consistent with
  `commands/ws/selector.md` visual rules.

## Machine-readable output policy

- `ws list` is specified as human-oriented task-list output in this UX phase.
- Do not introduce a new machine-readable format contract in this ticket.
- If structured output is needed later, define it in a dedicated follow-up spec item (for example `--format`).

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
