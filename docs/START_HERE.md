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

2) Open the related spec files listed in the backlog item
- Confirm the behavior is fully specified.
- If anything is ambiguous, update the spec first (spec-driven workflow).

3) Implement with tight commit loops
- Small commits, each mapped to one backlog item or a small sub-slice.
- Prefer adding tests for non-happy-path / drift scenarios early.

4) Mark progress in docs
- Update spec `status` when a behavior is implemented.
- Keep `docs/BACKLOG.md` current (check off items only when done).

## Where to look for key decisions

- Backlog / dependencies: `docs/BACKLOG.md`
- Data model: `docs/spec/core/DATA_MODEL.md`
- State store / migrations: `docs/spec/concepts/state-store.md` and `migrations/*.sql`
- Root layout + git tracking: `docs/spec/concepts/layout.md`
- Testing principles (dev guidance): `docs/dev/TESTING.md`

