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

- [ ] AGENT-030: `kra agent logs` baseline
  - What: add a log viewing command to inspect recent/streaming logs by workspace
    and/or agent session, aligned with existing output contract conventions.
  - Specs:
    - `docs/spec/commands/agent/logs.md`
    - `docs/spec/concepts/output-contract.md`
    - `docs/spec/commands/agent/activity.md`
  - Depends: AGENT-010
  - Parallel: yes
