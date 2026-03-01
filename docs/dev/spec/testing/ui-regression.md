---
title: "UI regression testing (CLI human output)"
status: implemented
---

# UI regression testing (CLI human output)

## Purpose

Protect human-facing CLI output from accidental drift (layout, section structure,
prompt placement, summary/result wording, and selector rendering).

## Strategy

Use two complementary layers:

- E2E golden snapshots:
  - run real commands through `CLI.Run`
  - capture `stdout` and `stderr` separately
  - normalize environment-dependent paths
  - compare against golden files
- Component golden snapshots:
  - snapshot shared renderer output (`Plan`, `Result`, selector, list renderer)
  - keep tests deterministic and focused on visual contract

## Scope

### E2E matrix (human mode)

- `init`
- `ws create`
- `ws list`
- `ws close`
- `ws reopen`
- `ws purge --no-prompt --force`
- `ws list --archived`

### Component matrix

- shared selector renderer
- add-repo plan renderer
- ws import jira plan/result renderer
- ws list human renderer
- ws flow result/aborted renderers
- ws close / ws purge risk section renderers

## Contracts

- Keep `stdout`/`stderr` responsibility explicit:
  - prompt lines and interactive guidance must appear on the intended stream
- Preserve section atoms and spacing:
  - heading -> body structure
  - single blank-line rules around sections
- Preserve `ID: title` style consistency across workspace and issue listings.

## Update rule

When intentionally changing UI output, update:

1. this spec (if contract changed),
2. golden files (`UPDATE_GOLDEN=1`),
3. affected command specs.
