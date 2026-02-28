---
title: "`kra shell completion`"
status: implemented
---

# `kra shell completion [<shell>]`

## Purpose

Print shell completion script for `kra`.
(`kra shell init --with-completion` may embed this script inline.)

## Inputs

- `<shell>` (optional): `zsh`, `bash`, `sh`, `fish`
  - when omitted, detect from `$SHELL`
  - when detection fails, fallback to `zsh`

## Behavior

- Print completion script to stdout.
- Top-level suggestions include root commands and global flags.
- Flag suggestions are context-aware:
  - command root only shows primary flags
  - action-specific flags are suggested after subcommand path is fixed
- Subcommand suggestions are provided for:
  - `context`
  - `repo`
  - `template`
  - `shell`
  - `ws`
- Unsupported shell names fail with usage error.

## Usage examples

- zsh:
  - `source <(kra shell completion zsh)`
- bash:
  - `source <(kra shell completion bash)`
- fish:
  - `kra shell completion fish | source`
