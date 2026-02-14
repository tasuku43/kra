---
title: "`kra ws import jira`"
status: implemented
---

# `kra ws import jira`

## Purpose

Import Jira issues into local workspaces in bulk with a plan-first flow.
This command is for workspace creation (0..N), not for actions on existing workspaces.

## Command forms

- `kra ws import jira [--sprint [<id|name>] [--space <key>|--project <key>] | --jql [<expr>]] [--limit <n>] [--apply] [--no-prompt] [--format human|json]`
- `kra ws import jira --sprint [<id|name>] --space <key> [--limit <n>] [--apply] [--no-prompt] [--format human|json]`
- `kra ws import jira --sprint [<id|name>] --project <key> [--limit <n>] [--apply] [--no-prompt] [--format human|json]`
- `kra ws import jira --jql "<expr>" [--limit <n>] [--apply] [--no-prompt] [--format human|json]`

## Input rules

- `--sprint` and `--jql` are mutually exclusive.
- If both are omitted, resolve mode from config:
  - `<current-root>/.kra/config.yaml` -> `integration.jira.defaults.type`
  - `~/.kra/config.yaml` -> `integration.jira.defaults.type`
  - fallback `sprint`
- `--space` is the primary scope key option for sprint mode.
- `--project` is an alias of `--space` (same behavior).
- `--space`/`--project` is required with sprint mode after config resolution.
- `--space` and `--project` must not be combined.
- `--board` is not supported.
- Legacy `--json` is supported as an alias for `--format json`.
- `--limit` default is `30` and valid range is `1..200`.
- Jira base URL resolution order:
  1. `KRA_JIRA_BASE_URL` (if set)
  2. `<current-root>/.kra/config.yaml` -> `integration.jira.base_url`
  3. `~/.kra/config.yaml` -> `integration.jira.base_url`
- Jira credentials are env-only: `KRA_JIRA_EMAIL`, `KRA_JIRA_API_TOKEN`.
- With `--no-prompt`:
  - if `--apply` is set, execute apply.
  - if `--apply` is not set, print plan only and exit with success.

## Resolution rules

- Config precedence is `CLI flag > root config > global config > command default`.
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
- If `--space`/`--project` is omitted:
  - read `integration.jira.defaults.space` or `integration.jira.defaults.project` from config.
  - if both keys are active at the same time, fail with a clear config error.
- If value is omitted (`--sprint` only):
  - in prompt mode, list `Active + Future` sprints under `--space/--project` and ask selection.
  - when TTY is available, use shared interactive selector UI.
  - when TTY is not available, fallback to numbered text prompt.
  - build JQL with selected sprint id.
  - in `--no-prompt`, return usage/error and ask for explicit `--sprint <id|name>` or `--jql`.

### Kanban/non-sprint usage

- For teams not using sprint mode, `--jql` is required.
- If mode is JQL and `<expr>` is omitted:
  - prompt mode asks for JQL input (`jql: `).
  - `--no-prompt` fails with usage guidance.
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
- JSON output (`--format json`) must provide equivalent information in the shared envelope.

### JSON contract (`--format json`)

- `stdout` must contain JSON only.
- Prompts and progress logs must go to `stderr`.
- In plan-only mode, items must be classified with `action=create|skip|fail`.
- Top-level shape must follow `docs/spec/concepts/output-contract.md`:
  - `ok`
  - `action=ws.import.jira`
  - `result` containing import details (`source`, `filters`, `summary`, `items`, `applied`)
  - `error` when `ok=false`

Example shape:

```json
{
  "ok": true,
  "action": "ws.import.jira",
  "result": {
    "source": {
      "type": "jira",
      "mode": "sprint",
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
    ],
    "applied": false
  }
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
