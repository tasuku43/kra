---
title: "`kra agent` activity tracking"
status: implemented
---

# `kra agent` activity tracking

## Purpose

Provide runtime visibility for agent sessions across workspaces with state files under `KRA_HOME` (outside `KRA_ROOT` Git tree).

## Scope (implemented)

- Command boundary: `kra agent ...`
- Command surface:
  - `kra agent run`
  - `kra agent stop`
  - `kra agent board`
  - internal transport primitive (not exposed as direct CLI subcommand):
    - broker attach stream RPC
- Discoverability policy:
  - `kra agent` is executable directly
  - root help intentionally does not list `agent`
- Runtime state location:
  - `KRA_HOME/state/agents/<root-hash>/<session-id>.json`
  - `KRA_HOME` default: `~/.kra`
- Runtime signal events (terminal-sequence subset):
  - `KRA_HOME/state/agents/<root-hash>/events/<session-id>.jsonl`

## Runtime record schema (session file)

- Required fields:
  - `session_id`
  - `root_path`
  - `workspace_id`
  - `execution_scope` (`workspace` | `repo`)
  - `repo_key` (empty for `workspace` scope)
  - `kind`
  - `pid`
  - `started_at`
  - `updated_at`
  - `seq`
  - `runtime_state` (`running` | `waiting_input` | `idle` | `exited` | `unknown`)
  - `exit_code` (nullable; set when `runtime_state=exited`)

## State write rules (implemented)

- one session = one file
- writes are atomic (`tmp -> rename`)
- `seq` is monotonic per session

## Runtime state model (operator-facing)

- `running`: process alive, and recent PTY output activity is observed
- `waiting_input`: process alive, and terminal-sequence completion hints indicate user input wait
- `idle`: process alive, and PTY output is quiet for a short window
- `exited`: process ended
- `unknown`: runtime could not determine state reliably

## Runtime state inference policy (implemented)

- source of truth for live inference is broker-owned PTY output stream
- broker handles two distinct directions:
  - input path: attached client -> broker -> PTY write
  - output path: PTY read -> broker -> attached clients
- runtime state inference uses only output-path activity (PTY read side)
- `running` is driven by output activity:
  - when PTY output bytes arrive, broker updates the session as `running`
- `waiting_input` is driven by terminal-sequence completion hints:
  - when hints like `OSC 133;D`, `OSC 9`, `OSC 777 notify` are observed, broker updates the session as `waiting_input`
- `idle` is driven by output silence:
  - when output remains silent for a short timeout window, broker updates the
    session as `idle`
- snapshots are updated on state change and periodic output heartbeat
- terminal control sequences (`OSC 9` / `OSC 777` / `OSC 133`) are captured as
  signal events for observability, but runtime state is not coupled to
  provider-specific screen text markers
- provider history files outside broker runtime (for example
  `~/.codex/sessions/*.jsonl`) are not read by this command surface

## Runtime signal events (implemented subset)

- broker appends recognized terminal-sequence events to per-session jsonl:
  - `osc_9_notify`
  - `osc_777_notify`
  - `osc_133_c`
  - `osc_133_d`
  - `bell`
- each row contains: `session_id`, `at`, `name`, optional `state_hint`, optional `details`

## `kra agent board`

- Data source:
  - primary: broker `sessions` RPC over root socket (live runtime snapshot)
  - fallback: directory scan of `KRA_HOME/state/agents/<root-hash>/`
  - merge policy (primary success + fallback readable):
    - live rows override same `session_id` persisted rows
    - persisted-only rows are retained (for exited history)
  - missing directory means empty persisted rows
- `board` output contract:
  - `--ui`:
    - full-screen TUI mode (interactive TTY only)
    - live refresh list + detail panes
    - session actions are available from key bindings (implemented: stop)
  - `human`:
    - interactive TTY default: selection flow
      - select one session from filtered scope
      - select action (`show`, `stop`, or `send`) unless `--act` is provided
      - `show`: prints selected session details
      - `stop`: delegates to stop command behavior for selected session
      - `send`: prompts one-line instruction and forwards it to session PTY input
    - non-interactive or `--no-select`: workspace-grouped view
  - `tsv`: machine-friendly flat rows
  - deterministic ordering
- Filtering:
  - workspace id
  - runtime state
  - execution location (`workspace` or `repo:<repo_key>`)
  - kind
  - explicit session (`--session`)

## Deferred to AGENT-100

- snapshot fields for attach/input ownership (`attached_clients`, `writer_owner`, `lease_expires_at`)
- full runtime lifecycle events (beyond terminal-sequence signal subset)
- lease/takeover event model
- launch mode metadata (`launch_mode`)
