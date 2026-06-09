---
title: "`kra serve`"
status: implemented
---

# `kra serve [--addr <host:port>]`

## Purpose

Serve a local read-only web UI for understanding open workspaces at a glance.

## Scope

- The command resolves `KRA_ROOT` from the current working directory.
- The MVP serves active/open workspaces only.
- The UI is read-only. It must not mutate `workspace.md`, `.kra.meta.json`, repo state, or root state.

## Routes

- `/` redirects to `/workspaces/`.
- `/workspaces/` renders the root workspace board.
- `/workspaces/<workspace-id>/` renders one workspace detail page.
- `/workspaces/<workspace-id>` redirects to the trailing-slash route.

Unknown routes return `404`.

## Root Workspace Board

`/workspaces/` renders one swimlane per open workspace under the page title `All Workspaces`.

- Each swimlane is headed by workspace id and title.
- Each swimlane contains task cards grouped by status:
  - `Todo`
  - `Doing`
  - `Blocked`
  - `Done`
- Task cards are derived from the workspace task contract in `workspace.md`.
- Workspace links navigate to `/workspaces/<workspace-id>/`.
- Sidebar workspace links display the workspace id with the workspace title below it in muted text.
- Archived workspaces are not shown in the MVP.
- Board columns use a fixed visual height that fits roughly five task cards; overflow tasks remain available via vertical scrolling within the column.

## Workspace Detail

`/workspaces/<workspace-id>/` renders:

- page title in the form `<workspace-id>: <workspace title>`
- the same single-workspace board component used by the root board
- tabbed read-only sections:
  - `README`
  - `Repositories`
- Board columns use the same fixed-height, vertically scrollable layout as the root board.

The `README` tab displays workspace-local Markdown text:

- prefer `<workspace>/README.md`
- fall back to `<workspace>/workspace.md`
- show an empty state when neither exists
- render Markdown as read-only HTML with GitHub-flavored Markdown support for common authoring features such as tables, strikethrough, task lists, fenced code blocks, and heading anchors
- escape raw HTML from README content by default
- render fenced `mermaid` code blocks as Mermaid diagrams in the browser when the Mermaid CDN is reachable
- use the full available detail panel width for the Markdown view

The `Repositories` tab displays one row per managed worktree/repo binding:

- repository alias/name
- current branch
- pull request link when it can be inferred from workspace source metadata; the link text is the stored workspace title, which is the PR title for GitHub PR imports, falling back to `#<number>` when the title is empty
- the repository, branch, and pull request columns use a `2:2:6` width ratio

## Pull Request Link Inference

The MVP does not call remote providers.

- If `.kra.meta.json.workspace.source_url` is a GitHub pull request URL and it matches a managed repo,
  expose it as that repo's pull request link.
- If there is exactly one managed repo and the source URL is a GitHub pull request URL, expose it for
  that repo.
- Otherwise render `none`.

## Server Behavior

- Default listen address is `127.0.0.1:8765`.
- `--addr <host:port>` overrides the listen address.
- On startup, print the served URL to stdout.
- The command runs until interrupted.

## Non-goals

- Editing tasks or repository state from the browser
- Archived workspace browsing
- Authentication or network exposure beyond the chosen local address
- Remote provider API aggregation
