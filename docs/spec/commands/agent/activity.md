---
title: "`kra agent` activity tracking"
status: planned
---

# `kra agent` activity tracking (v3 draft)

## Purpose

Provide an operational command and runtime contract to observe agent execution across workspaces with low-latency
runtime visibility and no Git-noise in `KRA_ROOT`.

## Scope (v3 draft)

- Command boundary is `kra agent ...`.
- Runtime architecture reference:
  - `docs/spec/concepts/agent-runtime.md`
- Command surface:
  - `kra agent run`
  - `kra agent attach`
  - `kra agent stop`
  - `kra agent list` (`ls` alias)
  - `kra agent board` (human-first grouped view)
- Discoverability policy:
  - `kra agent` is executable directly.
  - root help intentionally does not list `agent`.
- Runtime state location:
  - `KRA_HOME/state/agents/<root-hash>/<session-id>.json` (snapshot)
  - `KRA_HOME/state/agents/<root-hash>/events/<session-id>.jsonl` (append-only events)
  - `KRA_HOME` default is `~/.kra`
  - runtime state is intentionally outside `KRA_ROOT` to avoid Git churn

## Runtime record schema (session file)

- Required fields:
  - `session_id`
  - `root_path` (absolute canonical `KRA_ROOT`)
  - `workspace_id`
  - `execution_scope` (`workspace` | `repo`)
  - `repo_key` (empty for `workspace` scope)
  - `kind`
  - `pid`
  - `started_at`
  - `updated_at`
  - `seq` (monotonic increasing per session file)
  - `runtime_state` (`running` | `idle` | `exited` | `unknown`)
  - `exit_code` (nullable; set when `runtime_state=exited`)
  - `launch_mode` (`default` | `resume` | `continue`)
  - `attached_clients` (count)
  - `writer_owner` (nullable client id)
  - `lease_expires_at` (nullable unix ts)

## State write rules

- Snapshot:
  - one session = one file
  - writes must be atomic:
    - write to temp file in same directory
    - `fsync` temp file
    - rename temp -> target
- Events:
  - append-only JSONL per session
  - event order follows session-local sequence
- Readers must tolerate partial churn by directory scanning + per-file parse isolation.

## Runtime state model (operator-facing)

- Internal runtime states:
  - `running`: child process alive and recent PTY I/O observed
  - `idle`: child process alive but PTY I/O quiet beyond threshold
  - `exited`: child process exited
  - `unknown`: cannot determine reliably (stale/malformed/unreachable process info)
- Human display mapping:
  - `running` -> `running`
  - `idle` -> `idle`
  - `exited` -> `stopped`
  - `unknown` -> `error`
- Input ownership:
  - multiple clients may attach
  - only current writer lease owner may send input
  - takeover is allowed and must be event-logged

## `kra agent list` / `kra agent board`

- Data source:
  - directory scan of `KRA_HOME/state/agents/<root-hash>/`
  - missing directory means empty list
- `list` output contract:
  - `tsv` is flat machine-friendly rows
  - `human` is workspace-first summary view
  - `human` always renders per-session tree rows under each workspace
  - child row order is deterministic: `workspace` scope first, then `repo:<repo_key>`
- `board` is human-first grouped output:
  - parent row: workspace
  - child row: execution location (`workspace` or `repo:<repo_key>`)
  - default workspace order: lexical fixed order
- Both commands must support filtering by:
  - workspace id
  - runtime state
  - execution location (`workspace`/`repo key`)
  - kind

## Out of scope (v3 draft)

- Cross-host process supervision guarantees.
- Distributed remote control plane across hosts.
