---
title: "INT-JIRA backlog"
status: planned
---

# INT-JIRA Backlog

- [x] INT-JIRA-001: `ws create --jira <ticket-url>` (single issue MVP)
  - What: add strict Jira single-ticket creation path in `ws create`; resolve `id=issueKey` and `title=summary`
    from Jira API using env-only auth, fail-fast on retrieval errors, and disallow `--jira` with `--id/--title`.
    Keep implementation split by architecture layers so future sprint bulk import can reuse app use cases.
  - Specs:
    - `docs/dev/spec/commands/ws/create.md`
    - `docs/dev/spec/testing/integration.md`
  - Depends: ARCH-010, UX-WS-028
  - Serial: yes

- [x] INT-JIRA-002: `ws import jira` MVP (plan/apply + JQL/sprint)
  - What: add `ws import jira` as workspace-creation subcommand (not `--act`) with plan-first flow.
    Support `--jql` and `--sprint <id|name>` inputs, default limit, skip/fail classification, best-effort apply,
    and non-zero exit when failed items exist.
  - Specs:
    - `docs/dev/spec/commands/ws/import/jira.md`
    - `docs/dev/spec/testing/integration.md`
  - Depends: INT-JIRA-001
  - Serial: yes

- [x] INT-JIRA-003: sprint selector flow (`--sprint` without value)
  - What: implement interactive board/sprint selection when `--sprint` is given without value.
    Show board selector only when multiple candidates exist, then sprint selector with `Active + Future` scope.
    In `--no-prompt`, require explicit `--sprint <id|name>` or `--jql`.
  - Specs:
    - `docs/dev/spec/commands/ws/import/jira.md`
    - `docs/dev/spec/testing/integration.md`
  - Depends: INT-JIRA-002
  - Serial: yes

- [x] INT-JIRA-004: JSON contract and integration hardening for import
  - What: harden `ws import jira --json` contract and integration tests.
    Cover skip/fail reason codes, apply/non-apply behavior, and exit-code contract (`failed > 0` => non-zero).
  - Specs:
    - `docs/dev/spec/commands/ws/import/jira.md`
    - `docs/dev/spec/testing/integration.md`
  - Depends: INT-JIRA-002
  - Serial: no (Parallel)

- [x] INT-JIRA-005: `ws import jira` human UI alignment (plan/result color + section layout)
  - What: align human-readable output with shared CLI UI rules:
    semantic color tokens, section/bullet layout consistency, and prompt phrasing parity with other plan/apply commands.
    Keep JSON behavior unchanged (`stdout` JSON only, prompts on `stderr`).
  - Specs:
    - `docs/dev/spec/commands/ws/import/jira.md`
    - `docs/dev/spec/concepts/ui-color.md`
    - `docs/dev/spec/testing/integration.md`
  - Depends: INT-JIRA-004
  - Serial: no (Parallel)

- [x] INT-JIRA-006: sprint selector UX convergence (`TTY selector + non-TTY fallback`)
  - What: for `ws import jira --sprint --space <key>` with omitted sprint value,
    use shared interactive selector in TTY environments and keep numbered prompt fallback in non-TTY contexts.
    Preserve existing JSON behavior and `--no-prompt` constraints.
  - Specs:
    - `docs/dev/spec/commands/ws/import/jira.md`
    - `docs/dev/spec/testing/integration.md`
  - Depends: INT-JIRA-003
  - Serial: no (Parallel)

- [x] INT-JIRA-007: import apply feedback refinement (`Plan inline confirm + Result summary`)
  - What: improve human UX for `ws import jira` by embedding the apply-confirm line directly in `Plan`
    and printing `Result` summary after apply. Keep JSON contract unchanged (`stdout` JSON only,
    prompt on `stderr`).
  - Specs:
    - `docs/dev/spec/commands/ws/import/jira.md`
    - `docs/dev/spec/testing/integration.md`
  - Depends: INT-JIRA-005
  - Serial: no (Parallel)
