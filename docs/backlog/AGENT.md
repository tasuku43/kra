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
  - What: add a log viewing command to inspect recent/streaming logs by workspace
    and/or agent session, aligned with existing output contract conventions.
  - Specs:
    - `docs/spec/commands/agent/logs.md`
    - `docs/spec/concepts/output-contract.md`
    - `docs/spec/commands/agent/activity.md`
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

- [ ] AGENT-050: tmux/zellij bridge (agent lifecycle auto-report)
  - What: add operator-facing launcher/wrapper flow that reports
    `run -> heartbeat/update -> stop` automatically from tmux/zellij sessions.
  - Specs:
    - `docs/spec/commands/agent/run.md`
    - `docs/spec/commands/agent/stop.md`
    - `docs/spec/commands/agent/activity.md`
  - Depends: AGENT-040
  - Serial: yes

- [ ] AGENT-060: Current activity inspection (`agent list`/`agent logs`) v2 output
  - What: expose v2 fields in human/json/tsv outputs so operators can answer:
    who runs what, where, with what instruction summary, and whether user input is required.
  - Specs:
    - `docs/spec/commands/agent/activity.md`
    - `docs/spec/commands/agent/logs.md`
    - `docs/spec/concepts/output-contract.md`
  - Depends: AGENT-040
  - Parallel: yes

- [ ] AGENT-100: Agent timeline and detailed execution history (Phase3)
  - What: introduce append-only event timeline and richer transitions
    for postmortem/debug/audit views (keep v2 snapshot compatibility).
  - Specs:
    - `docs/spec/commands/agent/activity.md`
    - `docs/spec/concepts/output-contract.md`
  - Depends: AGENT-050, AGENT-060
  - Serial: yes
