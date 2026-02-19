---
title: "AGENT backlog"
status: planned
---

# AGENT Backlog

- [x] AGENT-001: agent activity view baseline spec
  - What: define a first-class `agent` command surface and state model to track
    "which agent runs in which workspace" with lifecycle visibility (`running/succeeded/failed/unknown`).
  - Specs:
    - `docs/spec/commands/agent/activity.md`
  - Depends: DOC-QUALITY-005
  - Serial: yes

- [x] AGENT-010: `kra agent run` baseline
  - What: add explicit agent start command to launch one agent for one workspace
    with initial state persistence and lifecycle transition to `running`.
  - Specs:
    - `docs/spec/commands/agent/run.md`
    - `docs/spec/commands/agent/activity.md`
  - Depends: AGENT-001
  - Serial: yes

- [x] AGENT-020: `kra agent stop` baseline
  - What: add explicit stop/cancel command for running agent sessions with
    deterministic state transition (`running -> failed|succeeded|unknown`) and
    clear non-interactive behavior.
  - Specs:
    - `docs/spec/commands/agent/stop.md`
    - `docs/spec/commands/agent/activity.md`
  - Depends: AGENT-010
  - Serial: yes

- [x] AGENT-030: `kra agent logs` baseline
  - What: retire `kra agent logs` from default agent surface and align
    runtime visibility to PTY + state snapshot/events.
  - Specs:
    - `docs/spec/commands/agent/logs.md`
    - `docs/spec/concepts/agent-runtime.md`
  - Depends: AGENT-010
  - Parallel: yes

- [x] AGENT-040: Agent activity v2 state model (workspace/repo/task/instruction summary)
  - What: extend activity schema for practical operations visibility:
    workspace + repo scope, task summary, instruction summary, and richer runtime states
    (`running`/`waiting_user`/`thinking`/`blocked`/`succeeded`/`failed`/`unknown`).
  - Specs:
    - `docs/spec/commands/agent/activity.md`
    - `docs/spec/concepts/output-contract.md`
  - Depends: AGENT-001, OPS-003
  - Serial: yes

- [x] AGENT-050: Local broker runtime foundation (per-root socket + detached default)
  - What: implement per-`KRA_ROOT` local broker runtime foundation
    (Unix socket, broker spawn/reconnect, detached default session lifecycle).
  - Specs:
    - `docs/spec/commands/agent/run.md`
    - `docs/spec/concepts/agent-runtime.md`
    - `docs/spec/commands/agent/activity.md`
  - Depends: AGENT-040
  - Serial: yes

- [x] AGENT-060: `kra agent attach` and scope-based reattach flow
  - What: add `kra agent attach` with strict current-context scope resolution
    and interactive session selection when `--session` is omitted.
  - Specs:
    - `docs/spec/commands/agent/attach.md`
    - `docs/spec/commands/agent/activity.md`
    - `docs/spec/concepts/agent-runtime.md`
  - Depends: AGENT-050
  - Serial: yes

- [x] AGENT-070: Attach replay baseline (broker-side output history)
  - What: add broker-side per-session output history replay so `kra agent attach`
    can restore terminal-visible context before switching to live stream.
  - Specs:
    - `docs/spec/commands/agent/attach.md`
    - `docs/spec/commands/agent/run.md`
    - `docs/spec/concepts/agent-runtime.md`
  - Depends: AGENT-050, AGENT-060
  - Serial: yes

- [x] AGENT-080: Run foreground default (single-owner terminal flow)
  - What: make `kra agent run` foreground by default in interactive TTY
    (start + immediate stream), while keeping broker/socket runtime and
    detached behavior for non-interactive usage. Hide direct `kra agent attach`
    command path while preserving attach stream logic for manager-side reuse.
  - Specs:
    - `docs/spec/commands/agent/run.md`
    - `docs/spec/commands/agent/attach.md`
    - `docs/spec/concepts/agent-runtime.md`
  - Depends: AGENT-050, AGENT-060
  - Serial: yes

- [ ] AGENT-100: Lease/takeover control plane and launch abstraction
  - What: implement writer lease/takeover + dangerous-key confirmation +
    snapshot/events dual-write and launch mode abstraction (`--launch` mapping).
  - Specs:
    - `docs/spec/commands/agent/run.md`
    - `docs/spec/commands/agent/activity.md`
    - `docs/spec/concepts/agent-runtime.md`
  - Depends: AGENT-050, AGENT-060, AGENT-070, AGENT-080
  - Serial: yes
