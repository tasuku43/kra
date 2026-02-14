---
title: "`kra ws insight add`"
status: implemented
---

# `kra ws insight add --id <workspace-id> --ticket <ticket> --session-id <session-id> --what "<text>" --approved [--context "<text>"] [--why "<text>"] [--next "<text>"] [--tag <tag> ...] [--format human|json]`

## Purpose

Persist one approved high-value insight into workspace-local worklog path.

## Behavior

- resolve target workspace from `--id` under:
  - `workspaces/<id>/`
  - `archive/<id>/`
- fail when workspace is missing.
- require `--approved` to write.
- write one markdown document under:
  - `<workspace>/worklog/insights/`

## File contract

- filename:
  - `<yyyyMMdd-HHmmss>-<slug>.md`
- slug source:
  - derive from `--what`, lowercase kebab-case, fallback `insight`
- frontmatter required:
  - `kind: insight`
  - `ticket`
  - `workspace`
  - `created_at` (RFC3339 UTC)
  - `session_id`
  - `tags` (array)
- body sections:
  - `Context`
  - `What happened`
  - `Why it matters`
  - `Next reuse`

## JSON envelope

- `ok`
- `action=ws.insight.add`
- `workspace_id`
- `result`:
  - `path`
  - `kind` (`insight`)
- `error`
