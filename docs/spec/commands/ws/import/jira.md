---
title: "`gionx ws import jira`"
status: implemented
---

# `gionx ws import jira`

## Purpose

Import Jira issues into local workspaces in bulk with a plan-first flow.
This command is for workspace creation (0..N), not for actions on existing workspaces.

## Command forms

- `gionx ws import jira --sprint [<id|name>] --space <key> [--limit <n>] [--apply] [--no-prompt] [--json]`
- `gionx ws import jira --sprint [<id|name>] --project <key> [--limit <n>] [--apply] [--no-prompt] [--json]`
- `gionx ws import jira --jql "<expr>" [--limit <n>] [--apply] [--no-prompt] [--json]`

## Input rules

- `--sprint` and `--jql` are mutually exclusive.
- One of `--sprint` or `--jql` is required.
- `--space` is the primary scope key option for sprint mode.
- `--project` is an alias of `--space` (same behavior).
- `--space`/`--project` is required with `--sprint`.
- `--space` and `--project` must not be combined.
- `--board` is not supported with `--sprint`.
- `--limit` default is `30` and valid range is `1..200`.
- With `--no-prompt`:
  - if `--apply` is set, execute apply.
  - if `--apply` is not set, print plan only and exit with success.

## Resolution rules

- Default import filter:
  - `assignee = currentUser()`
  - `statusCategory != Done`
- Apply order follows Jira rank order.

### `--sprint` resolution

- `--sprint` mode is JQL-first and does not require Jira board/sprint API resolution.
- If `<id|name>` is numeric:
  - build JQL with `sprint = <id>`.
- If `<id|name>` is non-numeric:
  - build JQL with `sprint = "<name>"`.
- `--space`/`--project` scope is always included:
  - `project = <space-key>`.
- If value is omitted (`--sprint` only):
  - in prompt mode, list `Active + Future` sprints under `--space/--project` and ask selection.
  - when TTY is available, use shared interactive selector UI.
  - when TTY is not available, fallback to numbered text prompt.
  - build JQL with selected sprint id.
  - in `--no-prompt`, return usage/error and ask for explicit `--sprint <id|name>` or `--jql`.

### Kanban/non-sprint usage

- For teams not using sprint mode, `--jql` is required.
- If sprint candidates are unavailable, guide user to `--jql "<expr>"`.

## Plan/apply flow

- Default behavior is plan-only (dry-run equivalent).
- In prompt mode:
  - print plan (`To Create`, `Skipped`, `Failed`) and include
    `apply this plan? [Enter=yes / n=no]` after plan body.
  - Enter means apply.
- In non-prompt mode:
  - apply only when `--apply` is explicitly provided.
- `--apply` is best-effort:
  - continue creating other items even when some items fail.
- After apply (human output):
  - print `Result:` with summary counts and completion message.

## Conflict policy

- Default conflict behavior is `skip`.
- Typical skip reasons include:
  - existing active workspace with same ID
  - archived workspace with same ID
  - invalid workspace ID derived from issue key

## Output

- Human output should include:
  - `Plan:`
  - bullet-based `source` and `filters`
  - `to create (N)` list
  - `skipped (N)` list (`already_active` reason is omitted for readability)
  - `failed (N)` list with reason/message
- In prompt mode (human):
  - include `apply this plan? [Enter=yes / n=no]` as the last plan line.
- After apply (human):
  - `Result:` + `create=<n> skipped=<n> failed=<n>`
- JSON output (`--json`) must provide equivalent information in stable schema.

### JSON contract (`--json`)

- `stdout` must contain JSON only.
- Prompts and progress logs must go to `stderr`.
- In plan-only mode, items must be classified with `action=create|skip|fail`.

Example shape:

```json
{
  "source": {
    "type": "jira",
    "mode": "sprint",
    "board": "TEAM",
    "sprint": "Sprint 34",
    "jql": ""
  },
  "filters": {
    "assignee": "currentUser()",
    "statusCategory": "not_done",
    "limit": 30
  },
  "summary": {
    "candidates": 18,
    "to_create": 12,
    "skipped": 4,
    "failed": 2
  },
  "items": [
    {
      "issue_key": "PROJ-101",
      "title": "API retry logic",
      "workspace_id": "PROJ-101",
      "action": "create"
    },
    {
      "issue_key": "PROJ-099",
      "title": "Old task",
      "workspace_id": "PROJ-099",
      "action": "skip",
      "reason": "already_active"
    },
    {
      "issue_key": "PROJ-120",
      "title": "Broken task",
      "workspace_id": "PROJ-120",
      "action": "fail",
      "reason": "fetch_failed",
      "message": "jira fetch timeout"
    }
  ]
}
```

## Reason codes

Stable reason codes for `skip`/`fail`:

- `already_active`
- `archived_exists`
- `invalid_workspace_id`
- `permission_denied`
- `not_found`
- `fetch_failed`
- `create_failed`

## Exit codes

- `0`: no failed items (including plan-only mode).
- non-zero: one or more failed items.
- usage errors (invalid flag combination or missing required mode) must use command usage error code.
