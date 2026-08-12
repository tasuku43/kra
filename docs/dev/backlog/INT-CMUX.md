---
title: "INT-CMUX backlog"
status: planned
---

# INT-CMUX Backlog

Note:
- Current source of truth for user-facing cmux integration is:
  - `docs/dev/spec/commands/ws/open.md`
  - `docs/dev/spec/commands/ws/select.md`
  - `docs/dev/spec/concepts/cmux-mapping.md`
  - `docs/dev/spec/concepts/cmux-title-policy.md`

- [x] CMUX-001: initial cmux integration command surface
  - What: introduced an initial dedicated command surface for cmux flows (later consolidated into `ws open`).
  - Specs:
    - `docs/dev/spec/commands/ws/open.md`
  - Depends: none
  - Serial: yes

- [x] CMUX-002: cmux mapping state store (1:N)
  - What: add persistent mapping store at `.kra/state/cmux-workspaces.json` with schema versioning,
    per-workspace `next_ordinal`, and `entries[]` lifecycle fields.
  - Specs:
    - `docs/dev/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-001
  - Serial: yes

- [x] CMUX-003: cmux adapter for workspace control primitives
  - What: add adapter layer for cmux CLI/socket interaction used by integration flows:
    capabilities/identify/workspace create/rename/select/list and `surface.send_text` for cwd sync.
  - Specs:
    - `docs/dev/spec/commands/ws/open.md`
    - `docs/dev/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-001
  - Serial: yes

- [x] CMUX-004: title policy + ordinal allocator
  - What: enforce naming format `"<kra-id> | <kra-title> [<n>]"` with ordinal starting at `[1]`
    and deterministic increment policy per `kra` workspace.
  - Specs:
    - `docs/dev/spec/concepts/cmux-title-policy.md`
    - `docs/dev/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-002, CMUX-003
  - Serial: yes

- [x] CMUX-005: workspace open flow v1
  - What: implement strict open flow:
    resolve kra workspace -> create/select cmux workspace -> cwd sync -> persist mapping update.
    If target workspace is omitted in human mode, fallback to interactive workspace selection.
  - Specs:
    - `docs/dev/spec/commands/ws/open.md`
    - `docs/dev/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-004
  - Serial: yes

- [x] CMUX-006: existing mapping selection policy
  - What: implement explicit-first resolution and ambiguity fallback for mapped targets and update
    mapping metadata on successful selection.
  - Specs:
    - `docs/dev/spec/commands/ws/open.md`
    - `docs/dev/spec/commands/ws/select.md`
    - `docs/dev/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-002, CMUX-003
  - Serial: yes

- [x] CMUX-007: mapping visibility/health foundation
  - What: provide mapping visibility and runtime health evaluation in integration flow internals.
  - Specs:
    - `docs/dev/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-002, CMUX-003
  - Parallel: yes

- [x] CMUX-008: docs finalization + regression guard
  - What: finalize specs and add regression tests ensuring `kra ws open` integration behavior remains stable.
  - Specs:
    - `docs/dev/spec/commands/ws/open.md`
    - `docs/dev/spec/commands/ws/select.md`
  - Depends: CMUX-005, CMUX-006, CMUX-007
  - Serial: yes

- [x] CMUX-009: `kra ws open --multi` v2
  - What: add multi-target open flow for batch workspace provisioning.
    Supports multi-selection and explicit repeated `--workspace <id>` inputs.
  - Specs:
    - `docs/dev/spec/commands/ws/open.md`
  - Depends: CMUX-005
  - Serial: yes

- [x] CMUX-010: `kra ws open --multi --concurrency <n>` v2
  - What: add bounded parallel execution with result aggregation and partial-failure reporting
    for batch open operations.
  - Specs:
    - `docs/dev/spec/commands/ws/open.md`
  - Depends: CMUX-009
  - Serial: yes

- [x] CMUX-011: `kra ws open` entrypoint migration
  - What: make `ws open` the primary user-facing cmux integration action and support
    workspace targeting options (`--id` / `--current` / `--select`) at `ws` action level.
  - Specs:
    - `docs/dev/spec/commands/ws/open.md`
    - `docs/dev/spec/commands/ws/select.md`
  - Depends: CMUX-005, CMUX-006
  - Serial: yes

- [x] CMUX-012: remove dedicated `cmux` command group
  - What: remove top-level `cmux` CLI entrypoint and route user-facing cmux operations through
    `kra ws open`.
  - Specs:
    - `docs/dev/spec/commands/ws/open.md`
    - `docs/dev/spec/commands/ws/select.md`
  - Depends: CMUX-011
  - Serial: yes

- [x] CMUX-013: `ws open` runtime-state and side-effect integrity
  - What: keep ignored cmux mapping state outside Git commits and treat newly-created cmux workspaces as
    provisional until mandatory setup and mapping persistence succeed. Compensate failures by closing only the
    newly-created runtime, never close reused/relinked workspaces, and align the current mapping/title contract
    with the implemented 1:1 policy.
  - Specs:
    - `docs/dev/spec/commands/ws/open.md`
    - `docs/dev/spec/concepts/cmux-mapping.md`
    - `docs/dev/spec/concepts/cmux-title-policy.md`
  - Depends: CMUX-012
  - Serial: yes

- [x] CMUX-014: KRA root Workspace Group
  - What: represent one KRA root as a cmux Workspace Group whose managed `KRA_ROOT` runtime is the anchor
    and whose managed KRA workspace runtimes are children. Persist window/group/anchor identity as ignored
    runtime state, reconcile stale membership without taking over foreign groups, and keep the legacy
    ungrouped open flow as an explicit capability fallback.
  - Specs:
    - `docs/dev/spec/concepts/cmux-workspace-groups.md`
    - `docs/dev/spec/commands/root.md`
    - `docs/dev/spec/commands/ws/open.md`
    - `docs/dev/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-013
  - Serial: yes

- [x] CMUX-015: cmux native Todo three-way reconciliation
  - What: project KRA root/workspace tasks into the mapped cmux workspace native Todo checklist and reconcile
    task state through local document, live Todo, and ignored last-sync base. Preserve foreign Todo items,
    fail closed on divergent edits or capacity loss, and make `task sync` the explicit permission boundary.
  - Specs:
    - `docs/dev/spec/concepts/cmux-todo-reconcile.md`
    - `docs/dev/spec/concepts/workspace-tasks.md`
    - `docs/dev/spec/concepts/workspace-tasks-yaml.md`
    - `docs/dev/spec/commands/task.md`
    - `docs/dev/spec/commands/ws/task.md`
    - `docs/dev/spec/commands/ws/task/sync.md`
  - Depends: CMUX-014, UX-WS-033, UX-CORE-014
  - Serial: yes

- [x] CMUX-016: retire `kra serve` and legacy task status-pill/Dock paths
  - What: remove the local web server and obsolete sidebar status-pill synchronization, stop scaffolding or
    installing the legacy task Dock, and make cmux Workspace Groups plus native Todo the supported runtime UI.
    Preserve existing user-authored Dock files but do not create or migrate new managed entries.
  - Specs:
    - `docs/dev/spec/commands/serve.md`
    - `docs/dev/spec/commands/root.md`
    - `docs/dev/spec/commands/ws/task.md`
    - `docs/dev/spec/concepts/workspace-template.md`
  - Depends: CMUX-015, OPS-018
  - Serial: yes
