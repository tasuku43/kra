---
title: "Agent Skillpack (project-local, flow-oriented)"
status: planned
---

# Agent Skillpack

## Purpose

Define a tool-provided, project-local skillpack model that improves execution flow quality
without prescribing the user's business domain details.

## Design principle

- Skills should optimize flow (how to work), not domain content (what to conclude).
- Users should not need to author all base skills manually.
- `kra` should provide maintainable default skillpacks for effective usage.

## Scope

This concept covers:

- project-local skill source of truth:
  - `<KRA_ROOT>/.agent/skills/`
- tool-managed base skillpack contents (later phase)
- guidance synchronization in `KRA_ROOT/AGENTS.md`

## Baseline skillpack direction

Flow-oriented packs should include patterns such as:

- investigation flow
- execution/change flow
- evidence/summarization flow

These are process templates, not domain-specific rule bundles.

## AGENTS.md relation

`KRA_ROOT/AGENTS.md` should explicitly guide agents to:

- use project-local skills under `.agent/skills`
- prefer flow templates for consistency and traceability
- store reusable insights into workspace-local worklog paths

## Versioning

Tool-provided skillpacks should support explicit version metadata to allow:

- upgrade visibility
- compatibility checks
- controlled migration in existing roots

## Non-goals (first phase)

- enforcing one fixed skill implementation for all model providers
- opinionated domain playbooks
- remote registry dependency for baseline operation

