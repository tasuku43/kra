---
title: "INT-CMUX backlog"
status: planned
---

# INT-CMUX Backlog

- [x] CMUX-001: `kra cmux` command skeleton
  - What: add `kra cmux` command group and usage routing (`open`/`switch`/`list`/`status`).
  - Specs:
    - `docs/spec/commands/cmux/README.md`
  - Depends: none
  - Serial: yes

- [x] CMUX-002: cmux mapping state store (1:N)
  - What: add persistent mapping store at `.kra/state/cmux-workspaces.json` with schema versioning,
    per-workspace `next_ordinal`, and `entries[]` lifecycle fields.
  - Specs:
    - `docs/spec/concepts/cmux-mapping.md`
    - `docs/spec/commands/cmux/README.md`
  - Depends: CMUX-001
  - Serial: yes

- [x] CMUX-003: cmux adapter for workspace control primitives
  - What: add adapter layer for cmux CLI/socket interaction used by integration flows:
    capabilities/identify/workspace create/rename/select/list and `surface.send_text` for cwd sync.
  - Specs:
    - `docs/spec/commands/cmux/README.md`
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

- [x] CMUX-005: `kra cmux open` v1
  - What: implement strict open flow:
    resolve kra workspace -> create cmux workspace -> rename -> select -> cwd sync (`cd <path>` send)
    -> persist mapping update.
    If `<kra-ws-id>` is omitted, fallback to interactive workspace selection in TTY.
  - Specs:
    - `docs/spec/commands/cmux/open.md`
    - `docs/spec/commands/cmux/README.md`
    - `docs/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-004
  - Serial: yes

- [ ] CMUX-006: `kra cmux switch` v1
  - What: implement explicit-first resolution and ambiguity fallback:
    `--workspace`/`--cmux` combinations, two-stage selector fallback, non-TTY strict errors,
    and `last_used_at` update on successful switch.
  - Specs:
    - `docs/spec/commands/cmux/switch.md`
    - `docs/spec/commands/cmux/README.md`
    - `docs/spec/concepts/cmux-mapping.md`
  - Depends: CMUX-002, CMUX-003
  - Serial: yes

- [ ] CMUX-007: `kra cmux list` / `kra cmux status` v1
  - What: provide visibility commands for mapping entries and health state in human/json formats,
    including `--workspace` filtering.
  - Specs:
    - `docs/spec/commands/cmux/list.md`
    - `docs/spec/commands/cmux/status.md`
    - `docs/spec/commands/cmux/README.md`
  - Depends: CMUX-002, CMUX-003
  - Parallel: yes

- [ ] CMUX-008: docs finalization + regression guard
  - What: finalize spec status and add regression tests ensuring existing `kra ws --act go`
    behavior/contract remains unchanged.
  - Specs:
    - `docs/spec/commands/cmux/README.md`
    - `docs/spec/commands/cmux/open.md`
    - `docs/spec/commands/cmux/switch.md`
    - `docs/spec/commands/cmux/list.md`
    - `docs/spec/commands/cmux/status.md`
    - `docs/spec/commands/ws/go.md`
  - Depends: CMUX-005, CMUX-006, CMUX-007
  - Serial: yes

- [ ] CMUX-009: `kra cmux open --multi` v2
  - What: add multi-target open flow for batch workspace provisioning.
    Supports multi-selection and explicit repeated `--workspace <id>` inputs.
  - Specs:
    - `docs/spec/commands/cmux/open-multi.md`
    - `docs/spec/commands/cmux/README.md`
  - Depends: CMUX-005
  - Serial: yes

- [ ] CMUX-010: `kra cmux open --multi --concurrency <n>` v2
  - What: add bounded parallel execution with result aggregation and partial-failure reporting
    for batch open operations.
  - Specs:
    - `docs/spec/commands/cmux/open-multi.md`
    - `docs/spec/commands/cmux/README.md`
  - Depends: CMUX-009
  - Serial: yes
