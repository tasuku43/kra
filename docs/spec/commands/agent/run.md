---
title: "`kra agent run` baseline"
status: planned
---

# `kra agent run` v3 draft

## Purpose

Start one agent session under broker-managed PTY and register runtime activity under `KRA_HOME`.

## Scope (v3 draft)

- Command:
  - `kra agent run [--workspace <id>] [--repo <repo-key>] [--kind <agent-kind>] [--launch <default|resume|continue>] [--attach]`
- Interactive behavior:
  - with no args, command enters interactive selector flow
  - workspace selector must include active workspaces only
  - execution target must always be selected when `--repo` is not given:
    - run at workspace scope
    - run at repo scope (pick repo key)
  - if `--kind` is omitted, prompt for kind selection
  - if `--launch` is omitted, use `default`
- Flags removed in v3:
  - `--task`
  - `--instruction`
  - `--status`
  - `--log-path`
- Behavior:
  - resolve current `KRA_ROOT`
  - connect broker socket: `KRA_HOME/run/agent/<root-hash>.sock`
  - if socket is missing/stale, spawn broker and reconnect
  - resolve run target (workspace scope or repo scope)
  - broker creates new `session_id`, allocates PTY, and starts provider process
  - broker persists snapshot (`KRA_HOME/state/agents/<root-hash>/<session-id>.json`)
  - broker appends lifecycle events (`KRA_HOME/state/agents/<root-hash>/events/<session-id>.jsonl`)
  - if same target+kind has active session, print warning but still allow start
  - if `--attach` is set, attach caller to created session; otherwise return in detached mode
  - print a human confirmation line including `session_id`
- Launch mode mapping:
  - `kind=codex`:
    - `default` -> `codex`
    - `resume` -> `codex resume`
    - `continue` -> unsupported (fail fast)
  - `kind=claude`:
    - `default` -> `claude`
    - `resume` -> `claude --resume`
    - `continue` -> `claude --continue`

## Out of scope (v3 draft)

- Rich instruction/task metadata capture in `run`.
- Provider-specific conversation data introspection inside `kra`.
