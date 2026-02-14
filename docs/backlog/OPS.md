---
title: "OPS backlog"
status: planned
---

# OPS Backlog

- [x] OPS-001: `kra doctor` baseline (state/fs/worktree health checks)
  - What: add a non-destructive health check command to detect common operational drifts before running workspace actions.
  - Specs:
    - `docs/spec/commands/doctor.md`
    - `docs/spec/concepts/state-store.md`
    - `docs/spec/concepts/layout.md`
  - Depends: none
  - Serial: yes

- [x] OPS-002: Workspace multi-select action (`ws select --multi`)
  - What: extend existing selector to support multi-selection with explicit action binding.
  - Specs:
    - `docs/spec/commands/ws/select-multi.md`
    - `docs/spec/commands/ws/select.md`
    - `docs/spec/concepts/workspace-lifecycle.md`
  - Depends: OPS-001
  - Serial: yes

- [x] OPS-003: Machine-readable output parity across major commands
  - What: align `--format json` contracts across core commands so automation can rely on common envelope and error semantics.
  - Specs:
    - `docs/spec/concepts/output-contract.md`
    - `docs/spec/commands/init.md`
    - `docs/spec/commands/context.md`
    - `docs/spec/commands/repo/add.md`
    - `docs/spec/commands/repo/remove.md`
    - `docs/spec/commands/repo/gc.md`
    - `docs/spec/commands/ws/list.md`
  - Depends: none
  - Parallel: yes

- [x] OPS-004: `kra doctor --fix` staged remediation (`--plan` / `--apply`)
  - What: extend `doctor` from detection-only to staged remediation with explicit plan/apply modes and stable action report.
  - Specs:
    - `docs/spec/commands/doctor.md`
    - `docs/spec/commands/doctor-fix.md`
    - `docs/spec/concepts/output-contract.md`
  - Depends: OPS-001, OPS-003
  - Serial: yes

- [x] OPS-005: Workspace lifecycle dry-run JSON parity (`close`/`reopen`/`purge`)
  - What: add `--dry-run --format json` preflight contract for lifecycle actions with unified checks/risk/planned-effects envelope.
  - Specs:
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/reopen.md`
    - `docs/spec/commands/ws/purge.md`
    - `docs/spec/commands/ws/dry-run.md`
    - `docs/spec/concepts/output-contract.md`
  - Depends: OPS-003
  - Parallel: yes

- [x] OPS-006: Workspace lock/unlock safety gate (`ws lock` / `ws unlock`)
  - What: add purge-guard metadata (`.kra.meta.json`) and enforce lock-aware conflict guard for `ws purge` only.
    Guard defaults to enabled on `ws create`, persists across `close/reopen`, and archived launcher exposes `unlock`.
  - Specs:
    - `docs/spec/commands/ws/lock.md`
    - `docs/spec/commands/ws/close.md`
    - `docs/spec/commands/ws/purge.md`
    - `docs/spec/concepts/workspace-lifecycle.md`
  - Depends: none
  - Parallel: yes

- [x] OPS-007: Branch naming policy templates for `ws add-repo`
  - What: introduce config-driven branch template rendering with deterministic placeholders and validation.
  - Specs:
    - `docs/spec/concepts/branch-naming-policy.md`
    - `docs/spec/concepts/config.md`
    - `docs/spec/commands/ws/add-repo.md`
  - Depends: CONFIG-002, MVP-031
  - Parallel: yes

- [x] OPS-008: `kra ws dashboard` operational overview
  - What: add one-screen read-only dashboard combining workspace risk/context/agent signals with JSON contract.
  - Specs:
    - `docs/spec/commands/ws/dashboard.md`
    - `docs/spec/commands/agent/activity.md`
    - `docs/spec/concepts/output-contract.md`
  - Depends: AGENT-001, OPS-003
  - Parallel: yes

- [x] OPS-009: `kra bootstrap agent-skills` foundation + `init` integration
  - What: add safe bootstrap flow for project-local agent skills:
    source of truth under `.agent/skills` and directory-level symlink references from `.codex/skills` and `.claude/skills`.
    Integrate via `kra init --bootstrap agent-skills`.
  - Specs:
    - `docs/spec/commands/bootstrap/agent-skills.md`
  - Depends: none
  - Serial: yes

- [ ] OPS-010: Tool-provided flow skillpack + AGENTS guidance optimization
  - What: provide official flow-oriented skillpack (not domain-specific playbooks) and align root AGENTS guidance
    so operators can use project-local skills effectively without manual setup burden.
  - Specs:
    - `docs/spec/concepts/agent-skillpack.md`
    - `docs/spec/core/AGENTS.md`
  - Depends: OPS-009
  - Parallel: yes

- [x] OPS-011: Workspace insight capture (important-only, conversational proposal)
  - What: introduce workspace-local insight capture contract:
    no always-on logging, propose capture in conversation, persist only approved insight documents
    under `worklog/insights` using markdown + frontmatter.
  - Specs:
    - `docs/spec/concepts/worklog-insight.md`
    - `docs/spec/commands/ws/insight.md`
  - Depends: OPS-009
  - Serial: yes
