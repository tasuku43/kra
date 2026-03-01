---
title: "UI Color Tokens"
status: implemented
---

# UI Color Tokens (CLI/TUI)

## Purpose

Define one consistent semantic color rule for all interactive and non-interactive human-readable outputs.

`kra` must not design output colors by raw color names.  
Commands must use semantic tokens and shared helpers.

## Core policy

- Color is an enhancement, not the only carrier of meaning.
- Status must always be readable without color (icon + wording).
- Keep emphasis ratio low; primary text stays mostly default color.

## MVP token set

- `text.primary`
- `text.muted`
- `accent`
- `status.success`
- `status.warning`
- `status.error`
- `status.error.subtle`
- `status.info`
- `focus`
- `selection`
- `diff.add`
- `diff.remove`
- `git.ref.local`
- `git.ref.remote`

## kra mapping rules

- Workspace status:
  - `active` -> `accent`
  - `archived` -> `text.muted`
- Error emphasis:
  - apply `status.error` primarily to leading marker / title
  - use `status.error.subtle` for secondary risk counters/details when you need contrast without dominating
  - keep detailed reason text as normal/muted when possible
- Git reference labels (plan/detail views):
  - local branch/ref -> `git.ref.local`
  - remote upstream/ref -> `git.ref.remote`
  - keep `status.error` reserved for risky/error meanings (for example `dirty`)
- Selector:
  - focus row: `>` marker + subtle background highlight (color-capable terminals only)
  - selected state: `●/○` (must remain textual in no-color mode)

## Capability fallback

- no-color environments must preserve meaning via plain text symbols and labels.
- avoid command-specific ad-hoc color semantics.

## Enforcement

- CI must run `./scripts/lint-ui-color.sh`.
- `scripts/lint-ui-color.sh` is a hard gate for:
  - raw ANSI color literals outside `internal/cli/ws_ui_common.go`
  - direct `lipgloss.Color(...)` usage in CLI code
  - direct foreground/background concrete color assignment outside approved shared renderer paths
- Use `./scripts/lint-ui-color-coverage.sh` as an audit to detect plain heading literals
  that bypass shared styled heading renderers.
