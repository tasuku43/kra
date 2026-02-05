# Start Here

This document is the fastest "next session" entrypoint for Codex CLI (and humans).

## One-minute orientation

- Specs live in `docs/spec/**` and are the source of truth.
- The implementation backlog lives in `docs/BACKLOG.md`.
- For every implemented backlog item:
  - implement the code + tests
  - then update the related spec frontmatter `status: implemented`

## Session startup checklist

1) Read `docs/BACKLOG.md`
- Pick the next **Serial** item on the critical path.
- If you plan to work in parallel, pick a **Parallel** item that is unblocked by dependencies.

1.5) Quick reality check (avoid stale assumptions)

- If the working tree is dirty, either:
  - finish and commit the slice, or
  - explicitly park it with a WIP commit (do not mix unrelated changes).

2) Open the related spec files listed in the backlog item
- Confirm the behavior is fully specified.
- If anything is ambiguous, update the spec first (spec-driven workflow).
- For commands that commit inside `GIONX_ROOT`, confirm the spec clearly defines the staging allowlist
  (which path prefixes may be staged/committed). If it doesn't, update the spec before writing code.

3) Implement with tight commit loops
- Small commits, each mapped to one backlog item or a small sub-slice.
- Prefer adding tests for non-happy-path / drift scenarios early.

4) Mark progress in docs
- Update spec `status` when a behavior is implemented.
- Keep `docs/BACKLOG.md` current (check off items only when done).

## What "done" means

For this project, a backlog item is complete only when:

- its behavior exists in code (and is usable from the CLI, if applicable)
- it has tests for at least some non-happy-path behavior (see `docs/dev/TESTING.md`)
- the related spec frontmatter is updated to `status: implemented`

## Session wrap-up (required)

Before ending a session, always write a short "next steps" note (in your final output) that includes:

- Next **Serial** item to start (based on `docs/BACKLOG.md` dependencies).
- 1-3 **Parallel** items that are unblocked (if any).
- Conditional unblocking statements when working in parallel (e.g. "If MVP-002 and MVP-003 are complete,
  then MVP-010 is ready.").

This is required because sessions may run in parallel and you may not know the latest completion status.

## Where to look for key decisions

- Backlog / dependencies: `docs/BACKLOG.md`
- Data model: `docs/spec/core/DATA_MODEL.md`
- State store / migrations: `docs/spec/concepts/state-store.md` and `migrations/*.sql`
- Root layout + git tracking: `docs/spec/concepts/layout.md`
- Testing principles (dev guidance): `docs/dev/TESTING.md`
