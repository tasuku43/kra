---
title: "`gionx ws` selector UI"
status: implemented
---

# Selector UI (shared by `ws --act close` / `ws --act go` / `ws --act reopen` / `ws --act purge`)

## Purpose

Provide a non-fullscreen interactive selector for frequent workspace operations.

## Interaction model

- Render an inline list in terminal (do not take over full screen).
- Cursor movement by arrow keys (`Up` / `Down`).
- `Space`: toggle check on focused row.
- `Enter`: proceed with current selection.
- `Esc` or `Ctrl+C`: cancel without side effects.
- Text input is always treated as filter query input (no dedicated filter mode).
- Filter matching is fuzzy ordered-subsequence (aligned with `gion`):
  - query characters must appear in order, but do not need to be contiguous.
  - spaces in query are ignored.
  - matching is case-insensitive.
  - examples:
    - `example-org/helmfiles` matches query `cs`.
    - `example-org/helmfiles` matches query `c s`.
- `Backspace` / `Delete`: remove one rune from filter query.
- Filter text must persist after selection toggle; it is cleared only when the user explicitly deletes it.
- Single-select mode (`ws --act go`) uses cursor + Enter confirmation:
  - selection markers stay visible (`○/●`) for visual parity with multi-select.
  - `Space` has no effect.
  - footer does not show `selected: n/m`.
  - `Enter` confirmation locks input briefly (`0.2s`) before stage transition.

## TTY requirement

- Selector mode requires an interactive TTY on stdin.
- If a selector-capable command is invoked without `<id>` in non-TTY context, it must fail fast with an error
  equivalent to `interactive workspace selection requires a TTY`.
- No automatic fallback is allowed in non-TTY mode.

## Display model

Each row should include at least:
- selected marker
- workspace `id`
- summary text (for example title)

Header/footer should show:
- command mode (`close`, `go`, `reopen`, `purge`)
- scope (`active` or `archived`)
- key hints (`Space`, `Enter`, text filter input, `Esc`/`Ctrl+C`)
- `Enter` hint label should be command-specific action text (for example `enter close` for `ws --act close`).
- Footer readability/truncation rule:
  - left-most `selected: n/m` must remain visible.
  - key hints are appended in a fixed order and dropped from the right on narrow terminals.
  - if hints are partially omitted, append `…` when width allows.

Section-style output should use consistent headings:
- `Workspaces(<status>):`
- `Risk:`
- `Result:`

Section body indentation must be controlled by shared global constants (no per-command hardcoded spaces).
- Global section body indentation is fixed to two spaces.
- Section spacing:
  - `Workspaces(...)` and `Risk:` have one blank line after heading.
  - `Result:` has no blank line after heading.
  - every section block ends with exactly one trailing blank line.
- Selector footer/status lines (for example `selected: n/m` and key hints) are part of section body and must use
  the same two-space indentation.
- Selector inline message lines (validation/error/help) must also use the same two-space indentation.
- Confirmation prompts shown under `Risk:` must follow the same body indentation.

`<status>` color semantics (TTY):
- `active`: accent/cool color
- `archived`: distinct alternate color
- no-color environments: plain text only

## Visual consistency rules

- Primary information (selection marker, workspace id, risk badge, final warnings) uses normal/high contrast.
- Supplemental information (repo tree lines, helper hints, command metadata) uses muted/low-contrast styling.
  - Preferred terminal style is gray-like ANSI colors (for example `bright black` family).
- Validation/error messages shown below selector footer must use the shared error token (danger/error color).
- Do not vary supplemental color semantics by command; the same visual hierarchy must be applied across
  `ws --act close/go/reopen/purge`.
- Color is optional fallback:
  - when color is unavailable, preserve hierarchy via prefixes/indentation only.

Semantic color token policy:
- Selector rendering must follow `docs/spec/concepts/ui-color.md`.
- Raw ad-hoc color assignments in command handlers are disallowed.
- `active`/`archived` label coloring uses:
  - `active` -> `accent`
  - `archived` -> `text.muted`

## Row layout and alignment

- Row information order is fixed:
  - focus marker, selection marker, `id`, `title`
- Canonical row shape:
  - `> ○ WS-101      login flow`
  - `  ● WS-202      payment hotfix`
- Single-select canonical row shape:
  - `> ○ WS-101      login flow`
  - `  ● WS-202      payment hotfix`
- For repo-pool selectors (`itemLabel=repo`), title column is omitted by default (no `(no title)` filler).
- Workspace selectors use metadata title as summary text and must not derive dynamic logical work-state
  labels (`todo` / `in-progress`) from per-repo runtime inspection.
- `status` is not rendered per row. State context is provided by header `scope`.
- The title column must be vertically aligned across rows (fixed title start column).
- Risk tags are not rendered in `Workspaces(...)` rows.
- Risk hint in `Workspaces(...)` is color-only and applies to workspace id:
  - clean: default
  - warning (`unpushed`/`diverged`): warn color
  - danger (`dirty`/`unknown`): error color
- `ws list` summary rows do not render risk hints; they use bullet-list `id: title` only.

## Selection visual behavior (task-list style)

- Unselected row:
  - normal contrast + `○`
- Selected row:
  - normal contrast + `●`
- Focus row:
  - `>` marker indicates current cursor row
  - On color-capable terminals, apply a subtle low-contrast background highlight to the focused row.
  - In no-color environments, keep marker-only focus indication (no extra decoration).

## Shared UI component policy

To prevent per-command drift, selector-related rendering must be built from shared components.

Required shared modules (logical units):
- `StyleToken`: semantic style set (`primary`, `muted`, `warning`, `danger`, `selected`, `status_active`, `status_archived`, `message_error`)
- `WorkspaceRowRenderer`: one-line summary renderer for a workspace
- `RepoTreeRenderer`: indented supplemental renderer for repo tree details
- `RiskBadgeRenderer`: canonical risk badge formatter for `Risk:` section
- `SectionTitleRenderer`: section heading renderer (`Workspaces(...)`, `Risk`, `Result`)
- `SectionBlockRenderer`: canonical section assembler (heading/body/trailing-blank contract)
- `StatusLabelRenderer`: canonical status label formatter (`active`, `archived`)
- `SelectorFrameRenderer`: footer and key-hint renderer

Rules:
- Command handlers (`ws --act close/go/reopen/purge`) must not define ad-hoc colors or row formats inline.
- Display differences by command should be expressed via data (mode/scope/actions), not bespoke render code.
- `ws list --tree` should reuse `WorkspaceRowRenderer` and `RepoTreeRenderer` for visual parity with selector flows.
- Selector-capable command handlers must delegate stage orchestration (`Workspaces -> Risk -> Result`) to the
  shared flow component (`runWorkspaceSelectRiskResultFlow`) instead of implementing bespoke stage transitions.

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

- `ws --act close`: default list is `active`
- `ws --act go`: default list is `active`
- `ws --act reopen`: default list is `archived`
- `ws --act purge`: default list is `archived`

Optional flags may switch scope if defined in each command spec.

## Unified launcher flow (planned)

- Human launcher:
  - `gionx ws`
- Explicit selection entrypoint:
  - `gionx ws select`

Behavior:
- outside workspace:
  - `gionx ws` requires explicit `--id <id>` (no implicit list fallback).
  - selection-first flow is `gionx ws select`.
- inside workspace:
  - if under `workspaces/<id>/`: show current-workspace action menu:
    - `add-repo`
    - `close`
  - if under `archive/<id>/`: show current-workspace action menu:
    - `reopen`
    - `purge`

Action menu ordering:
- fixed order for in-workspace active mode:
  - `add-repo` first
  - `close` second
- fixed order for in-workspace archived mode:
  - `reopen` first
  - `purge` second

Launcher and selector relationship:
- launcher flow must delegate to existing action command flows to keep behavior parity.
- direct edit operations are routed by `ws --act <action>`.
- operation-level `--select` is not supported.

## Selection cardinality

- `ws --act go`: single selection only (exactly one required)
- `ws --act close`: multiple selection allowed
- `ws --act reopen`: multiple selection allowed
- `ws --act purge`: multiple selection allowed

## Confirmation integration

- Selector proceed (`Enter`) finalizes current selection and moves to the next phase.
- Destructive commands (`close`, `purge`) must still run safety checks defined by their command specs.
- Confirmation policy split:
  - `ws --act close`: require confirmation only when selected set includes non-clean risk.
  - `ws --act purge`: always require at least one purge confirmation (and an additional confirmation for active risk).
- In stacked CLI-style flows, commands print sections sequentially:
  - clean-only selection: `Workspaces(...)` -> `Result:`
  - non-clean selection: `Workspaces(...)` -> `Risk:` -> `Result:`
