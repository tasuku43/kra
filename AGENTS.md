# Agent Instructions (gionx)

## Language

- Respond in Japanese, concisely and politely.

## Next Session Entry Point

When starting a new session, do this first:

- Read `docs/START_HERE.md` (session checklist + doc map)
- Then follow the critical path in `docs/BACKLOG.md`
- For each backlog item, open the linked spec files and implement spec-first

## Autonomous Execution Policy

When working from `docs/BACKLOG.md`:

- Once you start a backlog item, continue implementing until it satisfies the project's "done" definition
  (code + tests + spec `status: implemented`). Do not pause mid-item even if it takes 10-20 minutes.
- Only stop if you hit a spec ambiguity that must be resolved via spec changes before implementation.
  Prefer detecting and resolving ambiguity before you touch code (spec-first).

## Git Commit Scope Policy (no unrelated changes)

This project has two different "commit scopes":

1) **Development commits (inside this `gionx` repo)**
- Keep commits small and slice-aligned to the backlog item.
- Stage only the paths you intended to change for that slice.

2) **User-data commits (commands that commit inside `GIONX_ROOT`)**
- Many `gionx` commands intentionally commit changes in the user's `GIONX_ROOT`.
- The staging scope must be an allowlist derived from the command + arguments (typically fixed prefixes like
  `workspaces/<id>/` or `archive/<id>/`).
- Implementation rule:
  - stage only the allowlisted prefixes (e.g. `git add -A -- <prefixes...>`)
  - then verify the staged paths (`git diff --cached --name-only`) are a strict subset of the allowlist
  - if anything outside the allowlist is staged, abort the command (do not commit)

## Asking Questions

When asking the user to make a decision:

- Provide 2-3 concrete options.
- For each option, explicitly state trade-offs (pros/cons, risks, impact on UX/complexity/maintenance).
- Then ask the user to pick (e.g. by number).

Avoid asking with options only (no trade-offs).
