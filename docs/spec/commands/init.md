---
title: "`gionx init`"
status: implemented
---

# `gionx init [--root <path>] [--context <name>]`

## Purpose

Initialize a gionx root and filesystem-first runtime metadata.

## Root selection policy

- `--root <path>` is an explicit non-interactive root selector.
- Without `--root`:
  - TTY: ask interactively for root path.
  - default suggestion: `~/gionx`
  - non-TTY: fail fast and require `--root`.
- If selected root path does not exist, create it automatically (when parent exists).

## Context selection policy

- `--context <name>` is an explicit non-interactive context selector.
- Without `--context`:
  - TTY: ask interactively for context name.
  - default suggestion: current directory basename
  - non-TTY: fail fast and require `--context`.
- `init` always selects the context on success (no `use now?` confirmation).

## Behavior

- Ensure `<root>/` exists (create if missing)
- Ensure `<root>/workspaces/` exists
- Ensure `<root>/archive/` exists
- Ensure `<root>/templates/default/` exists on first init
  - create:
    - `<root>/templates/default/notes/`
    - `<root>/templates/default/artifacts/`
    - `<root>/templates/default/AGENTS.md`
  - do not overwrite existing `templates/default/` content
- Ensure `<root>/.gionx/config.yaml` exists on first init
  - create default content:
    - `workspace.defaults.template = default`
    - include comment header about config precedence/order
  - do not overwrite existing file content
- Create `<root>/AGENTS.md` with a default "how to use gionx" guidance (for the no-template MVP)
  - include a short explanation of `notes/` vs `artifacts/`
- If `<root>` is not a Git repo, run `git init`
- When `git init` is newly performed by `gionx init`, create an initial commit containing:
  - `<root>/.gitignore`
  - `<root>/.gionx/config.yaml`
  - `<root>/AGENTS.md`
  - `<root>/templates/default/AGENTS.md` (if created)
- Write `.gitignore` such that `workspaces/**/repos/**` is ignored
- Touch root registry metadata for this root.
- Register/refresh context binding (`name -> root`) in root registry.
- Update global current context (`~/.gionx/state/current-context`) to this root on success.

## Notes

- If selected root is already Git-managed, `gionx init` must not overwrite existing Git settings.
- Re-running `gionx init` on an already initialized root is idempotent success.

## Output

- Success output must use shared section style:
  - `Result:`
  - `  Initialized: <root>`
  - `  Context selected: <name>`
- `Result:` heading style follows shared UI token rules (`text.primary` + bold).
- Success line should use shared success semantics (`status.success`).
