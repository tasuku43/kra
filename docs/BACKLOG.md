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

## FS-first migration (new critical path)

- [x] FS-STATE-001: FS=SoT migration spec baseline
  - What: define filesystem as source of truth and position index data as rebuildable.
  - Specs:
    - `docs/spec/concepts/fs-source-of-truth.md`
    - `docs/spec/README.md`
  - Depends: -
  - Serial: yes (foundation for migration items)

- [x] FS-STATE-002: `ws create` writes `.gionx.meta.json`
  - What: create workspace-local canonical metadata file with schema version and workspace core fields.
  - Specs:
    - `docs/spec/concepts/workspace-meta-json.md`
    - `docs/spec/commands/ws/create.md`
  - Depends: FS-STATE-001
  - Serial: yes

- [x] FS-STATE-003: `ws add-repo` persists restore metadata in `.gionx.meta.json`
  - What: update `repos_restore` on successful bindings and keep alias/branch/base_ref deterministic.
  - Specs:
    - `docs/spec/concepts/workspace-meta-json.md`
    - `docs/spec/commands/ws/add-repo.md`
  - Depends: FS-STATE-002
  - Serial: yes

- [x] FS-STATE-004: `ws close` snapshots restore metadata from live worktrees
  - What: refresh `.gionx.meta.json.repos_restore` before removing worktrees and archiving directory.
  - Specs:
    - `docs/spec/concepts/workspace-meta-json.md`
    - `docs/spec/commands/ws/close.md`
  - Depends: FS-STATE-003
  - Serial: yes

- [x] FS-STATE-005: `ws reopen` restores from `.gionx.meta.json` (no DB binding dependency)
  - What: recreate worktrees exclusively from `repos_restore` and keep close/reopen reversible.
  - Specs:
    - `docs/spec/concepts/workspace-meta-json.md`
    - `docs/spec/commands/ws/reopen.md`
  - Depends: FS-STATE-004
  - Serial: yes

- [x] FS-STATE-006: logical work-state (`todo`/`in-progress`) in `ws list`/`ws go`
  - What: derive logical state at read time from filesystem + git signals, without DB persistence.
  - Specs:
    - `docs/spec/commands/ws/list.md`
    - `docs/spec/commands/ws/go.md`
    - `docs/spec/concepts/fs-source-of-truth.md`
  - Depends: FS-STATE-005
  - Parallel: yes

- [x] FS-STATE-007: `repo gc` safety gates read FS metadata + registry
  - What: evaluate references from workspace/archive metadata and known roots without relying on SQL joins.
  - Specs:
    - `docs/spec/commands/repo/gc.md`
    - `docs/spec/concepts/fs-source-of-truth.md`
  - Depends: FS-STATE-005
  - Parallel: yes (with FS-STATE-006)

- [x] FS-STATE-008: state-store downgrade/deprecation plan
  - What: redefine SQLite as optional/rebuildable index (or remove), and align docs/tests accordingly.
  - Specs:
    - `docs/spec/concepts/state-store.md`
    - `docs/spec/core/DATA_MODEL.md`
    - `docs/dev/TESTING.md`
  - Depends: FS-STATE-006, FS-STATE-007
  - Serial: yes (finalization step)

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

- [x] MVP-050: `gionx state` foundation (registry)
  - What: introduce registry metadata for root-scoped `state.db` discovery and hygiene workflows
  - Specs:
    - `docs/spec/commands/state/registry.md`
    - `docs/spec/concepts/state-store.md`
  - Depends: MVP-002, MVP-003, MVP-010
  - Parallel: yes (independent from ws archive lifecycle)

- [x] MVP-051: `gionx context` (root switch fallback)
  - What: add context management and root resolution fallback when `GIONX_ROOT` is unset
  - Specs:
    - `docs/spec/commands/context.md`
    - `docs/spec/commands/state/registry.md`
  - Depends: MVP-050
  - Parallel: yes

- [x] MVP-060: `gionx repo add` (shared pool upsert)
  - What: add root-scoped repo registration command that upserts shared bare pool and current root `repos` rows
    from repo specs (best-effort, conflict-safe).
  - Specs:
    - `docs/spec/commands/repo/add.md`
    - `docs/spec/concepts/state-store.md`
  - Depends: MVP-051
  - Parallel: yes

- [x] MVP-061: `gionx repo discover` (provider-based bulk add)
  - What: add provider adapter flow (`--provider` default github) to discover org repos, exclude current-root
    registered repos, and bulk add selected repos through `repo add` path.
  - Specs:
    - `docs/spec/commands/repo/discover.md`
    - `docs/spec/commands/repo/add.md`
  - Depends: MVP-060
  - Parallel: yes

- [x] MVP-062: `gionx repo remove` (root-local logical detach)
  - What: remove selected repos from current root `repos` registration using selector/direct mode.
    Keep physical bare repos untouched, and fail fast when selected repos are still referenced by
    `workspace_repos`.
  - Specs:
    - `docs/spec/commands/repo/remove.md`
    - `docs/spec/concepts/state-store.md`
  - Depends: MVP-060, MVP-061
  - Parallel: yes

- [x] MVP-063: `gionx repo gc` (safe physical pool cleanup)
  - What: garbage-collect bare repos from shared pool only when safety gates pass.
  - Specs:
    - `docs/spec/commands/repo/gc.md`
    - `docs/spec/commands/state/registry.md`
  - Depends: MVP-062
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
    - `docs/spec/commands/ws/select.md`
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

- [x] UX-WS-010: Risk presentation policy (color-only)
  - What: enforce color-only risk hints across ws selector/list surfaces (no textual risk tags in summary rows).
  - Specs:
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/list.md`
  - Depends: UX-WS-001
  - Parallel: yes

- [x] UX-WS-011: Confirmation policy consistency
  - What: keep `ws close` confirmation only when non-clean risk exists, and keep `ws purge` always-confirm policy
    explicit in specs/tests.
  - Specs:
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/purge.md`
    - `docs/spec/commands/ws/selector.md`
  - Depends: UX-WS-002, UX-WS-005
  - Parallel: yes

- [x] UX-WS-012: Selector keybind extensions (phase 1)
  - What: evaluate selector keybind extensions; `a`/`A` were explicitly rejected to preserve always-on filter input.
  - Specs:
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/commands/ws/selector.md`
  - Depends: UX-WS-001
  - Parallel: yes

- [x] UX-WS-013: Section indentation consistency (Workspaces/Risk/Result)
  - What: enforce shared indentation rules for selector footer/help and risk/confirm prompts so section body lines
    consistently align under each heading.
  - Specs:
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/purge.md`
    - `docs/spec/commands/ws/close.md`
  - Depends: UX-WS-001, UX-WS-005
  - Parallel: yes

- [x] UX-WS-014: Semantic color token baseline
  - What: define and apply a shared semantic token set for CLI/TUI output (`text.*`, `status.*`, `accent`,
    `focus`, `selection`, `diff.*`) and align selector/progress/result rendering with those tokens.
  - Specs:
    - `docs/spec/concepts/ui-color.md`
    - `docs/spec/commands/ws/selector.md`
  - Depends: UX-WS-001
  - Parallel: yes

- [x] UX-WS-015: Semantic color guardrail enforcement
  - What: add a CI/lint gate that blocks raw ANSI or ad-hoc color usage outside shared token paths,
    and document the rule in AGENTS/testing docs.
  - Specs:
    - `docs/spec/concepts/ui-color.md`
    - `docs/dev/TESTING.md`
  - Depends: UX-WS-014
  - Parallel: yes

- [x] UX-WS-016: Section spacing consistency (Risk newline parity)
  - What: enforce heading spacing parity across ws flows so `Risk:` keeps one blank line after heading
    (`Workspaces/Risk` one blank, `Result` no blank).
  - Specs:
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/purge.md`
  - Depends: UX-WS-013
  - Parallel: yes

- [x] UX-WS-017: `ws list` archived color semantic alignment
  - What: align `ws list` archived color description to shared semantic token rule (`text.muted`).
  - Specs:
    - `docs/spec/commands/ws/list.md`
    - `docs/spec/concepts/ui-color.md`
  - Depends: UX-WS-014
  - Parallel: yes

- [x] UX-REPO-001: Result color semantics parity (`repo remove`)
  - What: apply shared `Result:` summary color semantics to `repo remove` for consistency with `repo add/gc`.
  - Specs:
    - `docs/spec/commands/repo/remove.md`
    - `docs/spec/concepts/ui-color.md`
  - Depends: UX-WS-014
  - Parallel: yes

- [x] UX-REPO-002: `repo gc` summary condition cleanup
  - What: use consistent denominator (`eligibleSelected`) for summary color condition to avoid future drift.
  - Specs:
    - `docs/spec/commands/repo/gc.md`
  - Depends: MVP-063
  - Parallel: yes

- [x] UX-CORE-001: Core command `Result:` output parity (`init`/`ws create`/`context use`)
  - What: align human-readable success outputs of core commands to shared section flow and semantic color tokens.
  - Specs:
    - `docs/spec/commands/init.md`
    - `docs/spec/commands/ws/create.md`
    - `docs/spec/commands/context.md`
    - `docs/spec/concepts/ui-color.md`
  - Depends: UX-WS-014
  - Parallel: yes

- [x] UX-WS-018: `ws add-repo` inputs tree stability polish
  - What: finalize input-tree connector transitions and section spacing consistency while keeping editable defaults.
  - Specs:
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/commands/ws/selector.md`
  - Depends: MVP-031, UX-WS-013
  - Parallel: yes

- [x] UX-WS-019: `ws go` single-select UI mode
  - What: switch `ws go` selector to cursor-confirm single-select mode (no checkbox markers, no selected summary).
  - Specs:
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/go.md`
  - Depends: UX-WS-001, UX-WS-003
  - Parallel: yes

- [x] UX-CORE-002: Shell integration bootstrap (`gionx shell init`)
  - What: add shell integration command that prints eval-ready wrapper function and align `ws go` default output
    to shell snippet mode.
  - Specs:
    - `docs/spec/commands/shell/init.md`
    - `docs/spec/commands/ws/go.md`
  - Depends: UX-WS-003
  - Parallel: yes

## WS Unified Entry (idea competition agreed set: 2026-02-08)

- [ ] UX-WS-020: Dual-entry architecture spec (`human` interactive vs `agent` non-interactive)
  - What: define facade split while keeping a single execution core for workspace actions.
    Clarify role boundaries: humans use unified launcher flow, agents use operation-fixed commands.
  - Specs:
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/go.md`
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/README.md`
  - Depends: UX-WS-019, UX-CORE-002
  - Serial: yes (foundation)

- [ ] UX-WS-021: `ws select` unified launcher flow (single-select)
  - What: add `gionx ws select` as canonical human entrypoint:
    choose one workspace, then choose one action (`go` / `close` / `add-repo`) and delegate to existing flows.
  - Specs:
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/go.md`
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/add-repo.md`
  - Depends: UX-WS-020
  - Serial: yes

- [ ] UX-WS-022: Context-aware `ws` launcher behavior
  - What: make `gionx ws` context-aware:
    outside workspace -> behave as `ws select`;
    inside workspace -> skip workspace selection and open action menu for current workspace.
  - Specs:
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/go.md`
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/add-repo.md`
  - Depends: UX-WS-021
  - Serial: yes

- [ ] UX-WS-023: In-workspace action menu policy (`add-repo` first, then `close`, no `go`)
  - What: when current workspace is auto-selected, present action choices in fixed order:
    `add-repo` -> `close`; exclude `go`.
  - Specs:
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/go.md`
  - Depends: UX-WS-022
  - Serial: yes

- [ ] UX-WS-024: `ws list` alias (`ws ls`)
  - What: add `ws ls` as alias of `ws list` while keeping `ws list` read-only semantics unchanged.
  - Specs:
    - `docs/spec/commands/ws/list.md`
  - Depends: UX-WS-020
  - Parallel: yes

- [ ] UX-WS-025: Shell action protocol (`action file`) for post-exec parent-shell effects
  - What: evolve shell integration from pre-arg routing to post-exec action protocol so
    command-internal branching can still trigger safe `cd` in parent shell.
  - Specs:
    - `docs/spec/commands/shell/init.md`
    - `docs/spec/commands/ws/go.md`
  - Depends: UX-WS-022
  - Parallel: yes

- [ ] UX-WS-026: AI entrypoint contract for operation-fixed commands
  - What: define strict non-interactive contract for agent-facing commands:
    required explicit target input, no prompt fallback, stable JSON output, explicit exit-code mapping.
  - Specs:
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/go.md`
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/testing/integration.md`
  - Depends: UX-WS-020
  - Parallel: yes
