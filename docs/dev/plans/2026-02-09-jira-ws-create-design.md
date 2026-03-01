# Jira Ticket URL Integration UX Design (MVP)

Date: 2026-02-09
Status: agreed
Scope: `kra ws create --jira <ticket-url>` (single issue only)

## Goals

- Add a fast MVP path to create a workspace directly from a Jira ticket URL.
- Keep existing `ws create` UX consistent, and avoid introducing a separate command for single issue creation.
- Preserve architecture boundaries (`cli` -> `app` -> `infra`) so sprint bulk import can reuse the same app core later.

## User Experience (MVP)

- Entry point: `kra ws create --jira <ticket-url>`.
- On success:
  - `workspace id` is Jira `issueKey` (for example: `PROJ-123`).
  - `workspace title` is Jira `summary`.
  - Workspace creation then follows normal `ws create` behavior.
- Strict mode:
  - `--jira` cannot be combined with `--id` or `--title`.
  - Any Jira fetch failure aborts command (fail-fast, no workspace creation).

## Auth and Configuration

- MVP uses environment variables only (no local credential persistence).
- Variable naming uses `kra` prefix for future provider expansion:
  - `KRA_JIRA_BASE_URL`
  - `KRA_JIRA_EMAIL`
  - `KRA_JIRA_API_TOKEN`
- Rationale:
  - fastest secure delivery for MVP
  - no local secrets-at-rest concern
  - future-ready naming for GitHub integration (`KRA_GITHUB_*`)

## Architecture and Future Compatibility

- `cli` layer:
  - parse `--jira`
  - validate argument conflicts (`--jira` with `--id`/`--title`)
  - print user-facing result/errors
- `app` layer:
  - `fetchIssueFromJira(url)` use case step
  - `createWorkspaceFromIssue(issue)` use case step
  - reuse existing workspace creation core
- `infra` layer:
  - Jira API client + env loader + response mapping

This split intentionally prepares for future bulk flow (`jira import sprint ...`) by reusing `createWorkspaceFromIssue(issue)` for each fetched issue.

## Error UX (MVP)

- Missing env vars: clear message with required variable names.
- Invalid URL format: explicit Jira ticket URL error.
- Jira 401/403: auth error.
- Jira 404: issue not found or inaccessible.
- Any fetch/parsing failure: fail-fast, no side effects.

## Out of Scope (MVP)

- Sprint bulk creation command
- Secret storage command (for example `kra jira auth`)
- Multi-provider auto-detection (`--from <url>`)
- Long-term sync behavior with Jira updates
