---
title: "Workspace task overview integration"
status: implemented
---

# Workspace task overview integration

## Purpose

Define how structured workspace tasks appear in overview-oriented workspace commands without
replacing the existing activity-oriented workspace progress model.

## Inputs

- lifecycle status from workspace metadata (`active` / `archived`)
- existing workspace activity/work progress from `.kra.meta.json.workspace.work_state`
  (`todo` / `in-progress`)
- structured `task_state` derived from `workspace.md`

## Primary vs supplemental model

- Overview commands keep the existing activity-oriented workspace progress as the primary signal.
- Structured task data is supplemental.
- Structured task data must not mutate `.kra.meta.json.workspace.work_state`.
- Structured task data must not change `ws list` sort priority.

## Overview summary contract

Overview commands should expose one supplemental task summary per workspace:

- `tasks: empty`
  - no structured tasks exist
- `tasks: invalid`
  - the task contract could not be parsed safely
- `tasks: X total (doing Y, blocked Z, todo A, done B)`
  - valid structured task summary

## Invalid contract handling

- Task-specific commands (`ws task ...`) fail closed on invalid structured task blocks.
- Overview commands (`ws list`, `ws dashboard`) degrade gracefully:
  - primary workspace activity rendering still succeeds
  - task summary becomes `invalid`
  - warning details are exposed in machine-readable output where supported

## Composition examples

- `work_state=todo`, `task_state=empty`
  - primary workspace progress remains `todo`
  - supplemental summary is `tasks: empty`
- `work_state=in-progress`, `task_state=todo`
  - primary workspace progress remains `in-progress`
  - supplemental summary reflects task counts
- `work_state=todo`, `task_state=done`
  - primary workspace progress remains `todo`
  - supplemental summary shows all tasks done
- `work_state=in-progress`, invalid task contract
  - primary workspace progress remains `in-progress`
  - supplemental summary is `tasks: invalid`
