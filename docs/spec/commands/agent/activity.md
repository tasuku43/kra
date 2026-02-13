---
title: "`kra agent` activity tracking"
status: implemented
---

# `kra agent` activity tracking (v2)

## Purpose

Provide an operational command and data contract to observe agent execution across workspaces:
- which agent is running in which workspace
- which repo scope and work summary it is handling
- current lifecycle state (`running` / `waiting_user` / `thinking` / `blocked` / `succeeded` / `failed` / `unknown`)
- where to inspect logs

## Scope (v2)

- Command boundary is `kra agent ...`.
- Baseline command surface:
  - `kra agent run`
  - `kra agent stop`
  - `kra agent logs`
  - `kra agent list` (`ls` alias)
- Discoverability policy:
  - `kra agent` is executable directly.
  - root help intentionally does not list `agent`.
- Tracked fields (read model):
  - `workspace_id`
  - `repo_key` (optional)
  - `agent_kind`
  - `task_summary` (optional)
  - `instruction_summary` (optional)
  - `started_at`
  - `last_heartbeat_at`
  - `status`
  - `log_path`
- Status model:
  - live states: `running`, `waiting_user`, `thinking`, `blocked`
  - final states: `succeeded`, `failed`, `unknown`
- `kra agent run` options (v2 extension):
  - `--workspace <id>` (required)
  - `--kind <agent-kind>` (required)
  - `--repo <repo-key>` (optional)
  - `--task <summary>` (optional)
  - `--instruction <summary>` (optional)
  - `--status <running|waiting_user|thinking|blocked>` (optional; default: `running`)
  - `--log-path <path>` (optional)
- Data source for `list`:
  - `KRA_ROOT/.kra/state/agents.json`
  - missing or empty file means empty list.
- `kra agent list` options:
  - `--workspace <id>`: filter by workspace id
  - `--format human|tsv` (default: human)
- `kra agent list` output fields:
  - human: includes `workspace_id`, `repo_key` (if set), `agent_kind`, `status`, `last_heartbeat_at`,
    `task_summary`/`instruction_summary` (if set), and `log_path` (if set)
  - tsv header order:
    `workspace_id`, `repo_key`, `agent_kind`, `task_summary`, `instruction_summary`,
    `started_at`, `last_heartbeat_at`, `status`, `log_path`

## Out of scope (v2)

- Strong process supervision guarantees (PID ownership, hard crash recovery)
- External process discovery for agents launched outside `kra`
- Long-term retention policy and log redaction policy
- Rich historical timeline/event stream (planned in later phase)
