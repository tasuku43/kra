---
title: "`kra agent stop` baseline"
status: planned
---

# `kra agent stop` v3 draft

## Purpose

Stop one running session managed by the per-root broker runtime.

## Scope (v3 draft)

- Command:
  - `kra agent stop (--session <id> | --workspace <id> [--repo <repo-key>] [--kind <agent-kind>])`
- Required options:
  - either `--session`, or `--workspace` selector set
- Optional options:
  - `--repo`
  - `--kind`
- Runtime data source:
  - `KRA_HOME/state/agents/<root-hash>/<session-id>.json`
- Behavior:
  - resolve current `KRA_ROOT`
  - connect broker socket for the current root hash
  - locate target session
  - if session is already `exited`, return idempotent success
  - request broker to terminate target process
  - wait bounded grace period, then force kill if still alive
  - persist final runtime state (`exited`) with `updated_at` and `seq` increment
  - print final status line with `session_id`

## Out of scope (v3 draft)

- Distributed stop across remote hosts.
- Multi-step approval flow for stop.
