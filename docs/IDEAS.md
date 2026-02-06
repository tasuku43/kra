---
title: "gionx ideas"
status: idea
updated: 2026-02-06
---

# Ideas (Not Scheduled Yet)

This file is a place to capture ideas that are *not* in the backlog yet, but are part of the current direction.
Nothing here is a spec (`docs/spec/**`) or an implementation commitment.

## 1) Jira Integration: Ticket URL → Workspace Creation

### Goal

- Accept a Jira ticket URL (e.g. `https://<your-domain>/browse/PROJ-123`) and create a `gionx` workspace for it.
- Also support “create workspaces for all tickets in a sprint” as a bulk operation.

### Minimal baseline flow (proposal)

- Input: ticket URL
- Fetch from Jira API: minimum metadata such as `issueKey`, `summary`, `status`, `assignee`
- Create a workspace equivalent to `gionx ws create` (naming can be provisional, but must handle collisions/duplicates)
- Persist an “external reference (Jira)” link on the workspace for future sync and UI display

### Sprint bulk creation (extension)

- Input: sprint identifier (board/sprint ID, or URL)
- Enumerate all tickets in the sprint and create only missing workspaces (idempotency is key)
- Failure behavior: stop-on-first-error vs allow partial success; safe re-run and recovery story

### Open questions (intentionally deferred)

- Workspace shape (worktrees, templates, bootstrapped files, etc.)
- Auth strategy (PAT / OAuth / keychain / env) and secure storage
- Naming rules (e.g. `PROJ-123-<slug>`) and collision resolution
- What Jira metadata to store in the state DB (minimal vs future-proof)
- Rate limiting, incremental sync, and tracking changes (moved issues, sprint changes, deletions)

## 2) Workspace List TUI: “Check Off” Work Like a ToDo

### Goal

- Provide a polished TUI for the workspace list (think: checking off items in a ToDo list).
- By default, checked items are archived (initially: equivalent to `ws close`).

### Experience sketch

- List view (filter, search, sort)
- Toggle selection with `Space` (or similar)
- Default action: checked → archive (close)
- Safety: undo, confirmations, dry-run, and a preview for batch operations

### Open questions

- UI framework choice (defer; specify the UX and state transitions first)
- Whether to support actions beyond “check=close” (done / snooze / defer)
- Ensuring the interaction model is safe given `close` can involve Git commits in the root

## 3) View: Agent Activity Across Workspaces

Examples:
- Claude Code / Codex CLI / Cursor CLI / Gemini CLI (assume more in the future)

### Goal

- See “which agent is running in which workspace” and “what state it is in” from a single view.
- Make long-running work visible and reduce conflicts (e.g. multiple agents operating in the same workspace).

### Minimal approach (proposal)

- Encourage running agents via `gionx`, recording metadata at start
  - Example: `gionx agent run --ws <id> -- <agent-command...>`
- Candidate fields: `workspace_id`, `agent_kind`, `started_at`, `last_heartbeat_at`,
  `status (running/succeeded/failed/unknown)`, `log_path`
- Display via TUI/CLI (may be integrated into the workspace list)

### Open questions

- Execution model (PID tracking, tmux integration, log capture, heartbeats, crash detection)
- How to treat agents started outside of `gionx` (accept as unobservable vs attempt discovery)
- Security/privacy (logs may contain sensitive data)

## How to turn these into specs/backlog (proposal)

- Start by specifying “what we store in state (DB)” and the “command boundaries” in `docs/spec/**`.
- Only move items into `docs/BACKLOG.md` once ambiguity is reduced enough to define “done” precisely.
