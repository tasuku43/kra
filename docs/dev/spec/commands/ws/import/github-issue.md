---
title: "`kra ws import github issue`"
status: implemented
---

# `kra ws import github issue`

## Purpose

Import GitHub issues into local workspaces in bulk with a selector-first flow.
This command is for workspace creation (0..N), not for actions on existing workspaces.

## Command forms

- `kra ws import github issue [--org <name> | --repo <owner/name>] [--state open|closed|all] [--limit <n>]`

## Input rules

- `--org` and `--repo` are mutually exclusive.
- If both are omitted, resolve scope from config:
  - `<current-root>/.kra/config.yaml` -> `integration.github.defaults.issue.org` / `repo`
  - `~/.kra/config.yaml` -> `integration.github.defaults.issue.org` / `repo`
- Config precedence is `CLI flag > root config > global config > command default`.
- `integration.github.defaults.issue.org` and `integration.github.defaults.issue.repo` must not be active at the same time.
- If scope is still unresolved after config lookup, fail with usage guidance asking for `--org` or `--repo`.
- `--state` default is `open`.
- `--state` allowed values are `open`, `closed`, `all`.
- `--limit` default is `30` and valid range is `1..200`.
- Initial implementation is interactive-only.
- `--format`, `--json`, `--apply`, and `--no-prompt` are not supported for `github issue` import.

## Resolution rules

- GitHub issue import is scope-first.
- `--org <name>` searches issues within the organization scope.
- `--repo <owner/name>` searches issues within the single repository scope.
- Default behavior does not add assignee/author filters.
- Default behavior uses GitHub issue state `open`.

## Workspace mapping

- Each imported item maps to exactly one GitHub issue.
- Human-facing item labels should prefer `owner/repo#number title`.
- Workspace id must be normalized to lowercase and use:
  - `owner-repo-issue-<number>`
- Typical workspace title source is the GitHub issue title.
- After workspace creation, the matching repository is automatically added to the workspace.
- Auto-added repository branch behavior:
  - use the standard workspace branch template (`workspace.branch.template`)
  - do not prompt per issue for a branch name

## Selection/create flow

- First collect candidates from the resolved scope and state filter.
- Pre-existing workspaces are classified before selection:
  - active workspace with same ID -> `skip`
  - archived workspace with same ID -> `skip`
  - invalid derived workspace ID -> `fail`
- Remaining candidates are shown in the shared interactive selector UI.
- User can multi-select issues with space and confirm with Enter.
- After selection, prompt:
  - `create <N> workspaces? [Enter=yes / n=no]`
- Creation is best-effort:
  - continue creating other selected items even when some items fail.
- After create (human output):
  - print `Result:` with summary counts and completion message.

## Conflict policy

- Default conflict behavior is `skip`.
- Typical skip reasons include:
  - existing active workspace with same ID
  - archived workspace with same ID
  - invalid workspace ID derived from repo/name/number

## Output

- Human output should include:
  - interactive issue selector for creatable candidates
  - confirmation prompt for selected count
  - `Result:`
  - summary counts after execution
  - `skipped (N)` list (`already_active` reason is omitted for readability)
  - `failed (N)` list with reason/message
- Machine-readable JSON output is reserved for a later iteration.

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
