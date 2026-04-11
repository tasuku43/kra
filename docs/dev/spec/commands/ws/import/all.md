---
title: "`kra ws import all`"
status: implemented
---

# `kra ws import all`

## Purpose

Import workspaces from Jira and GitHub review requests in one command.
This command is for workspace creation (0..N), not for actions on existing workspaces.

## Command forms

- `kra ws import all [--target jira|github-review|both] [--limit <n>] [--apply] [--no-prompt] [--format human|json]`

## Input rules

- `--target` controls which source set to include:
  - `jira`
  - `github-review`
  - `both`
- `--target` resolution order:
  1. CLI flag
  2. `<current-root>/.kra/config.yaml` -> `integration.import.defaults.target`
  3. `~/.kra/config.yaml` -> `integration.import.defaults.target`
  4. fallback `both`
- `--limit` default is `30` and valid range is `1..200`.
- Legacy `--json` is supported as an alias for `--format json`.
- Scope/mode resolution is delegated to existing source-specific defaults:
  - Jira uses `integration.jira.defaults.*`
  - GitHub review uses `integration.github.defaults.review.*`
- This command does not introduce source-specific scope flags.
  Use the existing individual commands when you need ad-hoc source-specific overrides.

## Resolution rules

- `target=jira` runs only the Jira import planner.
- `target=github-review` runs only the GitHub review import planner.
- `target=both` runs both planners and merges their results into one plan/apply flow.
- Jira prompt behavior stays the same as `kra ws import jira`:
  - if Jira defaults are insufficient, the command may prompt for JQL or sprint selection unless `--no-prompt` is set.
- GitHub review scope resolution stays the same as `kra ws import github review`.

## Plan/apply flow

- Default behavior is plan-only.
- When both providers are selected:
  - resolve each provider plan independently
  - merge summaries into one combined plan
  - ask for a single final apply confirmation
- In non-prompt mode:
  - apply only when `--apply` is explicitly provided.
- Apply is best-effort across both providers:
  - continue creating other workspaces even when some items fail.

## Output

- Human output should include:
  - `Plan:`
  - combined `targets`
  - merged `to create`, `skipped`, `failed` sections
  - provider prefixes in item labels:
    - `[jira]`
    - `[github-review]`
- After apply (human):
  - `Result:` + `create=<n> skipped=<n> failed=<n>`
- JSON output (`--format json`) must provide:
  - `action=ws.import.all`
  - `targets`
  - merged `summary`
  - nested per-provider details under `jira` and/or `github_review`
  - `applied`

## Exit codes

- `0`: no failed items (including plan-only mode).
- non-zero: one or more failed items.
- usage errors must use command usage error code.
