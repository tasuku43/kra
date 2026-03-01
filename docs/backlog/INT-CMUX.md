---
title: "INT-CMUX backlog"
status: planned
---

# INT-CMUX Backlog

Note:
- Top-level `kra cmux` command-group specs were removed.
- Current source of truth for user-facing cmux integration is:
  - `docs/spec/commands/ws/open.md`
  - `docs/spec/commands/ws/select.md`
  - `docs/spec/concepts/cmux-mapping.md`
  - `docs/spec/concepts/cmux-title-policy.md`

- [x] CMUX-001: initial cmux integration command surface
  - What: introduced an initial dedicated command surface for cmux flows (later consolidated into `ws open`).
  - Specs:
    - `docs/spec/commands/ws/open.md`
  - Depends: none
  - Serial: yes

- [x] CMUX-002: cmux mapping state store (1:N)
  - What: add persistent mapping store at `.kra/state/cmux-workspaces.json` with schema versioning,
    per-workspace `next_ordinal`, and `entries[]` lifecycle fields.
  - Specs:
    - `docs/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-001
  - Serial: yes

- [x] CMUX-003: cmux adapter for workspace control primitives
  - What: add adapter layer for cmux CLI/socket interaction used by integration flows:
    capabilities/identify/workspace create/rename/select/list and `surface.send_text` for cwd sync.
  - Specs:
    - `docs/spec/commands/ws/open.md`
    - `docs/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-001
  - Serial: yes

- [x] CMUX-004: title policy + ordinal allocator
  - What: enforce naming format `"<kra-id> | <kra-title> [<n>]"` with ordinal starting at `[1]`
    and deterministic increment policy per `kra` workspace.
  - Specs:
    - `docs/spec/concepts/cmux-title-policy.md`
    - `docs/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-002, CMUX-003
  - Serial: yes

- [x] CMUX-005: workspace open flow v1
  - What: implement strict open flow:
    resolve kra workspace -> create/select cmux workspace -> cwd sync -> persist mapping update.
    If target workspace is omitted in human mode, fallback to interactive workspace selection.
  - Specs:
    - `docs/spec/commands/ws/open.md`
    - `docs/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-004
  - Serial: yes

- [x] CMUX-006: existing mapping selection policy
  - What: implement explicit-first resolution and ambiguity fallback for mapped targets and update
    mapping metadata on successful selection.
  - Specs:
    - `docs/spec/commands/ws/open.md`
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-002, CMUX-003
  - Serial: yes

- [x] CMUX-007: mapping visibility/health foundation
  - What: provide mapping visibility and runtime health evaluation in integration flow internals.
  - Specs:
    - `docs/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-002, CMUX-003
  - Parallel: yes

- [x] CMUX-008: docs finalization + regression guard
  - What: finalize specs and add regression tests ensuring `kra ws open` integration behavior remains stable.
  - Specs:
    - `docs/spec/commands/ws/open.md`
    - `docs/spec/commands/ws/select.md`
  - Depends: CMUX-005, CMUX-006, CMUX-007
  - Serial: yes

- [x] CMUX-009: `kra ws open --multi` v2
  - What: add multi-target open flow for batch workspace provisioning.
    Supports multi-selection and explicit repeated `--workspace <id>` inputs.
  - Specs:
    - `docs/spec/commands/ws/open.md`
  - Depends: CMUX-005
  - Serial: yes

- [x] CMUX-010: `kra ws open --multi --concurrency <n>` v2
  - What: add bounded parallel execution with result aggregation and partial-failure reporting
    for batch open operations.
  - Specs:
    - `docs/spec/commands/ws/open.md`
  - Depends: CMUX-009
  - Serial: yes

- [x] CMUX-011: `kra ws open` entrypoint migration
  - What: make `ws open` the primary user-facing cmux integration action and support
    workspace targeting options (`--id` / `--current` / `--select`) at `ws` action level.
  - Specs:
    - `docs/spec/commands/ws/open.md`
    - `docs/spec/commands/ws/select.md`
  - Depends: CMUX-005, CMUX-006
  - Serial: yes

- [x] CMUX-012: remove dedicated `cmux` command group
  - What: remove top-level `cmux` CLI entrypoint and route user-facing cmux operations through
    `kra ws open`.
  - Specs:
    - `docs/spec/commands/ws/open.md`
    - `docs/spec/commands/ws/select.md`
  - Depends: CMUX-011
  - Serial: yes
