---
title: "`kra agent run` baseline"
status: implemented
---

# `kra agent run` v2

## Purpose

Provide an explicit start command to register one live agent activity per workspace.

## Scope (v2)

- Command:
  - `kra agent run --workspace <id> --kind <agent-kind> [--repo <repo-key>] [--task <summary>] [--instruction <summary>] [--status <running|waiting_user|thinking|blocked>] [--log-path <path>]`
- Required options:
  - `--workspace`
  - `--kind`
- Optional options:
  - `--repo`
  - `--task`
  - `--instruction`
  - `--status` (default: `running`)
  - `--log-path`
- Data write target:
  - `KRA_ROOT/.kra/state/agents.json`
- Behavior:
  - resolve current `KRA_ROOT`
  - load existing activity records (missing file is treated as empty)
  - upsert a single record by `workspace_id`
  - set fields:
    - `workspace_id` = `--workspace`
    - `repo_key` = `--repo` (or empty)
    - `agent_kind` = `--kind`
    - `task_summary` = `--task` (or empty)
    - `instruction_summary` = `--instruction` (or empty)
    - `started_at` = current unix timestamp
    - `last_heartbeat_at` = current unix timestamp
    - `status` = `--status` (default: `running`)
    - `log_path` = `--log-path` (or empty)
  - write back JSON and print a human confirmation line

## Out of scope (v2)

- Actual process supervision/ownership guarantees
- Multi-session history per workspace (baseline keeps one record per workspace)
- Automatic log path generation policy
