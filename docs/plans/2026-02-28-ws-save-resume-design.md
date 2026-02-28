# WS Save/Resume UX Design (v1)

Date: 2026-02-28
Status: agreed
Scope: `kra ws save` / `kra ws resume` (cmux workspace session restore)

## Goals

- Make interrupted work easy to continue without requiring users to remember pane/surface details.
- Keep command UX short and consistent with existing workspace targeting (`--id`, `--current`, `--select`).
- Treat restore as `best-effort` by default, with explicit `--strict` for automation-grade failure semantics.

## Command UX (v1)

- Save:
  - `kra ws save [--id <id> | --current | --select] [-l <label>] [--no-browser-state] [--format human|json]`
- Resume:
  - `kra ws resume [--id <id> | --current | --select] [--latest] [--strict] [--no-browser] [--format human|json]`

Notes:
- User-facing snapshot id arguments are intentionally not exposed in v1.
- `--select` on `resume` uses two-step selection:
  1) select workspace
  2) select saved session in that workspace
- `--id` / `--current` on `resume` skip workspace selection and go directly to session selection.
- `--latest` skips session selection and restores the latest session for resolved workspace.
- Browser state is saved by default; `--no-browser-state` opt-out is provided for speed/stability.

## Restore Semantics

- Default mode (`best-effort`):
  - command succeeds (`ok=true`) when at least workspace resolution + restore attempt runs.
  - partial restore is allowed.
  - failures are returned as `warnings[]` with per-target reason.
- Strict mode (`--strict`):
  - any unresolved required restore item makes command fail (non-zero, `ok=false`).
- Initial restore scope:
  - workspace selection/focus
  - pane/surface inventory-aware restore attempt
  - terminal context hints (focus target + captured screen references)
  - browser state load when saved and not disabled
- Non-goal for v1:
  - full deterministic pane topology replay.

## Data Contract and Storage

- Save artifact path:
  - `workspaces/<id>/artifacts/cmux/sessions/<timestamp>-<slug>/`
  - (archived workspace uses `archive/<id>/...` when applicable)
- Artifacts include:
  - `session.json` (metadata + topology summary + restore hints)
  - `screen/<surface-ref>.txt` (captured text, bounded lines)
  - `browser/<surface-ref>.state.json` (when available)
- State index:
  - `.kra/state/cmux-sessions.json`
  - Contains per-workspace ordered session metadata used by selector and `--latest`.

## cmux API Orchestration (v1)

- Save path uses:
  - `identify`, `list-panes`, `list-pane-surfaces`, `read-screen`
  - `browser state save` (best-effort per browser surface)
- Resume path uses:
  - `select-workspace`, `focus-pane` / `tab-action` (where applicable)
  - `browser state load` (best-effort unless `--strict`)
  - optional user feedback hooks: `notify`, `set-status`, `set-progress`

## Error and Output Contract

- JSON envelope:
  - `ok`
  - `action` (`ws.save` or `ws.resume`)
  - `workspace_id`
  - `result`
  - `warnings[]` (best-effort diagnostics)
  - `error` (set when `ok=false`)
- Error code examples:
  - `workspace_not_found`
  - `cmux_not_mapped`
  - `cmux_runtime_unavailable`
  - `session_not_found`
  - `session_restore_partial` (strict mode failure)
  - `invalid_argument`

## Architecture and Test Strategy

- Layering:
  - `internal/cli`: parse flags, route command, render outputs/selectors
  - `internal/app/ws` (or dedicated `internal/app/cmuxsession`): orchestration/use case
  - `internal/infra/cmuxctl`: cmux command adapters
- Tests:
  - app unit tests for save/resume decision matrix (`best-effort`, `strict`, `latest`, selector path)
  - cli tests for usage/flag conflicts/JSON envelope
  - regression tests to ensure existing `ws open` behavior is unchanged

