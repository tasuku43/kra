---
title: "`kra serve` (removed)"
status: implemented
---

# `kra serve` (removed)

## Status

`kra serve` was removed by CMUX-016. It is no longer a command, help target, or shell-completion
candidate. Invoking it follows the normal unknown-command path and starts no local server.

The former read-only workspace board, HTTP routes, JSON endpoints, and GitHub pull-request lookup are
not part of the supported runtime surface.

## Supported alternatives

- Use cmux Workspace Groups for root/workspace runtime organization.
- Use cmux native Todo through `kra ws task sync` for explicit task reconciliation.
- Use `kra ws status`, `kra ws task list`, and `kra ws dashboard` for terminal inspection.

There is no compatibility alias or deprecation shim for the removed web server.
