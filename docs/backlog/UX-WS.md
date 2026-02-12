---
title: "UX-WS backlog"
status: planned
---

# UX-WS Backlog

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
  - What: lock default summary row order to `ID | risk | repos | title` with stable column alignment and
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
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/purge.md`
  - Depends: UX-WS-013
  - Parallel: yes

- [x] UX-WS-017: `ws list` archived color semantic alignment
  - What: align `ws list` archived color title to shared semantic token rule (`text.muted`).
  - Specs:
    - `docs/spec/commands/ws/list.md`
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
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/go.md`
  - Depends: UX-WS-001, UX-WS-003
  - Parallel: yes

- [x] UX-WS-020: Dual-entry architecture spec (`human` interactive vs `agent` non-interactive)
  - What: define facade split while keeping a single execution core for workspace actions.
    Clarify role boundaries: humans use unified launcher flow, agents use operation-fixed commands.
  - Specs:
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/go.md`
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/README.md`
  - Depends: UX-WS-019, UX-CORE-002, ARCH-001
  - Serial: yes (foundation)

- [x] UX-WS-021: `ws` unified launcher flow (single-select)
  - What: implement human launcher with single-select workspace resolution and action selection.
    For operation commands, add shared `--select` option to select workspace first, then run command.
  - Specs:
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/go.md`
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/add-repo.md`
  - Depends: UX-WS-020
  - Serial: yes

- [x] UX-WS-022: Context-aware `ws` launcher behavior
  - What: make `gionx ws` context-aware:
    outside workspace -> select workspace first;
    inside workspace -> skip workspace selection and open action menu for current workspace.
  - Specs:
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/go.md`
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/add-repo.md`
  - Depends: UX-WS-021
  - Serial: yes

- [x] UX-WS-023: In-workspace action menu policy (`add-repo` first, then `close`, no `go`)
  - What: when current workspace is auto-selected, present action choices in fixed order:
    `add-repo` -> `close`; exclude `go`.
  - Specs:
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/go.md`
  - Depends: UX-WS-022
  - Serial: yes

- [x] UX-WS-024: `ws list` alias (`ws ls`)
  - What: add `ws ls` as alias of `ws list` while keeping `ws list` read-only semantics unchanged.
  - Specs:
    - `docs/spec/commands/ws/list.md`
  - Depends: UX-WS-020
  - Parallel: yes

- [x] UX-WS-025: Shell action protocol (`action file`) for post-exec parent-shell effects
  - What: evolve shell integration from pre-arg routing to post-exec action protocol so
    command-internal branching can still trigger safe `cd` in parent shell.
  - Specs:
    - `docs/spec/commands/shell/init.md`
    - `docs/spec/commands/ws/go.md`
  - Depends: UX-WS-022
  - Parallel: yes

- [x] UX-WS-026: AI entrypoint contract for operation-fixed commands
  - What: define strict non-interactive contract for agent-facing commands:
    required explicit target input, no prompt fallback, stable JSON output, explicit exit-code mapping.
  - Specs:
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/go.md`
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/testing/integration.md`
  - Depends: UX-WS-020
  - Parallel: yes

- [x] UX-WS-027: `ws select` as primary interactive selection entrypoint
  - What: consolidate interactive selection to `ws select` (workspace -> action; optional `--act` fixed action),
    keep `ws` as context-aware launcher, and deprecate operation-level `--select` flags in favor of explicit
    `<id>`/`--id` command paths.
  - Specs:
    - `docs/spec/commands/ws/list.md`
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/go.md`
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/commands/ws/reopen.md`
    - `docs/spec/commands/ws/purge.md`
  - Depends: UX-WS-022, UX-WS-023, UX-WS-026
  - Serial: yes

- [x] UX-WS-028: `ws select --act` and `--id` command normalization
  - What: add `ws select --act <...>` fixed-action mode, keep `ws` as context-aware launcher, and normalize
    explicit targeting via `--id` for operation commands while preserving cwd-based fallback where applicable.
  - Specs:
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/commands/ws/list.md`
    - `docs/spec/commands/ws/go.md`
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/add-repo.md`
  - Depends: UX-WS-027
  - Serial: yes

- [x] UX-WS-029: `ws --act remove-repo` (workspace binding + worktree removal)
  - What: add `remove-repo` as the operational counterpart of `add-repo`; remove selected workspace repo bindings
    and corresponding `workspaces/<id>/repos/<alias>` worktrees, while keeping repo pool entries untouched.
  - Specs:
    - `docs/spec/commands/ws/remove-repo.md`
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/add-repo.md`
  - Depends: UX-WS-028, UX-WS-026, MVP-031
  - Serial: yes

## Architecture Refactor (full layering migration)
