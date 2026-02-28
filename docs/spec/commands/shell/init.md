---
title: "`kra shell init`"
status: implemented
---

# `kra shell init [<shell>] [--with-completion[=true|false]]`

## Purpose

Print shell integration script that applies parent-shell side effects via action-file protocol.
Shell completion script generation is handled separately by `kra shell completion`.

## Inputs

- `<shell>` (optional): shell name (`zsh`, `bash`, `sh`, `fish`)
  - when omitted, detect from `$SHELL`
  - when detection fails, fallback to `zsh`
- `--with-completion` (optional): append shell completion script in the same output
  - default: `false`
  - accepted values via `--with-completion=<value>`: `true/false`, `1/0`, `yes/no`, `on/off`

## Behavior

- Print eval-ready script to stdout.
- Script contains:
  - one-time setup hint comment
  - shell function `kra` override
  - for all command paths, set `KRA_SHELL_ACTION_FILE=<tempfile>` and let `kra` emit post-exec action
    (for example `cd '<path>'`) into that file when needed
  - after command success, apply action file content if present
- Unsupported shell names must fail with usage error.
- When `--with-completion` is enabled, append the same shell-specific script as `kra shell completion <shell>`.

## Output examples

- POSIX shells:
  - `eval "$(kra shell init zsh)"`
  - `eval "$(kra shell init zsh --with-completion)"`
- fish:
  - `eval (kra shell init fish)`
  - `eval (kra shell init fish --with-completion)`
