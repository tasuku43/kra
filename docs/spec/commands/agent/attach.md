title: "`kra agent attach`"
status: implemented
---

# `kra agent attach`

## Purpose

Attach current terminal to an existing broker-managed agent session.

## Scope (implemented)

- Command:
  - `kra agent attach [--session <id>] [--renderer <auto|raw|vterm>]`
- Behavior:
  - validate target session id and scope (workspace/repo context)
  - resolve current `KRA_ROOT`
  - connect broker socket for the root hash
  - broker replays buffered PTY output for selected session
  - after replay catch-up, switch to live attach stream for selected session PTY
  - renderer selection:
    - `raw`: byte-stream relay (legacy behavior)
    - `vterm`: terminal emulator render path (requires `vterm` build tag)
    - `auto` (default): prefer `vterm` when available, fallback to `raw`
  - `--renderer vterm` on non-vterm builds returns clear error
  - vterm build requires system `libvterm` (`pkg-config vterm`)
- Selection:
  - if `--session` is omitted and stdin is TTY, prompt session selection in scope
  - if `--session` is omitted in non-interactive mode, fail with usage error
- Detach behavior:
  - during attach stream, `Ctrl-C` detaches local terminal only (session keeps running)

## Context Resolution Rules

- Inside `workspaces/<id>/repos/<repo-key>/...`:
  - candidate scope = same `workspace + repo`
- Inside `workspaces/<id>/...`:
  - candidate scope = same workspace
- At `KRA_ROOT` root:
  - fail (context too broad for `attach`)
- Outside `KRA_ROOT`:
  - fail

## Output Contract

- Success:
  - caller terminal enters attached stream until session exits or connection closes
  - replay is emitted first so attached terminal can recover current visual context
- Errors:
  - clear reason + next action (missing broker, session not found, invalid context)
  - non-zero exit code

## Deferred to AGENT-100

- writer lease / takeover control
- dangerous key confirmation (`Ctrl-C`, `Ctrl-D`, `Ctrl-Z`)
