---
title: "`kra agent attach` session reattach"
status: planned
---

# `kra agent attach` v3 draft

## Purpose

Attach current terminal to an existing broker-managed agent session.

This command is for returning to an already running session from the current
workspace/repo context, not for global manager discovery.

## Scope (v3 draft)

- Command:
  - `kra agent attach [--session <id>]`
- Behavior:
  - resolve current `KRA_ROOT`
  - resolve current context scope from `cwd`
  - connect broker socket for the root hash
  - attach terminal stream to the selected session PTY stream
- Session selection:
  - if `--session` is set:
    - attach directly, fail if not found
  - if `--session` is omitted:
    - query broker for candidate sessions in current scope
    - prompt interactive selector

## Context Resolution Rules

- Inside `workspaces/<id>/repos/<repo-key>/...`:
  - candidate scope = same `workspace + repo`
- Inside `workspaces/<id>/...`:
  - candidate scope = same workspace
- At `KRA_ROOT` root:
  - fail (context too broad for `attach`)
- Outside `KRA_ROOT`:
  - fail

## Input and Lease Rules

- Multi-attach is allowed (multiple viewers).
- Input is allowed only for current writer lease owner.
- If caller has no lease and sends input:
  - prompt takeover confirmation
  - on confirm, perform immediate takeover
- Dangerous keys (`Ctrl-C`, `Ctrl-D`, `Ctrl-Z`) require confirmation.

## Output Contract

- Success:
  - terminal enters attached interactive stream
- Errors:
  - clear reason + next action (missing broker, session not found, invalid context)
  - non-zero exit code

## Out of scope (v3 draft)

- Cross-root global attach selector from `attach` command.
- Remote host attach.
- Provider-specific conversation history browsing.
