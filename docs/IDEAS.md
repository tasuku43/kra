---
title: "gionx ideas"
status: idea
updated: 2026-02-09
---

# Ideas (Not Scheduled Yet)

This file is a place to capture ideas that are *not* in the backlog yet, but are part of the current direction.
Nothing here is a spec (`docs/spec/**`) or an implementation commitment.

## 1) View: Agent Activity Across Workspaces

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

## 2) Logical Work State in Active Workspaces (`todo` vs `in-progress`)

### Goal

- Keep physical workspace lifecycle as-is: `active` and `archive`.
- Within `active`, derive a logical work state (`todo` or `in-progress`) at read time.
- Make status visible in `gionx ws list` and actionable commands (e.g. `go`) so users can quickly tell
  whether work has started.

### Constraints / principles

- Do not persist this logical work state in the DB.
- Compute it from observable signals at runtime (derived state only).
- Keep the classification deterministic and explainable (avoid opaque heuristics).

### Candidate signals (proposal)

- Workspace-side Git activity:
  - local commit history since workspace creation or since last lifecycle event
  - modified/untracked files in the workspace repos
- Repo-level activity hints:
  - whether branches/worktrees under `repos/` show active development movement
  - optional comparison against base/default branch for "work has diverged"

### Open questions

- Exact decision rule and precedence when signals disagree (e.g. clean tree but local commits exist)
- Performance budget for list rendering when many workspaces/repos are present
- UX wording in list/go flows (`todo`, `in-progress`, or symbols/colors) and fallback when unknown

## How to turn these into specs/backlog (proposal)

- Start by specifying “what we store in state (DB)” and the “command boundaries” in `docs/spec/**`.
- Only move items into `docs/backlog/*.md` once ambiguity is reduced enough to define “done” precisely.
