---
title: "Root workspace tasks"
status: implemented
---

# Root workspace tasks

## Purpose

Define root-level inbox and cross-workspace task state for the current `KRA_ROOT`.

## Scope

- Canonical root task document path is:
  - `<KRA_ROOT>/workspace.md`
- The file is optional.
- Absence of root `workspace.md` means the root inbox has no recorded current state and zero structured tasks.
- Root tasks are separate from workspace-local tasks stored under:
  - `<KRA_ROOT>/workspaces/<id>/workspace.md`
  - `<KRA_ROOT>/archive/<id>/workspace.md`

## Design goals

- Treat the root as one workspace for daily, unclassified, and cross-workspace work.
- Keep the same Markdown-first, AI-editable contract used by workspace-local `workspace.md`.
- Let humans and AI agents edit root `workspace.md` directly.
- Avoid making `kra ws status --all` responsible for root inbox state.

## File contract

Root `workspace.md` follows the existing workspace task contract:

- `kra` interprets only the first level-2 heading named `Tasks`.
- Structured tasks use level-3 headings:
  - `### TASK-<nnn> <title>`
- Each structured task block must contain:
  - `status: <todo|doing|blocked|done>`
- Freeform Markdown may exist before or after `## Tasks`.

Root task descriptions may include cross-workspace references using ordinary Markdown, for example:

```md
### TASK-002 Align auth behavior
status: doing

Related workspaces:
- AUTH-001
- UI-014
```

Structured `workspaces:` metadata is out of scope for the first root task slice.

## Command relation

- `kra task ...` operates on root `workspace.md`.
- `kra task` without a subcommand opens the root task status TUI, matching the single-workspace `kra ws status`
  interaction pattern.
- `kra task status` is the explicit root task status TUI command.
- `kra ws task ...` operates on one workspace-local `workspace.md`.
- `kra ws status --all` remains a workspace overview; it does not read root tasks.

## Initialization

`kra init` may leave root `workspace.md` absent. Root task commands create it lazily when needed.
