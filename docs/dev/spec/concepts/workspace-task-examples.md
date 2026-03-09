---
title: "Workspace task examples"
status: implemented
---

# Workspace task examples

## Purpose

Provide concrete `tasks.md` examples for humans and AI agents so direct editing remains compatible
with `kra`.

## Canonical mixed-content example

One `tasks.md` file may contain both freeform notes and structured tasks:

```md
# SREP-5000 work memo

Short freeform summary for the ticket.

## Plan

- confirm current production behavior
- decide rollback or fix-forward

## Tasks

### TASK-001 Confirm current ALB timeout value
status: done

Checked Terraform, AWS console, and the latest apply log.
Current timeout is `60s`.

### TASK-002 Reproduce timeout locally
status: doing

Need one local repro with the same upstream path and payload size.

### TASK-003 Ask platform team about safe timeout ceiling
status: blocked

Waiting for an answer in `#platform-help`.

## Notes

Anything here is freeform again and ignored by `kra` task parsing.
```

## Parse behavior for the example

- `kra` ignores `# SREP-5000 work memo` and the `## Plan` section.
- `kra` reads only the first `## Tasks` section.
- `TASK-001`, `TASK-002`, and `TASK-003` are structured task IDs.
- Each task title is derived from the level-3 heading remainder.
- Each task body starts after the required `status:` line and continues until the next task heading
  or the next level-2 heading.
- `## Notes` ends the structured task section.

## AI authoring guidance

When an AI agent edits `tasks.md` directly:

- preserve all Markdown outside `## Tasks`
- preserve stable task IDs
- preserve non-target task blocks unless intentionally changing them
- keep one required `status:` line per structured task
- write new tasks as new `### TASK-<nnn> <title>` blocks
- when cmux sidebar state matters, run `kra ws task sync` after direct edits

## Invalid examples

These examples are invalid structured task contracts:

- duplicate IDs:

```md
## Tasks

### TASK-001 First title
status: todo

### TASK-001 Second title
status: doing
```

- missing required `status:`:

```md
## Tasks

### TASK-002 Missing status

This block is not valid because `status:` is required.
```

- empty title:

```md
## Tasks

### TASK-003
status: todo
```

Task-specific commands fail closed on these invalid contracts.
Overview commands may degrade gracefully and surface task parsing warnings as supplemental data.
