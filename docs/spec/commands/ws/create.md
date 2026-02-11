---
title: "`gionx ws create`"
status: implemented
---

# `gionx ws create`

## Purpose

Create a workspace from a root-local template.

## Inputs

- `id`: user-provided workspace ID
  - validation rules should follow `gion` (e.g. reject `/`)
- `--template <name>` (optional): template name under `<current-root>/templates`
  - if omitted, use `default`
- `--no-prompt` (optional): do not prompt for `title` (store empty)
- `--jira <ticket-url>` (optional): resolve `id` and `title` from Jira issue
  - `id = issueKey`
  - `title = issue summary`
  - auth is env-only:
    - `GIONX_JIRA_BASE_URL`
    - `GIONX_JIRA_EMAIL`
    - `GIONX_JIRA_API_TOKEN`
  - fail-fast if issue fetch/auth/parse fails (no workspace dir, no state row)
  - must not be combined with `--id` / `--title`
  - can be combined with `--template`

## Behavior

- Resolve current root with existing root policy (context/nearest root).
- Resolve template from `<current-root>/templates/<name>`.
- Template must pass shared validation before any workspace directory is created.
  - reserved top-level paths are forbidden:
    - `repos/`
    - `.git/`
    - `.gionx.meta.json`
  - symlink entries are forbidden
  - validation reports all found violations (not first-only)
- Create `<current-root>/workspaces/<id>/`
- Copy `templates/<name>/` contents into `workspaces/<id>/` (static copy, no placeholder expansion).
- Prompt for `title` and store it in workspace metadata (`.gionx.meta.json`)
  - if in a no-prompt mode, store an empty title
- In `--jira` mode:
  - do not prompt
  - store `workspace.source_url = <ticket-url>`
- Workspace ID collisions:
  - if `<id>` already exists as `active`, return an error and reference the existing workspace
  - if `<id>` already exists as `archived`, guide the user to `gionx ws --act reopen <id>`
  - if `<id>` was previously purged, allow creating it again as a new generation
- Do not create repos at this stage (repos are added via `ws --act add-repo`).
- If copy or metadata write fails after workspace dir creation, remove `workspaces/<id>/` and fail.

## Output

- Success output must use shared section style:
  - `Result:`
  - `  Created 1 / 1`
  - `  ✔ <workspace-id>`
  - `  path: <GIONX_ROOT/workspaces/<id>>`
- `Result:` heading style follows shared UI token rules (`text.primary` + bold).
- Summary line should follow shared result color semantics (`status.success` on success).

## FS metadata behavior

- `ws create` must create `workspaces/<id>/.gionx.meta.json` as canonical workspace metadata.
- Initial file content must include:
  - `schema_version`
  - `workspace` object (`id`, `title`(stored as `title` for compatibility), `source_url`, `status=active`, timestamps)
  - `repos_restore` as an empty array
- File write must be atomic (`temp + rename`).

## Errors

- Missing template:
  - fail and show available template names
- `--template` omitted and `default` missing:
  - fail (no fallback to scaffold mode)
- Template validation errors:
  - fail and print each violation with path + reason
