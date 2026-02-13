---
title: "`kra agent stop` baseline"
status: implemented
---

# `kra agent stop` baseline

## Purpose

Provide an explicit stop command to finalize one running agent activity for a workspace.

## Scope (baseline)

- Command:
  - `kra agent stop --workspace <id> [--status succeeded|failed|unknown]`
- Required options:
  - `--workspace`
- Optional options:
  - `--status` (default: `failed`)
- Data write target:
  - `KRA_ROOT/.kra/state/agents.json`
- Behavior:
  - resolve current `KRA_ROOT`
  - load existing activity records
  - find record by `workspace_id`
  - if record does not exist: fail
  - if record status is not `running`: fail
  - otherwise update:
    - `status` = selected final status
    - `last_heartbeat_at` = current unix timestamp
  - write back JSON and print a human confirmation line

## Out of scope (baseline)

- Process signal delivery / external process termination
- Automatic final status inference from runtime signals
