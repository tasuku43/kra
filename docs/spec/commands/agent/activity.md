---
title: "`kra agent` activity tracking"
status: implemented
---

# `kra agent` activity tracking (baseline)

## Purpose

Provide a baseline command and data contract to observe agent execution across workspaces:
- which agent is running in which workspace
- current lifecycle state (`running` / `succeeded` / `failed` / `unknown`)
- where to inspect logs

## Scope (baseline)

- Command boundary is `kra agent ...`.
- Baseline command surface:
  - `kra agent run`
  - `kra agent stop`
  - `kra agent list` (`ls` alias)
- Discoverability policy:
  - `kra agent` is executable directly.
  - root help intentionally does not list `agent`.
- Tracked fields (read model):
  - `workspace_id`
  - `agent_kind`
  - `started_at`
  - `last_heartbeat_at`
  - `status`
  - `log_path`
- Data source for `list`:
  - `KRA_ROOT/.kra/state/agents.json`
  - missing or empty file means empty list.
- `kra agent list` options:
  - `--workspace <id>`: filter by workspace id
  - `--format human|tsv` (default: human)

## Out of scope (baseline)

- Strong process supervision guarantees (PID ownership, hard crash recovery)
- External process discovery for agents launched outside `kra`
- Long-term retention policy and log redaction policy
