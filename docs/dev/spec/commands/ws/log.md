---
title: "`kra ws log`"
status: implemented
---

# `kra ws log [--id <id> | --current] [--] <message>`

## Purpose

Append one text log line into workspace-local `log.txt`.

## Behavior

- target workspace resolution:
  - `--id <id>`: resolve from `workspaces/<id>/` or `archive/<id>/`
  - `--current` or no target flag: resolve from current path under
    `workspaces/<id>/...` or `archive/<id>/...`
- append `<message>` to:
  - `<workspace>/log.txt`
  - write mode: create-if-missing + append
  - newline is appended at the end of each command call

## cmux mirror

- if `CMUX_WORKSPACE_ID` is set, execute:
  - `cmux log --level info "<message>"`
- cmux mirror is best-effort:
  - failure in cmux mirror must not rollback or fail workspace file append
