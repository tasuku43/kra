---
title: "Debug logging"
status: implemented
---

# Debug logging

## Activation

Debug logging is enabled only when `--debug` is provided.

## Output location

- Directory: `<KRA_ROOT>/.kra/logs/`
- File name pattern: `<UTC timestamp>-<pid>-<scope>.log`

## Format

One line per event:

- UTC timestamp (`RFC3339Nano`)
- free-form message text

Typical events:

- command start/end
- per-command scope marker
- internal execution traces for diagnostics
- timing-capable commands may emit phase timing lines:
  - `phase=<name>`
  - `elapsed_ms=<duration>`
  - optional command-specific fields such as `scope`, `workspace`, `count`, `format`

## Operator notes

- Debug logs are local diagnostic artifacts.
- Logs may contain file paths and command context; mask sensitive values before sharing.
