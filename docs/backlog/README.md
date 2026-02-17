---
title: "kra backlog index"
status: planned
---

# Backlog Index

This is the single entrypoint for backlog operation.
Backlog is managed by epic/prefix under `docs/backlog/*.md`.

## Session Startup Checklist

1. Check working tree status first.
2. Pick the next ticket from this backlog set:
   - prefer the smallest-numbered unblocked **Serial** item
   - if parallelizing, pick an unblocked **Parallel** item
3. Open linked specs and confirm behavior before coding.
4. If spec is ambiguous, fix spec first.
5. Implement in small, ticket-aligned slices.
6. Update ticket checkboxes and `File Status` after completion.

## Key Docs

- Data model: `docs/spec/core/DATA_MODEL.md`
- State store / registry: `docs/spec/concepts/state-store.md`, `docs/spec/commands/state/registry.md`
- Root layout + git tracking: `docs/spec/concepts/layout.md`
- Testing principles: `docs/dev/TESTING.md`

## Files

- `docs/backlog/FS-STATE.md`: FS-first migration
- `docs/backlog/MVP.md`: MVP foundation, lifecycle, hardening
- `docs/backlog/UX-WS.md`: workspace UX and unified entry
- `docs/backlog/UX-REPO.md`: repo UX polish
- `docs/backlog/UX-CORE.md`: core UX polish
- `docs/backlog/ARCH.md`: architecture refactor
- `docs/backlog/INT-JIRA.md`: external integration (Jira)
- `docs/backlog/CTX-ROOT.md`: context and root management
- `docs/backlog/CONFIG.md`: global/root config and runtime path policy
- `docs/backlog/DOC-QUALITY.md`: documentation and quality hardening
- `docs/backlog/AGENT.md`: agent execution visibility
- `docs/backlog/TEMPLATE-WS.md`: workspace template lifecycle and validation
- `docs/backlog/OPS.md`: operational resilience and automation ergonomics
- `docs/backlog/PUBLIC.md`: public release readiness
- `docs/backlog/PROVIDER.md`: provider extensibility

## File Status

- [x] `docs/backlog/FS-STATE.md` (`8/8` done)
- [x] `docs/backlog/MVP.md` (`19/19` done)
- [x] `docs/backlog/UX-WS.md` (`30/30` done)
- [x] `docs/backlog/UX-REPO.md` (`2/2` done)
- [x] `docs/backlog/UX-CORE.md` (`13/13` done)
- [x] `docs/backlog/ARCH.md` (`10/10` done)
- [x] `docs/backlog/INT-JIRA.md` (`7/7` done)
- [x] `docs/backlog/CTX-ROOT.md` (`4/4` done)
- [x] `docs/backlog/CONFIG.md` (`6/6` done)
- [x] `docs/backlog/DOC-QUALITY.md` (`5/5` done)
- [ ] `docs/backlog/AGENT.md` (`5/8` done)
- [x] `docs/backlog/TEMPLATE-WS.md` (`4/4` done)
- [ ] `docs/backlog/OPS.md` (`12/14` done)
- [x] `docs/backlog/PUBLIC.md` (`7/7` done)
- [ ] `docs/backlog/PROVIDER.md` (`0/1` done)

Update this section whenever any ticket checkbox changes.

## Definition of done (per backlog item)

Do not treat an item as "done" until all of the following are true:

- Code exists and behavior matches the linked specs.
- Tests exist, including at least some non-happy-path coverage (see `docs/dev/TESTING.md`).
- The linked spec frontmatter is updated to `status: implemented`.

Special note (commands that commit inside `KRA_ROOT`):
- The spec must define the staging allowlist (which path prefixes may be staged/committed).
- The implementation must enforce it (stage allowlist only, verify staged paths are within allowlist, abort otherwise).

## How to pick the next item (dynamic)

1. If the working tree is dirty, finish that slice first (or explicitly park it with a WIP commit).
2. Prefer the smallest-numbered **Serial** item whose dependencies are satisfied.
3. If you want parallel work, pick a **Parallel** item that is unblocked by dependencies.

Tip:
- An item is "done" only when its linked specs are updated to `status: implemented` and the code/tests exist.

## Reporting next steps (required)

When you finish a backlog item, include a short next-steps report in your final output:

- Next Serial candidate (smallest-numbered Serial whose deps are satisfied).
- Parallel candidates (unblocked items).
- Conditional guidance: "If <deps> are done, then <item> is ready."

Recommended (workspace handoff):

```sh
gion manifest add --no-apply --repo git@github.com:tasuku43/kra.git MVP-001
gion plan
gion apply --no-prompt
```

Rules:
- Each backlog item maps to one or more spec files in `docs/spec/**`.
- Dependencies are explicit so we can see what must be serial vs what can be parallel.
- When an item is complete, update the related spec file frontmatter to `status: implemented`.

Legend:
- **Serial**: on the critical path (blocks other items).
- **Parallel**: can be implemented independently once its dependencies are done.

## Parallelizable groups (guide)

- After MVP-001:
  - MVP-002 and MVP-003 can proceed in parallel.
- After MVP-002 + MVP-003:
  - MVP-010 can proceed (usually best to do early).
  - MVP-030 can proceed in parallel (repo pool is used later by add-repo/reopen).
- After MVP-010:
  - MVP-020 and MVP-021 can proceed in parallel.
- After MVP-020 + MVP-030 + MVP-010:
  - MVP-031 unblocks the archive lifecycle commands (MVP-040/041/042).
