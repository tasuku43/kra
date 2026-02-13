---
title: "`kra ws dashboard`"
status: implemented
---

# `kra ws dashboard [--archived] [--workspace <id>] [--format human|json]`

## Purpose

Provide a one-screen operational overview across workspace status, risk, context, and agent activity.

## Data sources

- workspace metadata (`.kra.meta.json`)
- live repo risk signals (same policy as `ws close`)
- current context (`~/.kra/state/current-context`)
- agent activity (`KRA_ROOT/.kra/state/agents.json`)

## Behavior

Default scope is active workspaces.

- `--archived` switches list scope to archived workspaces.
- phase 1 default without `--archived` is active-only list.

- header:
  - root path
  - context name
  - generated timestamp
- summary cards:
  - `active`, `archived`
  - risk totals (`clean`, `warning`, `danger`, `unknown`)
  - running agent count
- workspace rows:
  - `id`, `title`, `risk`, `repos`, `agent_status`
- with `--workspace <id>`, show one detailed panel:
  - repo-level risk tree
  - workspace-level aggregated risk

## JSON envelope

- `ok`
- `action=ws.dashboard`
- `result`:
  - `root`
  - `context`
  - `summary`
  - `workspaces[]`
  - `generated_at`
- `error`

## Performance/safety

- read-only command (no mutation)
- should degrade gracefully when optional sources are missing (`agents.json`)
- in degraded mode, return `ok=true` with warning details in `result.warnings[]`

## Non-goals (phase 1)

- full TUI mode
- sparkline/history charts
- remote API aggregation
