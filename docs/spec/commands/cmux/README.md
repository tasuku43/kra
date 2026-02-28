---
title: "`kra cmux` command group"
status: implemented
---

# `kra cmux`

## Purpose

Provide a dedicated command group for cmux integration flows without changing
existing `kra ws --act go` behavior.

## Subcommands

- `kra cmux open`
- `kra cmux switch`
- `kra cmux list`
- `kra cmux status`

## Behavior

- `kra cmux --help` prints command-group usage.
- `kra cmux <subcommand> --help` prints subcommand usage.
- Unknown subcommands fail with usage (`exitUsage`).
- `open` is implemented in this phase.
- `switch` / `list` / `status` remain unimplemented and return
  `not implemented` (`exitNotImplemented`).

## Notes

- `open` semantics are defined in `docs/spec/commands/cmux/open.md`.
