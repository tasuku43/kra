---
title: "`kra ws import github review`"
status: proposed
---

# `kra ws import github review`

## Purpose

Import GitHub pull requests that are assigned to the current user for review into local workspaces in bulk with a plan-first flow.
This command is for workspace creation (0..N), not for actions on existing workspaces.

## Command forms

- `kra ws import github review [--org <name> | --repo <owner/name>] [--limit <n>] [--apply] [--no-prompt] [--format human|json]`

## Input rules

- `--org` and `--repo` are mutually exclusive.
- If both are omitted, resolve scope from config:
  - `<current-root>/.kra/config.yaml` -> `integration.github.defaults.review.org` / `repo`
  - `~/.kra/config.yaml` -> `integration.github.defaults.review.org` / `repo`
- Config precedence is `CLI flag > root config > global config > command default`.
- `integration.github.defaults.review.org` and `integration.github.defaults.review.repo` must not be active at the same time.
- If scope is still unresolved after config lookup, fail with usage guidance asking for `--org` or `--repo`.
- Legacy `--json` is supported as an alias for `--format json`.
- `--limit` default is `30` and valid range is `1..200`.
- With `--no-prompt`:
  - if `--apply` is set, execute apply.
  - if `--apply` is not set, print plan only and exit with success.

## Resolution rules

- GitHub review import is scope-first.
- `--org <name>` searches review-requested pull requests within the organization scope.
- `--repo <owner/name>` searches review-requested pull requests within the single repository scope.
- Default review filter is:
  - review requested for the authenticated user
  - pull request state is `open`
- Draft pull requests are included when they satisfy the review-requested + open conditions.
- Initial version does not support extra filters such as author/label/team reviewer.

## Workspace mapping

- Each imported item maps to exactly one GitHub pull request.
- Human-facing item labels should prefer `owner/repo#number title`.
- Workspace id must be normalized to lowercase and use:
  - `owner-repo-pr-<number>`
- Typical workspace title source is the GitHub pull request title.

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
  - invalid workspace ID derived from repo/name/number

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
- Top-level shape must follow `docs/dev/spec/concepts/output-contract.md`:
  - `ok`
  - `action=ws.import.github.review`
  - `result` containing import details (`source`, `filters`, `summary`, `items`, `applied`)
  - `error` when `ok=false`

Example shape:

```json
{
  "ok": true,
  "action": "ws.import.github.review",
  "result": {
    "source": {
      "type": "github",
      "mode": "review"
    },
    "filters": {
      "scope": {
        "kind": "org",
        "value": "my-org"
      },
      "reviewer": "currentUser()",
      "state": "open",
      "limit": 30
    },
    "summary": {
      "candidates": 6,
      "to_create": 4,
      "skipped": 1,
      "failed": 1
    },
    "items": [
      {
        "repo": "my-org/api",
        "number": 456,
        "title": "Add retry budget",
        "workspace_id": "my-org-api-pr-456",
        "action": "create"
      },
      {
        "repo": "my-org/web",
        "number": 789,
        "title": "Cleanup old dashboard",
        "workspace_id": "my-org-web-pr-789",
        "action": "skip",
        "reason": "already_active"
      },
      {
        "repo": "my-org/infra",
        "number": 912,
        "title": "Refactor deploy pipeline",
        "workspace_id": "my-org-infra-pr-912",
        "action": "fail",
        "reason": "create_failed",
        "message": "workspace metadata write failed"
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
- usage errors (invalid flag combination or unresolved required scope) must use command usage error code.
