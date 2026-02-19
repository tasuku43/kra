---
title: "Agent attach stream primitive (internal)"
status: implemented
---

# Agent Attach Stream (internal)

## Purpose

Attach current terminal to an existing broker-managed agent session.

This is an internal app-layer primitive for terminal stream connection and
reattach. It is currently not exposed as a direct user-facing subcommand.
Primary daily flow is `kra agent run` foreground execution.

## Scope (implemented)

- Internal behavior:
  - validate target session id and scope (workspace/repo context)
  - resolve current `KRA_ROOT`
  - connect broker socket for the root hash
  - broker replays buffered PTY output for selected session
  - after replay catch-up, switch to live attach stream for selected session PTY
- Exposure:
  - not listed in `kra agent` subcommand help
  - reserved for reuse by manager-side connection paths

## Context Resolution Rules

- Inside `workspaces/<id>/repos/<repo-key>/...`:
  - candidate scope = same `workspace + repo`
- Inside `workspaces/<id>/...`:
  - candidate scope = same workspace
- At `KRA_ROOT` root:
  - fail (context too broad for `attach`)
- Outside `KRA_ROOT`:
  - fail

## Output Contract

- Success:
  - caller terminal enters attached stream until session exits or connection closes
  - replay is emitted first so attached terminal can recover current visual context
- Errors:
  - clear reason + next action (missing broker, session not found, invalid context)
  - non-zero exit code

## Deferred to AGENT-100

- writer lease / takeover control
- dangerous key confirmation (`Ctrl-C`, `Ctrl-D`, `Ctrl-Z`)
