---
title: "UX-CORE backlog"
status: planned
---

# UX-CORE Backlog

- [x] UX-CORE-001: Core command `Result:` output parity (`init`/`ws create`/`context use`)
  - What: align human-readable success outputs of core commands to shared section flow and semantic color tokens.
  - Specs:
    - `docs/spec/commands/init.md`
    - `docs/spec/commands/ws/create.md`
    - `docs/spec/commands/context.md`
    - `docs/spec/concepts/ui-color.md`
  - Depends: UX-WS-014
  - Parallel: yes

- [x] UX-CORE-002: Shell integration bootstrap (`gionx shell init`)
  - What: add shell integration command that prints eval-ready wrapper function and align `ws go` default output
    to shell snippet mode.
  - Specs:
    - `docs/spec/commands/shell/init.md`
    - `docs/spec/commands/ws/go.md`
  - Depends: UX-WS-003
  - Parallel: yes

- [x] UX-CORE-003: Selector UX spec (`single`/`multi` shared contract)
  - What: define shared selector rules across commands (same marker, keymap, confirm timing, no-color behavior).
  - Specs:
    - `docs/spec/concepts/ui-selector.md`
    - `docs/spec/concepts/ui-color.md`
  - Depends: none
  - Parallel: no

- [x] UX-CORE-004: Extract reusable selector component for `ws`/`context`/`repo`
  - What: move selector runtime/rendering to reusable component and remove command-specific duplication.
  - Specs:
    - `docs/spec/concepts/ui-selector.md`
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/context.md`
    - `docs/spec/commands/repo/discover.md`
    - `docs/spec/commands/repo/remove.md`
    - `docs/spec/commands/repo/gc.md`
  - Depends: UX-CORE-003
  - Parallel: no

- [x] UX-CORE-005: Reduced motion toggle for selector confirm transition
  - What: add opt-out for confirm delay animation (e.g. `GIONX_REDUCED_MOTION=1`) while preserving behavior parity.
  - Specs:
    - `docs/spec/concepts/ui-selector.md`
    - `docs/spec/concepts/ui-color.md`
  - Depends: UX-CORE-003
  - Parallel: yes

- [x] UX-CORE-006: Context management extensions (`rename` / `remove`)
  - What: add `context rename` and `context rm` with current-context safety checks and clear error UX.
  - Specs:
    - `docs/spec/commands/context.md`
    - `docs/spec/commands/state/registry.md`
  - Depends: UX-CORE-004
  - Parallel: yes

- [x] UX-CORE-007: Terminology consistency (`repo` vs `worktree` vs `context`)
  - What: standardize user-facing wording and update command outputs/tests to one glossary.
  - Specs:
    - `docs/spec/concepts/ui-terms.md`
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/commands/context.md`
    - `docs/spec/commands/repo/discover.md`
  - Depends: UX-CORE-003
  - Parallel: yes

- [x] UX-CORE-008: Golden tests for core interactive screens
  - What: snapshot/golden tests for key interactive outputs (`ws launcher`, `ws add-repo`, `context use`).
  - Specs:
    - `docs/dev/TESTING.md`
    - `docs/spec/concepts/ui-selector.md`
  - Depends: UX-CORE-004
  - Parallel: yes

- [x] UX-CORE-009: Backlog-spec sync guardrail
  - What: formalize maintenance checklist so spec status/backlog status drift is caught early in development flow.
  - Specs:
    - `docs/backlog/README.md`
    - `docs/dev/TESTING.md`
  - Depends: none
  - Parallel: yes

- [x] UX-CORE-010: Single-select confirm contrast cue
  - What: during single-select confirm delay, dim non-selected rows with `text.muted` so final selection is clearer.
  - Specs:
    - `docs/spec/concepts/ui-selector.md`
    - `docs/spec/concepts/ui-color.md`
  - Depends: UX-CORE-003
  - Parallel: yes

- [x] UX-CORE-011: Shared section block spacing atom
  - What: unify section rendering (`Workspaces`, `Repos(pool)`, `Inputs`, `Plan`, `Result`, etc.) by reusing one
    section-block atom so each section ends with exactly one trailing blank line.
  - Specs:
    - `docs/spec/commands/ws/selector.md`
    - `docs/spec/commands/ws/add-repo.md`
    - `docs/spec/commands/ws/list.md`
  - Depends: UX-CORE-004
  - Parallel: yes
