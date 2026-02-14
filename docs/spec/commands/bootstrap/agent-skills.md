---
title: "`kra bootstrap agent-skills`"
status: planned
---

# `kra bootstrap agent-skills`

## Purpose

Bootstrap shared agent-skill references for the current `KRA_ROOT` with a safe, non-destructive policy.

This command standardizes where project-local skills live and how each agent runtime references them.

## Command forms

```sh
kra bootstrap agent-skills [--format human|json]
kra init [--root <path>] [--context <name>] --bootstrap agent-skills
```

## Scope

- This command is for bootstrap/setup only.
- It does not create or install concrete skill contents in the first phase.
- It only prepares structure and references.

## Root resolution

`kra bootstrap agent-skills`:

- always targets the root resolved from **current context**.
- does not accept `--root` or `--context`.

If current context is missing or invalid:

- fail with usage/runtime error and recovery hint.

## Init integration

`kra init` supports:

- `--bootstrap agent-skills` (single value only in first phase)

When present:

- run the same bootstrap flow against the initialized root.

## Filesystem contract

Source of truth path:

- `<KRA_ROOT>/.agent/skills/`

Reference paths (directory-level symlink):

- `<KRA_ROOT>/.codex/skills` -> `<KRA_ROOT>/.agent/skills`
- `<KRA_ROOT>/.claude/skills` -> `<KRA_ROOT>/.agent/skills`

First phase creates:

- empty `<KRA_ROOT>/.agent/skills/` directory if missing.

## Conflict policy (safe-first)

- If `<KRA_ROOT>/.codex/skills` or `<KRA_ROOT>/.claude/skills` already exists and is not the expected symlink:
  - do not overwrite
  - fail fast
  - print recovery hints (for example backup/rename then rerun)

No destructive mutation is allowed by default.

## Idempotency

Re-running bootstrap on an already-correct root should be success with no-op result.

## Output

### Human

- `Result:` summary with:
  - target root
  - created/linked/skipped counts
  - conflict paths (if any)
  - concrete recovery hints

### JSON

Shared output envelope:

- `ok`
- `action` = `bootstrap.agent-skills`
- `result`:
  - `root`
  - `created[]`
  - `linked[]`
  - `skipped[]`
  - `conflicts[]` (`path`, `reason`, `hint`)
- `error` on command-level failure

## Non-goals (first phase)

- auto-installing official skill packs
- per-agent custom mapping policy
- multi-target bootstrap in one call

