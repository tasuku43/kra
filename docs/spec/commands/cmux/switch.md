---
title: "`kra cmux switch`"
status: implemented
---

# `kra cmux switch [--workspace <workspace-id>] [--cmux <id|ref>] [--format human|json]`

## Purpose

Switch to an existing mapped cmux workspace.

## Resolution Policy

- Explicit-first resolution:
  - `--workspace + --cmux`: resolve inside that workspace mapping.
  - `--workspace` only: resolve in that workspace (single auto-select, multi prompts in human mode).
  - `--cmux` only: global unique match across mappings.
- Ambiguity handling:
  - Human mode: fallback to interactive selector flow.
  - JSON mode: fail with stable ambiguity errors.
- `--cmux` accepts:
  - direct mapped cmux workspace ID
  - `workspace:<n>` ordinal handle (per kra workspace mapping entry ordinal)

## Non-interactive JSON Rule

- `--format json` does not open interactive selectors.
- If target cannot be uniquely resolved from flags, return
  `non_interactive_selection_required`.

## Success Effects

- Run cmux workspace selection for resolved target.
- Update `last_used_at` for the selected mapping entry.

## JSON Response

Success:
- `ok=true`
- `action=cmux.switch`
- `workspace_id`
- `result.kra_workspace_id`
- `result.cmux_workspace_id`
- `result.ordinal`
- `result.title`

Error:
- `ok=false`
- `action=cmux.switch`
- `workspace_id` (if resolved)
- `error.code`
- `error.message`
