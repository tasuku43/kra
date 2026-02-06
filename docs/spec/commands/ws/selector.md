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

## Display model

Each row should include at least:
- selected marker
- workspace `id`
- `status`
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
