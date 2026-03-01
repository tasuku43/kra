---
title: "cmux workspace title and ordinal policy"
status: implemented
---

# cmux Workspace Title And Ordinal Policy

## Purpose

Provide deterministic title rendering and per-workspace ordinal allocation for
cmux workspace creation flows.

## Title Format

- `"<kra-id> | <kra-title> [<n>]"`
- `n` is 1-based (`[1]` is the first cmux workspace under one `kra` workspace).

## Fallback Rules

- If `kra-title` is empty, use `(untitled)`.
- `kra-id` is required.
- `n` must be `>= 1`.

## Ordinal Allocation

- Allocation is per `kra` workspace ID.
- Source of truth is `.kra/state/cmux-workspaces.json`.
- Each workspace mapping keeps `next_ordinal`.
- On allocation:
  - return current `next_ordinal`
  - increment and persist `next_ordinal`
