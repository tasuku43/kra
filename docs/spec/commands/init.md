---
title: "`gionx init`"
status: implemented
---

# `gionx init [--root <path>]`

## Purpose

Initialize a gionx root and filesystem-first runtime metadata.

## Root selection policy

- `--root <path>` is an explicit non-interactive root selector.
- Without `--root`:
  - TTY: ask interactively for root path.
  - default suggestion: `~/gionx`
  - non-TTY: fail fast and require `--root`.
- If selected root path does not exist, create it automatically (when parent exists).

## Behavior

- Ensure `<root>/` exists (create if missing)
- Ensure `<root>/workspaces/` exists
- Ensure `<root>/archive/` exists
- Create `<root>/AGENTS.md` with a default "how to use gionx" guidance (for the no-template MVP)
  - include a short explanation of `notes/` vs `artifacts/`
- If `<root>` is not a Git repo, run `git init`
- When `git init` is newly performed by `gionx init`, create an initial commit containing:
  - `<root>/.gitignore`
  - `<root>/AGENTS.md`
- Write `.gitignore` such that `workspaces/**/repos/**` is ignored
- Touch root registry metadata for this root.
- Update global current context (`current-context`) to this root on success.

## Notes

- If selected root is already Git-managed, `gionx init` must not overwrite existing Git settings.
- Re-running `gionx init` on an already initialized root is idempotent success.

## Output

- Success output must use shared section style:
  - `Result:`
  - `  Initialized: <root>`
- `Result:` heading style follows shared UI token rules (`text.primary` + bold).
- Success line should use shared success semantics (`status.success`).
