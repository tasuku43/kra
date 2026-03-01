---
title: "Shell integration guide"
status: implemented
---

# Shell integration guide

This guide explains how to enable shell integration for `kra`.

## Why this is needed

Some commands (for example `kra ws open`) may request parent-shell side effects such as `cd`.
Without shell integration, `kra` still runs commands, but it cannot mutate your parent shell cwd.

## Enable integration

```sh
# zsh
eval "$(kra shell init zsh)"

# bash
eval "$(kra shell init bash)"

# fish
eval (kra shell init fish)
```

To persist this, add the corresponding line to your shell rc file:

- `~/.zshrc`
- `~/.bashrc`
- `~/.config/fish/config.fish`

## Optional: include completion in one step

```sh
kra shell init zsh --with-completion
kra shell init bash --with-completion
kra shell init fish --with-completion
```

## Notes

- `kra shell init <shell>` prints shell code to stdout.
- You can review the output script before evaluating it.
- Unsupported shell names fail with usage error.

## Related docs

- Command overview: `docs/user/guides/COMMANDS.md`
- Shell command contract: `docs/dev/spec/commands/shell/init.md`
