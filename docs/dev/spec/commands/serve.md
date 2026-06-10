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
- `/api/workspaces` returns read-only JSON for open workspace boards.
- `/api/workspaces/<workspace-id>` returns read-only JSON for one open workspace board.
- `/api/workspaces/<workspace-id>/repos` returns read-only HTML for the workspace repository overview, including lazily inferred pull request links.

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
- Sidebar workspace links include a compact task progress bar.
- The page includes a light/dark theme toggle and persists the user's choice in browser local storage.
- Archived workspaces are not shown in the MVP.
- Board columns use a fixed visual height that fits roughly five task cards; overflow tasks remain available via vertical scrolling within the column.
- Each swimlane header displays a task progress bar derived from `done / total` task counts.
- The browser polls the read-only JSON API and updates the sidebar, progress bars, and board cards without a full page reload.
- Polling workspace board JSON does not perform remote pull request lookups.

## Workspace Detail

`/workspaces/<workspace-id>/` renders:

- page title in the form `<workspace-id>: <workspace title>`
- a left sidebar file tree rooted at the workspace directory
- a central read-only tab viewer for files and workspace-specific virtual views

The file tree:

- lists regular workspace files and directories.
- supports collapsible directories.
- treats `workspace.md` as a special openable item.
- treats `repos/` as a special openable item and does not list repository children under it.

The tab viewer:

- opens selected tree items as tabs.
- allows multiple open tabs.
- renders `workspace.md` as the same board component used by the root board.
- renders `repos/` as a repository overview.
- renders Markdown files as read-only HTML with GitHub-flavored Markdown support for common authoring features such as tables, strikethrough, task lists, fenced code blocks, and heading anchors.
- escapes raw HTML from Markdown content by default.
- renders fenced `mermaid` code blocks as Mermaid diagrams in the browser when the Mermaid CDN is reachable.
- renders HTML files in a sandboxed preview frame and exposes source below the preview.
- sizes HTML preview frames to the rendered document height instead of a fixed viewer height.
- renders source files with syntax highlighting based on file extension, including Go, shell (`.sh`, `.bash`, `.zsh`), and JavaScript.
- keeps the UI read-only.

The `workspace.md` board uses the same fixed-height, vertically scrollable columns as the root board and updates from the read-only JSON API without a full page reload.

The `repos/` tab displays one card per managed worktree/repo binding:

- repository alias/name
- current branch
- matching pull requests with status labels (`open`, `draft`, `merged`, `closed`, or `unknown`)
- the pull request for the repo's current branch first, followed by other pull requests whose title contains the workspace id
- Pull request lookup is lazy: the initial workspace detail page may render a loading state in the `repos/` tab, then the browser fetches `/api/workspaces/<workspace-id>/repos` when that tab is opened and replaces the loading state with links.

## Pull Request Link Inference

- For GitHub repos, `kra serve` uses the local `gh` CLI on a best-effort read-only basis to list pull requests only when the repository overview is requested.
- For each managed GitHub repo, collect:
  - pull requests whose head branch matches the current workspace repo branch
  - pull requests whose title contains the workspace id
- Dedupe pull requests by URL/number and render current-branch matches first.
- If `.kra.meta.json.workspace.source_url` is a GitHub pull request URL and it matches a managed repo,
  include it as a fallback link when remote lookup is unavailable or does not return that PR. Matching normalizes GitHub identifiers such as `owner/repo`,
  `github.com/owner/repo`, `https://github.com/owner/repo(.git)`, and `git@github.com:owner/repo.git`.
- If there is exactly one managed repo and the source URL is a GitHub pull request URL, the fallback link is associated with that repo.
- If no matching pull request exists or lookup is unavailable and there is no fallback link, render an empty PR state for that repo.

## Server Behavior

- Default listen address is `127.0.0.1:8765`.
- `--addr <host:port>` overrides the listen address.
- On startup, print the served URL to stdout.
- The command runs until interrupted.

## Non-goals

- Editing tasks or repository state from the browser
- Archived workspace browsing
- Authentication or network exposure beyond the chosen local address
