---
title: "`kra ws create`"
status: implemented
---

# `kra ws create`

## Purpose

Create a workspace from a root-local template.

## Inputs

- `id`: user-provided workspace ID
  - validation rules should follow `gion` (e.g. reject `/`)
- `--id <id>` (optional): automation-friendly explicit ID flag
  - must not be combined with positional `<id>`
- `--title <title>` (optional): explicit title for non-Jira creation
  - when provided, title prompt is skipped
- `--template <name>` (optional): template name under `<current-root>/templates`
  - if omitted, resolve in this order:
    1. `<current-root>/.kra/config.yaml` -> `workspace.defaults.template`
    2. `~/.kra/config.yaml` -> `workspace.defaults.template`
    3. fallback `default`
- `--no-prompt` (optional): do not prompt for `title` (store empty)
- `--format human|json` (optional, default: `human`)
- `--jira <ticket-url>` (optional): resolve `id` and `title` from Jira issue
  - `id = issueKey`
  - `title = issue summary`
  - base URL resolution order:
    1. `KRA_JIRA_BASE_URL` (if set)
    2. `<current-root>/.kra/config.yaml` -> `integration.jira.base_url`
    3. `~/.kra/config.yaml` -> `integration.jira.base_url`
  - auth credentials are env-only:
    - `KRA_JIRA_EMAIL`
    - `KRA_JIRA_API_TOKEN`
  - fail-fast if issue fetch/auth/parse fails (no workspace dir, no state row)
  - must not be combined with `--id` / `--title`
  - can be combined with `--template`

## Behavior

- Resolve current root with existing root policy (context/nearest root).
- Config precedence is `CLI flag > root config > global config > command default`.
- Resolve template from `<current-root>/templates/<name>`.
- Template must pass shared validation before any workspace directory is created.
  - reserved top-level paths are forbidden:
    - `repos/`
    - `.git/`
    - `.kra.meta.json`
  - symlink entries are forbidden
  - validation reports all found violations (not first-only)
- Create `<current-root>/workspaces/<id>/`
- Copy `templates/<name>/` contents into `workspaces/<id>/` (static copy, no placeholder expansion).
- Prompt for `title` and store it in workspace metadata (`.kra.meta.json`)
  - if in a no-prompt mode, store an empty title
  - if `--title` is provided, use it and do not prompt
- In `--format json` mode:
  - command behaves non-interactively (no title prompt)
  - caller should provide either positional `<id>` or `--id <id>` for non-Jira create
  - machine response uses shared JSON envelope (`action=ws.create`)
- In `--jira` mode:
  - do not prompt
  - store `workspace.source_url = <ticket-url>`
- Workspace ID collisions:
  - if `<id>` already exists as `active`, return an error and reference the existing workspace
  - if `<id>` already exists as `archived`, guide the user to `kra ws reopen <id>`
  - if `<id>` was previously purged, allow creating it again as a new generation
- Do not create repos at this stage (repos are added via `ws add-repo`).
- If copy or metadata write fails after workspace dir creation, remove `workspaces/<id>/` and fail.
- `ws create` must auto-commit the create scope in `KRA_ROOT`:
  - commit message: `create: <workspace-id>`
  - staging allowlist:
    - `workspaces/<id>/`
    - `.kra/state/workspace-baselines/<id>.json`
    - `.kra/state/workspace-workstate.json`
  - if staged paths contain entries outside the allowlist, abort.
- `ws create` must initialize workspace baseline state at:
  - `.kra/state/workspace-baselines/<id>.json`
  - baseline captures:
    - repo baselines: `repos/<alias>` `baseline_head`
    - non-repo FS baselines: `path -> sha256` map (exclude `repos/**` and `.kra.meta.json`)

## Output

- Success output must use shared section style:
  - `Result:`
  - `  Created 1 / 1`
  - `  ✔ <workspace-id>`
  - `  path: <KRA_ROOT/workspaces/<id>>`
- `Result:` heading style follows shared UI token rules (`text.primary` + bold).
- Summary line should follow shared result color semantics (`status.success` on success).
- JSON mode output (`--format json`) must follow shared envelope:
  - `ok=true`
  - `action=ws.create`
  - `workspace_id=<id>`
  - `result.created=1`
- `result.path=<KRA_ROOT/workspaces/<id>>`
- `result.template=<resolved-template-name>`
  - `result.commit_sha=<create-commit-sha>`

## FS metadata behavior

- `ws create` must create `workspaces/<id>/.kra.meta.json` as canonical workspace metadata.
- Initial file content must include:
  - `schema_version`
  - `workspace` object (`id`, `title`(stored as `title` for compatibility), `source_url`, `status=active`, timestamps)
  - `repos_restore` as an empty array
  - `protection.purge_guard.enabled=true`
- File write must be atomic (`temp + rename`).

## Errors

- `--format json` with missing explicit workspace target:
  - fail with `invalid_argument` and usage exit code
- Missing template:
  - fail and show available template names
- `--template` omitted and `default` missing:
  - fail (no fallback to scaffold mode)
- Template validation errors:
  - fail and print each violation with path + reason
