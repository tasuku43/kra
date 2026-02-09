---
title: "INT-JIRA backlog"
status: planned
---

# INT-JIRA Backlog

- [ ] INT-JIRA-001: `ws create --jira <ticket-url>` (single issue MVP)
  - What: add strict Jira single-ticket creation path in `ws create`; resolve `id=issueKey` and `title=summary`
    from Jira API using env-only auth, fail-fast on retrieval errors, and disallow `--jira` with `--id/--title`.
    Keep implementation split by architecture layers so future sprint bulk import can reuse app use cases.
  - Specs:
    - `docs/spec/commands/ws/create.md`
    - `docs/spec/testing/integration.md`
  - Depends: ARCH-010, UX-WS-028
  - Serial: yes
