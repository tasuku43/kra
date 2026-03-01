---
title: "kra version"
status: implemented
---

## Synopsis

`kra version`

Also available as a global flag:
`kra --version`

## Intent

Print build version metadata in a single line for support/debugging.

## Behavior

- `kra --version` prints the version and exits 0.
- `kra version` prints the same output and exits 0.
- Output format is a single line:
  - `<version> [<commit>] [<date>]`
- Default build values:
  - when not set via `-ldflags`, `<version>` defaults to `dev`
  - `<commit>` and `<date>` are omitted when empty

## Success criteria

- Prints one line to stdout.
- Exits 0.
