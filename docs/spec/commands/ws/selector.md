---
title: "`gionx ws` selector UI"
status: planned
---

# Selector UI (shared by `ws close` / `ws go` / `ws reopen` / `ws purge`)

## Purpose

Provide a non-fullscreen interactive selector for frequent workspace operations.

## Interaction model

- Render an inline list in terminal (do not take over full screen).
- Cursor movement by arrow keys (`Up` / `Down`).
- `Enter`: toggle check on focused row.
- `Ctrl+D`: confirm current selection and execute command-specific action.
- `Ctrl+C`: cancel without side effects.

## TTY requirement

- Selector mode requires an interactive TTY on stdin.
- If a selector-capable command is invoked without `<id>` in non-TTY context, it must fail fast with an error
  equivalent to `interactive workspace selection requires a TTY`.
- No automatic fallback is allowed in non-TTY mode.

## Display model

Each row should include at least:
- selected marker
- workspace `id`
- `risk` (command-appropriate)
- summary text (for example description)

Header/footer should show:
- command mode (`close`, `go`, `reopen`, `purge`)
- scope (`active` or `archived`)
- key hints (`Enter`, `Ctrl+D`, `Ctrl+C`)

## Visual consistency rules

- Primary information (selection marker, workspace id, risk badge, final warnings) uses normal/high contrast.
- Supplemental information (repo tree lines, helper hints, command metadata) uses muted/low-contrast styling.
  - Preferred terminal style is gray-like ANSI colors (for example `bright black` family).
- Do not vary supplemental color semantics by command; the same visual hierarchy must be applied across
  `ws close/go/reopen/purge`.
- Color is optional fallback:
  - when color is unavailable, preserve hierarchy via prefixes/indentation only.

## Row layout and alignment

- Row information order is fixed:
  - focus marker, selection marker, `id`, `[risk]`, `- description`
- Canonical row shape:
  - `> ✔︎ WS-202 [dirty] - payment hotfix`
  - `  ○  WS-101 [clean] - login flow`
- `status` is not rendered per row. State context is provided by header `scope`.
- The description column must be vertically aligned across rows (fixed description start column).
- `risk` is always rendered as a word badge:
  - `[clean]`, `[unpushed]`, `[diverged]`, `[dirty]`, `[unknown]`

## Selection visual behavior (task-list style)

- Unselected row:
  - normal contrast + `○`
- Selected row:
  - row-wide muted/low-contrast + `✔︎`
  - this muted style also applies to the risk badge on that row
- Focus row:
  - `>` marker indicates current cursor row
- Optional enhancement:
  - strikethrough for selected rows may be used when terminal capability is reliable; otherwise muted-only.

## Shared UI component policy

To prevent per-command drift, selector-related rendering must be built from shared components.

Required shared modules (logical units):
- `StyleToken`: semantic style set (`primary`, `muted`, `warning`, `danger`, `selected`)
- `WorkspaceRowRenderer`: one-line summary renderer for a workspace
- `RepoTreeRenderer`: indented supplemental renderer for repo tree details
- `RiskBadgeRenderer`: canonical risk badge formatter (`[clean]`, `[dirty]`, `[unpushed]`, `[unknown]`)
- `SelectorFrameRenderer`: header/footer and key-hint renderer

Rules:
- Command handlers (`ws close/go/reopen/purge`) must not define ad-hoc colors or row formats inline.
- Display differences by command should be expressed via data (mode/scope/actions), not bespoke render code.
- `ws list --tree` should reuse `WorkspaceRowRenderer` and `RepoTreeRenderer` for visual parity with selector flows.

## Risk label semantics (aligned with `gion`)

- Repo-level labels:
  - `unknown`: status cannot be determined (for example git status error, detached HEAD, missing/unknown upstream)
  - `dirty`: uncommitted changes exist (staged / unstaged / untracked / unmerged)
  - `diverged`: `ahead > 0 && behind > 0`
  - `unpushed`: `ahead > 0` (and not diverged)
  - `clean`: none of the above
- `behind only` is treated as `clean`.
- Workspace-level aggregation priority:
  - `unknown > dirty > diverged > unpushed > clean`

## Risk severity (warning strength)

- `danger` (strong warning): `unknown`, `dirty`
- `warning` (normal warning): `diverged`, `unpushed`
- `clean`: no warning marker required

## Scope rules

- `ws close`: default list is `active`
- `ws go`: default list is `active`
- `ws reopen`: default list is `archived`
- `ws purge`: default list is `archived`

Optional flags may switch scope if defined in each command spec.

## Selection cardinality

- `ws go`: single selection only (exactly one required)
- `ws close`: multiple selection allowed
- `ws reopen`: multiple selection allowed
- `ws purge`: multiple selection allowed

## Confirmation integration

- Selector confirmation (`Ctrl+D`) only finalizes selection.
- Destructive commands (`close`, `purge`) must still run safety checks and explicit final confirmation steps
  defined by their command specs.
