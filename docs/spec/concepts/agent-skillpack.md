---
title: "Agent Skillpack (project-local, non-intrusive)"
status: implemented
---

# Agent Skillpack

## Purpose

Define a tool-provided, project-local skillpack model that improves traceability
without prescribing how users execute their actual work.

## Design principle

- Skills must not prescribe work style.
- Baseline skills should focus on traceability and reusable output capture only.
- Users should not need to author all base skills manually.
- `kra` should provide maintainable default skillpacks for effective usage.

## Scope

This concept covers:

- project-local skill source of truth:
  - `<KRA_ROOT>/.agent/skills/`
- tool-managed base skillpack contents
- guidance synchronization in `KRA_ROOT/AGENTS.md`

## Availability (v1)

Default users should not receive this skillpack automatically.

- enable seeding explicitly:
  - `KRA_EXPERIMENTS=agent-skillpack`
- without flag, bootstrap keeps `.agent/skills` structure only.

## Baseline skillpack (v1)

Bootstrap installs a default pack under:

- `KRA_ROOT/.agent/skills/.kra-skillpack.yaml`
- `KRA_ROOT/.agent/skills/flow-insight-capture/SKILL.md`

Included baseline pattern:

- insight capture proposal flow only

This skillpack is intentionally non-intrusive and does not define investigation/execution playbooks.

## AGENTS.md relation

`KRA_ROOT/AGENTS.md` should explicitly guide agents to:

- use project-local skills under `.agent/skills`
- keep skill usage non-intrusive to work style
- store reusable insights into workspace-local worklog paths
- propose insight capture in conversation and persist only after explicit approval

## Versioning

Skillpacks include explicit version metadata (`.kra-skillpack.yaml`) to allow:

- upgrade visibility
- compatibility checks
- controlled migration in existing roots

## Non-goals (v1)

- enforcing one fixed skill implementation for all model providers
- opinionated domain playbooks
- remote registry dependency for baseline operation
- forced overwrite of existing skill files
