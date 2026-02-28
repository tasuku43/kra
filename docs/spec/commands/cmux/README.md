---
title: "`kra cmux` command group"
status: implemented
---

# `kra cmux`

## Purpose

Provide a dedicated command group for cmux integration flows without changing
existing workspace action behavior.

## Subcommands

- `kra cmux open`
- `kra cmux switch`
- `kra cmux list`
- `kra cmux status`

## Behavior

- `kra cmux --help` prints command-group usage.
- `kra cmux <subcommand> --help` prints subcommand usage.
- Unknown subcommands fail with usage (`exitUsage`).
- `open`, `switch`, `list`, and `status` are implemented in this phase.

## Notes

- `open` semantics are defined in `docs/spec/commands/cmux/open.md`.
- `open --multi` semantics are defined in `docs/spec/commands/cmux/open-multi.md`.
- `switch` semantics are defined in `docs/spec/commands/cmux/switch.md`.
- `list` semantics are defined in `docs/spec/commands/cmux/list.md`.
- `status` semantics are defined in `docs/spec/commands/cmux/status.md`.
