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

## Go Formatting (required)

- Before committing (and before opening/updating a PR), ensure `gofmt` is clean:
  - run: `gofmt -w $(gofmt -l .)`
  - verify: `test -z "$(gofmt -l .)"`
- If CI has a `gofmt` step, treat it as a hard requirement (fix formatting first).

## Minimum Quality Gate (required)

Before committing (and before opening/updating a PR), run the minimum checks:

- Formatting: `test -z "$(gofmt -l .)"`
- UI color rules: `./scripts/lint-ui-color.sh`
- Static checks: `go vet ./...`
- Tests: `go test ./...`

If any check fails, fix it before pushing.

## UI Color Guardrail (required)

To keep CLI/TUI output consistent across sessions:

- Follow `docs/spec/concepts/ui-color.md` for all user-facing output.
- Do not introduce raw ANSI color codes outside `internal/cli/ws_ui_common.go`.
- Do not introduce direct `lipgloss.Color(...)` usage in command handlers/renderers.
- Use shared semantic style helpers (`styleTokenize`, `styleMuted`, `styleAccent`, etc.) from
  `internal/cli/ws_ui_common.go`.
- If you need a new visual meaning, add/update semantic token definitions first, then apply them.

## WS Flow Guardrail (required)

For workspace selector commands, architecture must stay consistent across sessions.

- Selector-capable `runWS*` handlers MUST use the shared flow orchestrator:
  - `runWorkspaceSelectRiskResultFlow` in `internal/cli/ws_flow.go`
- Do not implement bespoke stage transitions (`Workspaces -> Risk -> Result`) directly inside command handlers.
- Do not bypass shared selector entrypoint:
  - use `promptWorkspaceSelector` (do not call lower-level selector runtime directly from handlers)
- Guard tests enforce this architecture. If they fail, align implementation instead of relaxing tests.

## Asking Questions

When asking the user to make a decision:

- Provide 2-3 concrete options.
- For each option, explicitly state trade-offs (pros/cons, risks, impact on UX/complexity/maintenance).
- Then ask the user to pick (e.g. by number).

Avoid asking with options only (no trade-offs).

## Session Wrap-up Output (required)

At the end of a work session (or after completing a backlog item), always output:

- What you completed (commits + backlog item ids).
- The next **Serial** candidate to start.
- 1-3 **Parallel** candidates (if any are unblocked).
- Conditional guidance for parallel work, using "if X is done, then Y is unblocked" wording, because you may
  not know what other sessions have completed.

How to decide "next":
- Treat `docs/BACKLOG.md` as the source of truth for items + dependencies.
- If you are not sure whether an item is already complete, check whether its linked spec frontmatter is
  `status: implemented` and whether the code/tests exist. If still uncertain, phrase the recommendation
  conditionally.
