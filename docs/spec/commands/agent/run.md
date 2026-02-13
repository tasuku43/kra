---
title: "`kra agent run` baseline"
status: implemented
---

# `kra agent run` baseline

## Purpose

Provide an explicit start command to register one running agent activity per workspace.

## Scope (baseline)

- Command:
  - `kra agent run --workspace <id> --kind <agent-kind> [--log-path <path>]`
- Required options:
  - `--workspace`
  - `--kind`
- Data write target:
  - `KRA_ROOT/.kra/state/agents.json`
- Behavior:
  - resolve current `KRA_ROOT`
  - load existing activity records (missing file is treated as empty)
  - upsert a single record by `workspace_id`
  - set fields:
    - `workspace_id` = `--workspace`
    - `agent_kind` = `--kind`
    - `started_at` = current unix timestamp
    - `last_heartbeat_at` = current unix timestamp
    - `status` = `running`
    - `log_path` = `--log-path` (or empty)
  - write back JSON and print a human confirmation line

## Out of scope (baseline)

- Actual process supervision/ownership guarantees
- Multi-session history per workspace (baseline keeps one record per workspace)
- Automatic log path generation policy
