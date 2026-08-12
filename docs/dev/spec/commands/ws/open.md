---
title: "`kra ws open`"
status: implemented
---

# `kra ws open [--id <id> | --current | --select | --multi-select | --all] [--command <cmd>] [--concurrency <n>] [--format human|json]`

## Purpose

Open cmux workspace(s) from workspace action entrypoint and notify the user from the opened target.

## Inputs

- target mode:
  - `--id <id>`
  - `--current`
  - `--select`
  - `--multi-select`
  - `--all`
- batch options:
  - `--multi-select`
  - `--concurrency <n>`
- command option:
  - `--command <cmd>`: send `<cmd>` to the cmux workspace terminal after the workspace has started in `<workspace-path>`

## Behavior

- Uses cmux integration flow from workspace command entrypoint.
- `--id` targets one active workspace.
- `--current` resolves workspace from current path under `workspaces/<id>/...`.
- `--select` opens workspace selector and resolves target workspace(s) interactively.
- `--multi-select` enables multi-target open flow.
- `--all` resolves all active workspaces non-interactively and opens them sequentially.
- `--concurrency` is valid only with `--multi-select`.
- `--all` accepts `--concurrency` for compatibility, but always uses effective concurrency `1`.
- `--multi-select` かつ `--concurrency` 未指定時は、自動並列度（`min(targets, max(2, GOMAXPROCS))`）で goroutine 実行する。
- JSON mode remains non-interactive.
- `--command <cmd>` behavior:
  - with Workspace Group support, create a new workspace with direct `cwd=<workspace-path>`, then send
    `<cmd>\n` to its selected surface
  - in the legacy ungrouped fallback, create the workspace with startup command `cd <workspace-path>`, then send
    `<cmd>\n` to the selected surface
  - for reused cmux workspaces, send `<cmd>\n` to the selected surface
  - command send failures are command failures (`cmux_send_command_failed`) rather than best-effort warnings
  - in fallback directory-open mode, `--command` is not executed
- On successful open, advance each succeeded target's `.kra.meta.json.workspace.work_state` to
  `in-progress`.
  - transition is monotonic (`todo -> in-progress`)
  - already `in-progress` workspaces remain unchanged
  - failed targets must not be advanced
- 1:1 policy (`kra workspace` : `cmux workspace`):
  - when no mapping exists, create a new cmux workspace with direct `cwd`, title, `focus=false`, and membership
    in the root Workspace Group in one `workspace.create` operation
  - creation uses a UUID `operation_id` for durable at-most-once startup when the cmux method supports it
  - if a typed create response is lost while the caller remains active, retry exactly once with the same
    `operation_id`; if the result stays uncertain, expose the operation/window/group identity for manual repair
  - after a cmux workspace is created/reused, raise a best-effort `cmux notify` notification from that opened
    workspace (`--workspace <cmux-workspace-id> --surface <selected-surface-id>`)
  - `ws open` must not select/focus the cmux workspace; users follow the cmux notification action to jump to it
  - when mapping already exists and runtime workspace is reachable, create is skipped and the existing workspace is reused
  - when saved mapping is stale but exactly one runtime workspace matches the expected `kra` title
    (`FormatWorkspaceTitle(<workspace-id>, <workspace-title>, <existing-ordinal>)`), update mapping to that runtime workspace and reuse it instead of creating a new one
  - if title match is missing or ambiguous, stale mapping falls back to new workspace creation
- Workspace Group policy:
  - ensure one root group and dedicated `KRA_ROOT` anchor before creating child workspaces
  - a reused ungrouped KRA runtime is added to the root group
  - a reused runtime already owned by a foreign group yields `cmux_group_conflict` and is not moved
  - foreign group members and presentation preferences are preserved
  - missing group capabilities retain the ungrouped flow and return a non-fatal warning
- cmux capability fallback:
  - when cmux runtime/capabilities are unavailable (`cmux_capability_missing`) and target is single workspace,
    command falls back to directory-open behavior (emit shell action `cd <workspace-path>`)
  - fallback response marks `mode=fallback-cd` and keeps `cwd_synced=true`
  - when multiple targets are requested, fallback is not applied (return `cmux_capability_missing`)
- Newly-created runtime compensation:
  - a newly-created cmux workspace remains provisional until required workspace/group setup, optional command
    delivery, any requested selection, and mapping persistence have succeeded
  - when a mandatory post-create step fails, restore that target's pre-open in-memory mapping and close only the
    newly-created cmux workspace
  - reused or title-relinked cmux workspaces must never be closed as failure compensation
  - a mapping-save failure closes every newly-created successful result from that invocation; reused results are
    left open
  - notification is best-effort and does not trigger compensation
  - notification runs after mapping persistence and outside the mapping transaction lock
  - compensation uses an independent bounded context so caller cancellation does not skip cleanup
  - if compensation close also fails, preserve the primary failure code and report the created cmux workspace ID,
    cleanup error, and manual-cleanup requirement

## Commit behavior

- On successful open, `ws open` auto-commits kra-managed state updates in `KRA_ROOT`.
- Commit messages:
  - single target: `open: <workspace-id>`
  - multiple targets: `open: <n> workspaces`
- Staging allowlist:
  - `workspaces/<id>/.kra.meta.json` for each successful target
- `.kra/state/cmux-workspaces.json` is ignored runtime-local state and must not be staged or committed, even when
  the file exists.
- In fallback directory-open mode, the allowlist only captures workspace metadata changes.
- Preserve unrelated staged changes outside the allowlist.
- If the operation produces no Git diff, do not create an empty commit.

## Notes

- Parent shell cwd mutation still follows action-file protocol.
- Workspace-level command shape is `kra ws open ...`.
