---
title: "gionx backlog"
status: planned
---

# Backlog

This file is the implementation backlog for `gionx`.

## Definition of done (per backlog item)

Do not treat an item as "done" until all of the following are true:

- Code exists and behavior matches the linked specs.
- Tests exist, including at least some non-happy-path coverage (see `docs/dev/TESTING.md`).
- The linked spec frontmatter is updated to `status: implemented`.

Special note (commands that commit inside `GIONX_ROOT`):
- The spec must define the staging allowlist (which path prefixes may be staged/committed).
- The implementation must enforce it (stage allowlist only, verify staged paths are within allowlist, abort otherwise).

## How to pick the next item (dynamic)

Avoid baking "next up" decisions into this file. Instead, decide per session:

1) If the working tree is dirty, finish that slice first (or explicitly park it with a WIP commit).
2) Prefer the smallest-numbered **Serial** item whose dependencies are satisfied.
3) If you want parallel work, pick a **Parallel** item that is unblocked by dependencies.

Tip:
- An item is "done" only when its linked specs are updated to `status: implemented` and the code/tests exist.

## Reporting next steps (required)

When you finish a backlog item, include a short next-steps report in your final output:

- Next Serial candidate (smallest-numbered Serial whose deps are satisfied).
- Parallel candidates (unblocked items).
- Conditional guidance: "If <deps> are done, then <item> is ready."

Recommended (workspace handoff):
- Pre-stage the next ticket in the manifest, preview, then apply only after agreement:

```sh
gion manifest add --no-apply --repo git@github.com:tasuku43/gionx.git MVP-001
gion plan
gion apply --no-prompt
```

Note: `MVP-001` is **an example** ticket/backlog id. Replace it with the **next** ticket you intend to start.

Rules:
- Each backlog item maps to one or more spec files in `docs/spec/**`.
- Dependencies are explicit so we can see what must be serial vs what can be parallel.
- When an item is complete, update the related spec file frontmatter to `status: implemented`.

Legend:
- **Serial**: on the critical path (blocks other items).
- **Parallel**: can be implemented independently once its dependencies are done.

## Parallelizable groups (guide)

This is a quick guide for what can be worked on in parallel once prerequisites are met.
It does not replace per-item dependencies.

- After MVP-001:
  - MVP-002 and MVP-003 can proceed in parallel.
- After MVP-002 + MVP-003:
  - MVP-010 can proceed (usually best to do early).
  - MVP-030 can proceed in parallel (repo pool is used later by add-repo/reopen).
- After MVP-010:
  - MVP-020 and MVP-021 can proceed in parallel.
- After MVP-020 + MVP-030 + MVP-010:
  - MVP-031 unblocks the archive lifecycle commands (MVP-040/041/042).

## Foundation (critical path)

- [x] MVP-001: Project skeleton (CLI entrypoint + subcommand routing)
  - Specs: `docs/spec/README.md`
  - Depends: -
  - Parallel: with MVP-002/MVP-003 if different owners; otherwise do serial

- [x] MVP-002: Path resolution and root detection
  - What: resolve `GIONX_ROOT`, XDG paths for `state.db` and repo pool
  - Specs:
    - `docs/spec/concepts/state-store.md`
    - `docs/spec/concepts/layout.md`
  - Depends: MVP-001
  - Parallel: yes (with MVP-003)

- [x] MVP-003: SQLite state store + migrations runner
  - What: open DB, `PRAGMA foreign_keys=ON`, apply `migrations/*.sql`, track `schema_migrations`
  - Specs:
    - `docs/spec/concepts/state-store.md`
    - `docs/spec/core/DATA_MODEL.md`
    - `migrations/0001_init.sql`
  - Depends: MVP-001
  - Parallel: yes (with MVP-002)

## Init

- [x] MVP-010: `gionx init`
  - What: ensure layout dirs, create root `AGENTS.md`, create `.gitignore`, run `git init` if needed,
    initialize state store settings
  - Specs:
    - `docs/spec/commands/init.md`
    - `docs/spec/core/AGENTS.md`
    - `docs/spec/concepts/layout.md`
    - `docs/spec/concepts/state-store.md`
  - Depends: MVP-002, MVP-003
  - Serial: yes (blocks most commands)

## Workspace core (no Git worktrees yet)

- [x] MVP-020: `gionx ws create`
  - What: create workspace scaffolding + insert workspace snapshot + append `created` event
  - Specs:
    - `docs/spec/commands/ws/create.md`
    - `docs/spec/concepts/layout.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-010
  - Parallel: yes (with MVP-021 once MVP-010 done)

- [x] MVP-021: `gionx ws list` (without repo risk at first)
  - What: list snapshot + basic drift import/mark-missing behavior
  - Specs:
    - `docs/spec/commands/ws/list.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-010
  - Parallel: yes

## Repo pool + worktrees

- [x] MVP-030: Repo pool (bare clone store) access
  - What: ensure bare repo for `repo_uid`, fetch, default branch detection
  - Specs:
    - `docs/spec/concepts/state-store.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-002, MVP-003
  - Parallel: yes (can start before MVP-010; used by add-repo/reopen)

- [x] MVP-031: `gionx ws add-repo`
  - What: normalize repo spec, derive alias, prefetch, branch/base_ref prompt, create worktree,
    record `workspace_repos`
  - Specs:
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/concepts/layout.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-010, MVP-030, MVP-020
  - Serial: yes (blocks close/reopen for real usage)

## Archive lifecycle (Git-managed root)

- [x] MVP-040: `gionx ws close`
  - What: risk inspection (live), delete worktrees, atomic rename to `archive/`, commit touched paths,
    update snapshot + append event
  - Specs:
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/concepts/layout.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-031
  - Serial: yes

- [x] MVP-041: `gionx ws reopen`
  - What: atomic rename back, recreate worktrees, commit touched paths, update snapshot + append event
  - Specs:
    - `docs/spec/commands/ws/reopen.md`
    - `docs/spec/concepts/layout.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-040
  - Serial: yes

- [x] MVP-042: `gionx ws purge`
  - What: confirmations, delete dirs, remove snapshot row, append event, commit touched paths
  - Specs:
    - `docs/spec/commands/ws/purge.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-040
  - Parallel: with MVP-041 if we want (but usually serial to validate lifecycle first)

## Hardening / tests

- [ ] MVP-050: `gionx state` foundation (registry)
  - What: introduce registry metadata for root-scoped `state.db` discovery and hygiene workflows
  - Specs:
    - `docs/spec/commands/state/registry.md`
    - `docs/spec/concepts/state-store.md`
  - Depends: MVP-002, MVP-003, MVP-010
  - Parallel: yes (independent from ws archive lifecycle)

- [ ] MVP-051: `gionx context` (root switch fallback)
  - What: add context management and root resolution fallback when `GIONX_ROOT` is unset
  - Specs:
    - `docs/spec/commands/context.md`
    - `docs/spec/commands/state/registry.md`
  - Depends: MVP-050
  - Parallel: yes

- [x] MVP-900: Test harness + non-happy-path coverage baseline
  - What: temp `GIONX_ROOT`, isolated sqlite file per test, drift scenario tests
  - Specs:
    - `docs/dev/TESTING.md`
  - Depends: MVP-003
  - Parallel: continuous (start early; extend per command)

- [x] MVP-901: Integration tests expansion (through MVP-042)
  - What: expand CLI-level tests to cover drift/partial-failure scenarios through the full archive lifecycle
  - Specs:
    - `docs/spec/testing/integration.md`
  - Depends: MVP-042
  - Parallel: yes (recommended once lifecycle commands land)

## WS UX Polish (status-managed rollout)

- [x] UX-WS-001: Lifecycle concept + selector architecture spec
  - What: define canonical state transitions (`active -> archived -> purged`) and shared non-fullscreen selector UI.
    Also clarify `ws list` as read-only summary command (`--tree` as opt-in detail).
  - Specs:
    - `docs/spec/concepts/workspace-lifecycle.md`
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/core/DATA_MODEL.md`
  - Depends: MVP-042
  - Serial: yes (foundation for all ws UX changes)

- [x] UX-WS-002: `gionx ws close` selector-mode + bulk safety gate
  - What: keep direct mode (`ws close <id>`) and add selector mode (`ws close`) with multi-select and all-or-nothing
    risk gate; enforce strict allowlist + gitignore abort policy for archive commits.
  - Specs:
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/selector.md`
  - Depends: UX-WS-001, MVP-040
  - Serial: yes

- [x] UX-WS-003: `gionx ws go` command (start-work flow)
  - What: add `ws go` with selector/direct mode, default `active` scope, optional `--archived`, and `--emit-cd`
    output for shell function integration.
  - Specs:
    - `docs/spec/commands/ws/go.md`
    - `docs/spec/commands/ws/selector.md`
  - Depends: UX-WS-001, MVP-020
  - Parallel: yes (with UX-WS-002)

- [x] UX-WS-004: `gionx ws reopen` selector-mode
  - What: keep direct mode (`ws reopen <id>`) and add selector mode (`ws reopen`) scoped to archived workspaces.
  - Specs:
    - `docs/spec/commands/ws/reopen.md`
    - `docs/spec/commands/ws/selector.md`
  - Depends: UX-WS-001, MVP-041
  - Parallel: yes

- [x] UX-WS-005: `gionx ws purge` selector-mode
  - What: keep direct mode (`ws purge <id>`) and add selector mode (`ws purge`) scoped to archived workspaces.
  - Specs:
    - `docs/spec/commands/ws/purge.md`
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/concepts/workspace-lifecycle.md`
  - Depends: UX-WS-001, MVP-042
  - Parallel: yes

- [x] UX-WS-006: `gionx ws list` selector-parity output
  - What: replace current TSV output with selector-parity non-interactive list UI (summary-first), and add
    optional expanded detail mode (`--tree`) using the same shared rendering hierarchy.
  - Specs:
    - `docs/spec/commands/ws/list.md`
    - `docs/spec/commands/ws/selector.md`
  - Depends: UX-WS-001, MVP-021
  - Serial: yes (prevent UI drift across ws commands)

- [x] UX-WS-007: WS close wording consistency (`close` vs `archived`)
  - What: unify user-facing wording to `close` (for command/action/result labels), while keeping internal lifecycle
    state naming as `archived`.
  - Specs:
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/selector.md`
  - Depends: UX-WS-002
  - Serial: yes

- [x] UX-WS-008: Selector footer readability and truncation rules
  - What: shorten key-hint text, define deterministic truncation behavior for narrow terminals, and keep
    `selected: n/m` consistently visible.
  - Specs:
    - `docs/spec/commands/ws/selector.md`
  - Depends: UX-WS-001
  - Parallel: yes

- [x] UX-WS-009: `ws list` row layout hardening
  - What: lock default summary row order to `ID | risk | repos | description` with stable column alignment and
    ellipsis policy.
  - Specs:
    - `docs/spec/commands/ws/list.md`
    - `docs/spec/commands/ws/selector.md`
  - Depends: UX-WS-006
  - Serial: yes

- [ ] UX-WS-010: Risk presentation policy (color-only)
  - What: enforce color-only risk hints across ws selector/list surfaces (no textual risk tags in summary rows).
  - Specs:
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/list.md`
  - Depends: UX-WS-001
  - Parallel: yes

- [ ] UX-WS-011: Confirmation policy consistency
  - What: keep `ws close` confirmation only when non-clean risk exists, and keep `ws purge` always-confirm policy
    explicit in specs/tests.
  - Specs:
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/purge.md`
    - `docs/spec/commands/ws/selector.md`
  - Depends: UX-WS-002, UX-WS-005
  - Parallel: yes

- [ ] UX-WS-012: Selector keybind extensions (phase 1)
  - What: add `a` select-all and `A` clear-all to selector-mode commands (filter input is baseline behavior).
  - Specs:
    - `docs/spec/commands/ws/selector.md`
  - Depends: UX-WS-001
  - Parallel: yes

- [ ] UX-WS-013: Section indentation consistency (Workspaces/Risk/Result)
  - What: enforce shared indentation rules for selector footer/help and risk/confirm prompts so section body lines
    consistently align under each heading.
  - Specs:
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/purge.md`
    - `docs/spec/commands/ws/close.md`
  - Depends: UX-WS-001, UX-WS-005
  - Parallel: yes
