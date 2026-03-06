---
title: "External Command Execution"
status: implemented
---

# External Command Execution

## Purpose

Standardize how `kra` invokes local external tools such as `git`, `gh`, and `cmux`.

Goals:

- honor caller cancellation when a context already has deadline/cancel
- apply a safe default timeout when caller uses `context.Background()`
- keep stderr/stdout handling consistent
- surface timeout failures as `context deadline exceeded`-compatible errors

## Policy

- If caller context already has a deadline, runner must respect it.
- If caller context has no deadline, runner applies tool-family default timeout.
- Current defaults:
  - `git`: `2m`
  - `gh`: `2m`
  - `cmux`: `15s`

## Error contract

- Timeout failures must wrap `context.DeadlineExceeded`.
- Callers may continue to add command-specific prefixes/messages.
- Non-timeout process failures preserve normal exit-error handling.

## Scope

- shared process execution helper is used by:
  - `internal/gitutil`
  - `internal/repodiscovery` (`gh`)
  - `internal/infra/cmuxctl`

## Non-goals

- per-command timeout configuration flags
- retry/backoff policy
- structured telemetry export beyond debug logs
