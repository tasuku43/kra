---
title: "`gionx shell init`"
status: implemented
pending:
  - shell_action_file_protocol
---

# `gionx shell init [<shell>]`

## Purpose

Print shell integration script that enables `gionx ws go` to change parent shell cwd.

## Inputs

- `<shell>` (optional): shell name (`zsh`, `bash`, `sh`, `fish`)
  - when omitted, detect from `$SHELL`
  - when detection fails, fallback to `zsh`

## Behavior

- Print eval-ready script to stdout.
- Script contains:
  - one-time setup hint comment
  - shell function `gionx` override
  - special path for `ws go`:
    - execute `command gionx ws go ...`
    - `eval` returned `cd '<path>'`
  - all other commands delegate to `command gionx "$@"`
- Unsupported shell names must fail with usage error.

Planned extension:
- To support command-internal action branching in unified launcher flows, shell integration may adopt
  post-exec action protocol (`action file`) instead of pre-arg `ws go` routing.
- User-visible behavior (running `gionx` wrapper and landing in target directory on go action) must remain stable.

## Output examples

- POSIX shells:
  - `eval "$(gionx shell init zsh)"`
- fish:
  - `eval (gionx shell init fish)`
