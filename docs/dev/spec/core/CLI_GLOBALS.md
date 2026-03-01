---
title: "Global CLI behavior"
status: implemented
---

# Global CLI behavior

## Command form

`kra [global-flags] <command> [args]`

## Global flags

- `--debug`
  - Enables debug logging.
  - Command traces are written under `<KRA_ROOT>/.kra/logs/`.
- `--version`
  - Prints version and exits 0.
  - Does not run subcommand logic.
- `--help` / `-h`
  - Prints root usage and exits 0.

## Version output

- `kra version` and `kra --version` print the same line.
- Build metadata fields:
  - `version`
  - `commit` (optional)
  - `date` (optional)
