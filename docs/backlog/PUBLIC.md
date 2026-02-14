---
title: "PUBLIC backlog"
status: implemented
---

# PUBLIC Backlog

Derived from `docs/public-readiness-gap-analysis.md`.
This backlog tracks release-readiness work item by item.
`agent` command GA work is out of scope here because agent capabilities remain EXP by design.

## P0: Must be completed before public release

- [x] PUBLIC-001: Public-facing README rewrite
  - What: rewrite `README.md` as an external entrypoint with installation, target users, core use-cases, and FAQ.
  - Specs:
    - `README.md`
    - `docs/START_HERE.md`
    - `docs/concepts/product-concept.md`
  - Depends: none
  - Serial: yes

- [x] PUBLIC-002: OSS governance docs minimum baseline
  - What: add minimum OSS operation docs required at release time (`CONTRIBUTING.md`, `SECURITY.md`).
  - Specs:
    - `CONTRIBUTING.md`
    - `SECURITY.md`
  - Depends: PUBLIC-001
  - Parallel: yes

- [x] PUBLIC-003: Build metadata injection (`version/commit/date`)
  - What: replace fixed `dev` version behavior with release-grade build metadata via ldflags.
  - Specs:
    - `cmd/kra/main.go`
    - `Taskfile.yml`
  - Depends: none
  - Parallel: yes

- [x] PUBLIC-004: Release/distribution workflow
  - What: add tag-driven release workflow that publishes binaries and checksums.
  - Specs:
    - `.github/workflows/`
    - `README.md`
  - Depends: PUBLIC-003
  - Serial: yes

## P1: High priority after release baseline

- [x] PUBLIC-011: Output contract parity audit completion
  - What: remove remaining output contract exceptions (including `ws import jira`) so automation can rely on a unified JSON envelope.
  - Specs:
    - `docs/spec/concepts/output-contract.md`
    - `docs/spec/commands/ws/import/jira.md`
  - Depends: OPS-003
  - Parallel: yes

- [x] PUBLIC-013: Non-interactive automation parity for `ws create`
  - What: strengthen automation-oriented mode for `ws create` to reduce TTY-only operation requirements.
  - Specs:
    - `docs/spec/commands/ws/create.md`
    - `docs/spec/concepts/output-contract.md`
  - Depends: OPS-003
  - Parallel: yes

## P2: Medium-term improvements

- [x] PUBLIC-022: `ws import jira` help consistency audit
  - What: resolve flag/help consistency drift (including `--board` wording mismatch) and lock behavior with tests.
  - Specs:
    - `docs/spec/commands/ws/import/jira.md`
  - Depends: INT-JIRA-007
  - Parallel: yes
