# Agent Instructions (kra)

## Language

- Respond in Japanese, concisely and politely.

## Next Session Entry Point

When starting a new session, do this first:

- Read `docs/backlog/README.md`
- For each backlog item, open the linked spec files and implement spec-first

## Autonomous Execution Policy

When working from `docs/backlog/README.md`:

- Once you start a backlog item, continue implementing until it satisfies the project's "done" definition
  (code + tests + spec `status: implemented`). Do not pause mid-item even if it takes 10-20 minutes.
- Only stop if you hit a spec ambiguity that must be resolved via spec changes before implementation.
  Prefer detecting and resolving ambiguity before you touch code (spec-first).

## Backlog Management

- Backlog is managed under `docs/backlog/` by epic/prefix (`docs/backlog/*.md`).
- Always start from `docs/backlog/README.md` (index + file status).
- Keep `docs/backlog/README.md` `File Status` in sync when ticket checkboxes change in any backlog file.

## Git Commit Scope Policy (no unrelated changes)

This project has two different "commit scopes":

1) **Development commits (inside this `kra` repo)**
- Keep commits small and slice-aligned to the backlog item.
- Stage only the paths you intended to change for that slice.

2) **User-data commits (commands that commit inside `KRA_ROOT`)**
- Many `kra` commands intentionally commit changes in the user's `KRA_ROOT`.
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
