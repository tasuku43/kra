---
title: "`kra ws dashboard`"
status: proposed
---

# `kra ws dashboard [--archived] [--workspace <id>] [--format human|json]`

## Purpose

Provide a one-screen operational overview across workspace status, risk, and context.

## Data sources

- workspace metadata (`.kra.meta.json`)
- live repo risk signals (same policy as `ws close`)
- current context (`~/.kra/state/current-context`)
- filesystem-derived output coverage

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
  - output coverage totals (`empty`, `notes-only`, `artifacts-only`, `documented`)
- workspace rows:
  - `id`, `title`, `risk`, `repos`, `coverage`
- with `--workspace <id>`, show one detailed panel:
  - repo-level risk tree
  - workspace-level aggregated risk
  - output coverage summary
  - summary cards still reflect global active/archived counts for the root, not the filtered subset

## JSON envelope

- `ok`
- `action=ws.dashboard`
- `result`:
  - `root`
  - `context`
  - `summary`
  - `workspaces[]`
  - `generated_at`
  - `warnings[]`
- `error`

## Performance/safety

- read-only command (no mutation)
- must not create baseline files or rewrite `.kra.meta.json`
- should degrade gracefully when optional sources are missing
- in degraded mode, return `ok=true` with warning details in `result.warnings[]` where possible
- with `--debug`, emit phase timing entries to debug log for list/risk/context/summary/render steps
- summary counts should use lightweight scope counting and must not rebuild workspace rows only for card totals

## Non-goals (phase 1)

- full TUI mode
- sparkline/history charts
- remote API aggregation
