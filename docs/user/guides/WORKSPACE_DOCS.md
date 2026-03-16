---
title: "Workspace docs viewer guide"
status: implemented
---

# Workspace docs viewer guide

This guide explains how to open workspace Markdown files in the `cmux` Markdown viewer with `kra ws doc open`.

## What it does

`kra ws doc open` opens one Markdown file and keeps viewer tabs collected into one docs pane slot per workspace.

- repeated opens append more `docs:*` viewer tabs in the same workspace docs area
- the docs pane is created on the right side when needed
- `repos/` content is intentionally excluded from this workflow

This is meant for workspace notes, plans, and other task-local Markdown, not repository browsing.

## Typical flow

```sh
kra ws doc open --current
kra ws doc open --current notes/
kra ws doc open --current tasks.md --no-focus
```

Typical results:

- no path: pick one Markdown file from the workspace
- directory path: pick one Markdown file only from that directory subtree
- file path: open that file directly

## Path rules

Accepted targets:

- workspace root Markdown files
- Markdown under workspace-local folders such as `notes/`
- `tasks.md`

Rejected targets:

- anything under `repos/`
- paths that resolve outside the workspace
- non-Markdown files

Supported Markdown extensions:

- `.md`
- `.markdown`

## Viewer behavior

`kra` uses `cmux markdown open` and routes the created viewer into the workspace docs pane.

- viewer tabs are renamed to `docs:<basename>`
- by default, focus moves to the newly opened viewer tab
- with `--no-focus`, the tab is added without moving your current focus

If no docs pane exists yet, `kra` creates one and reuses it for later `ws doc open` calls in the same workspace.

## cmux requirements

This command requires `cmux` Markdown viewer support.

`kra` uses the `KRA_ROOT` `cmux` workspace as an internal staging workspace when opening Markdown and then moves the viewer into the target workspace docs pane. The `KRA_ROOT` mapping is ensured automatically when needed, using the same runtime creation logic as `kra root open` without selecting it.

## Advanced usage

```sh
kra ws doc open --id TASK-1234 notes/README.md
kra ws doc open --current worklog/insights/
kra ws doc open --current --surface <surface-ref> tasks.md
```

Use `--surface` only when you need to override the staging surface selection. In normal use, the default routing is preferred.

## Related docs

- Command overview: `docs/user/guides/COMMANDS.md`
- cmux integration guide: `docs/user/guides/CMUX.md`
- `KRA_ROOT` helper: `docs/dev/spec/commands/root.md`
- Command contract: `docs/dev/spec/commands/ws/doc/open.md`
