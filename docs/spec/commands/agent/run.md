---
title: "`kra agent run`"
status: implemented
---

# `kra agent run`

## Purpose

Start one agent session under broker-managed PTY and register runtime activity under `KRA_HOME`.

## Scope (implemented)

- Command:
  - `kra agent run [--workspace <id>] [--repo <repo-key>] [--kind <agent-kind>]`
- Interactive behavior:
  - with no args:
    - when `cwd` is under `workspaces/<id>/...`, workspace selection is skipped and `<id>` is used
    - otherwise command enters interactive workspace selector flow
  - workspace selector includes active workspaces only
  - when `--repo` is omitted and stdin is TTY, execution target is selected:
    - run at workspace scope
    - run at repo scope (pick repo key)
  - when `--kind` is omitted and stdin is TTY, kind selector is shown
- Flags removed:
  - `--task`
  - `--instruction`
  - `--status`
  - `--log-path`

## Behavior (implemented)

- resolve current `KRA_ROOT`
- resolve workspace target by this order:
  1. explicit `--workspace`
  2. `cwd` context (`workspaces/<id>/...`)
  3. interactive selector (TTY only)
- fail fast when workspace is unresolved in non-interactive mode
- connect broker socket: `KRA_HOME/run/agent/<root-hash>.sock`
- if socket is missing/stale, spawn broker and reconnect
- resolve run target (workspace scope or repo scope)
- broker creates new `session_id`, allocates PTY, and starts provider process
- broker initializes per-session in-memory output replay buffer
- broker persists snapshot: `KRA_HOME/state/agents/<root-hash>/<session-id>.json`
- if same target+kind has active session, print warning but still allow start
- print confirmation line including `session_id`
- run is detached by default

## Out of scope (current)

- launch abstraction (`--launch default|resume|continue`)
- run-time auto-attach option (`--attach`)
- append-only events stream for runtime lifecycle
