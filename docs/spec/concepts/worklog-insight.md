---
title: "Workspace Worklog Insight Capture"
status: implemented
---

# Workspace Worklog Insight Capture

## Purpose

Improve traceability by preserving only high-value insights during work,
without always-on logging that may hurt flow.

## Core policy

- Do **not** persist continuous/always-on activity logs by default.
- Persist only important insights when proposed and approved.
- Keep records lightweight and reusable.

## Workspace reserved path

Reserve workspace-local path:

- `workspaces/<id>/worklog/insights/`

Archived workspaces keep the same relative path under:

- `archive/<id>/worklog/insights/`

## Capture flow (phase 1)

1. Agent detects a potential high-value insight.
2. Agent proposes capture in conversation.
3. User approves or rejects.
4. On approval, write one insight document.
5. On reject, write nothing.

No background capture is executed in this phase.

## CLI entrypoint (phase 1)

Use an explicit one-shot write command:

```sh
kra ws insight add --id <workspace-id> --ticket <ticket> --session-id <session-id> --what "<text>" --approved [--context "<text>"] [--why "<text>"] [--next "<text>"] [--tag <tag> ...] [--format human|json]
```

Guardrails:

- `--approved` is required to write.
- without approval flag, command must fail and write nothing.
- this command is the only mutation path in phase 1; no background writer.

Workspace scope:

- `--id` must target an existing workspace in either `workspaces/` or `archive/`.
- if not found, fail with `not_found`.

## File format

Each insight is saved as Markdown with frontmatter.

Filename:

- `<yyyyMMdd-HHmmss>-<slug>.md`

Frontmatter (required fields):

- `kind` (`insight` in phase 1)
- `ticket`
- `workspace`
- `created_at`
- `session_id`
- `tags` (array)

Body sections (recommended):

- `Context`
- `What happened`
- `Why it matters`
- `Next reuse`

## Git policy

- `worklog/insights/` is Git-tracked.
- Heavy runtime logs should be stored outside insight docs
  and managed with separate ignore policy.

## Non-goals (phase 1)

- automatic capture at fixed intervals
- full event stream persistence
- multi-kind taxonomy beyond `kind: insight`
