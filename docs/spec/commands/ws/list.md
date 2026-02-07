---
title: "`gionx ws list`"
status: implemented
pending:
  - UX-WS-006-selector-parity-output
  - UX-WS-006-tree-detail-mode
  - UX-WS-006-machine-readable-policy
---

# `gionx ws list`

## Purpose

List workspaces with status and risk, similar in spirit to `gion manifest ls`.

`ws list` is a read-only listing command. Interactive selection flows are specified in `selector.md` and
action commands (`ws close`, `ws go`, `ws reopen`, `ws purge`).

## Role boundary

- `ws list` only shows current state and exits (non-interactive).
- Action execution belongs to `ws close/go/reopen/purge` selector flows.

## Default display (planned refinement, UX-WS-006)

- One row per workspace (summary view) using selector-parity visual hierarchy.
- Row content is single-line and summary-first:
  - `id`
  - `status`
  - `risk` (currently color-only in row body; textual risk tags are optional and still under discussion)
  - `repo_count`
  - `description`
- `updated_at` should be available in expanded detail or optional metadata line, not mandatory in the default
  summary line.
- Summary output should follow the same shared row rendering semantics as selector flows
  (`commands/ws/selector.md`), while remaining non-interactive.
- Status label coloring must follow shared semantics from selector UI:
  - `active`: active accent color
  - `archived`: archived accent color
  - no-color terminals: plain text fallback

## Expanded display (planned refinement, UX-WS-006)

- `--tree` shows repo-level detail under each workspace row.
- Default output remains summary-first to keep task-list UX and scripting usage simple.
- Repo tree lines are supplemental information and should use muted/low-contrast styling consistent with
  `commands/ws/selector.md` visual rules.

## Machine-readable output policy

- `ws list` is specified as human-oriented task-list output in this UX phase.
- Machine-readable output policy is still open:
  - keep current TSV contract, or
  - replace with explicit `--format` contract (`tsv`/`json` etc.) in a follow-up.
- This ticket should not finalize a long-term machine-readable contract without an explicit decision.

## Open points (to finalize before implementation)

- whether default summary row includes textual risk tags (`[clean]`, `[dirty]`) or color-only risk hint
- exact `--tree` shape (indent depth, repo metadata fields, missing repo marker style)
- machine-readable policy (`legacy TSV` keep vs `--format` introduction)

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
