---
title: "Agent Runtime Architecture"
status: implemented
---

# Agent Runtime Architecture

## Goal

Define a runtime architecture for `kra agent` that:

- keeps agent sessions alive independently of terminal tabs
- supports attach/reattach from another terminal
- keeps runtime files outside `KRA_ROOT` Git working tree

## Decision Snapshot (implemented)

- broker model: per-`KRA_ROOT` local broker over Unix socket
- launch model: detached by default
- connection model: multi-attach view is supported
- attach replay model: broker keeps per-session PTY output history in memory and replays it on attach before live relay
- attach scope: workspace/repo context only (root/outside is error)
- state model: snapshot JSON per session under `KRA_HOME`

## Beginner-Friendly Terms

- session: one running agent process instance
- attach: connect terminal I/O to existing session PTY
- detach: disconnect terminal while session keeps running
- broker: local manager process that owns PTYs and child processes
- PTY: pseudo terminal used to run CLI agents as interactive programs

## Component Topology

```mermaid
flowchart LR
  subgraph Client["Client terminals"]
    C1["Terminal A<br>kra agent run"]
    C2["Terminal B<br>kra agent attach --session <id>"]
    C3["Terminal C<br>kra agent list / board"]
  end

  subgraph Runtime["Per KRA_ROOT runtime"]
    B["Broker<br>~/.kra/run/agent/<root-hash>.sock"]
    P["PTY per session"]
    H["Output history buffer<br>per session (in-memory)"]
    A["Agent process<br>codex / claude / custom command"]
  end

  subgraph State["Persistent state"]
    S["~/.kra/state/agents/<root-hash>/<session-id>.json"]
  end

  C1 -->|start| B
  C2 -->|attach + replay + live stream| B
  C3 -->|read snapshots| S
  B --> P --> A
  B --> H
  B --> S
```

## Concept Map (ASCII)

```text
KRA_ROOT
└─ root-hash
   ├─ broker socket
   │  └─ ~/.kra/run/agent/<root-hash>.sock
   └─ runtime state
      └─ ~/.kra/state/agents/<root-hash>/
         └─ <session-id>.json

Broker (per root)
├─ session s-...-1234
│  ├─ PTY
│  ├─ output history buffer (in-memory byte stream)
│  ├─ child process (agent CLI)
│  └─ attached clients (0..N)
└─ session s-...-5678
```

## Directory and Socket Layout

- socket path:
  - `~/.kra/run/agent/<root-hash>.sock`
- snapshot path:
  - `~/.kra/state/agents/<root-hash>/<session-id>.json`

Notes:

- same `KRA_ROOT` always maps to same socket path
- different roots are isolated by different `root-hash`

## Broker Lifecycle

- one broker per `KRA_ROOT`
- `run/stop/attach` connect to socket
- when socket is missing/stale, `run` starts broker and reconnects
- broker auto-exits only when:
  - `session_count=0`
  - no broker requests for 60 seconds
- while sessions exist, broker stays alive

## Lifecycle: run (detached default)

```mermaid
sequenceDiagram
  participant U as User
  participant CLI as kra agent run
  participant B as Broker
  participant PTY as PTY
  participant AG as Agent
  participant ST as Snapshot

  U->>CLI: kra agent run ...
  CLI->>B: connect/start broker if needed
  CLI->>B: start session request
  B->>PTY: allocate PTY
  B->>AG: spawn process
  B->>ST: write runtime_state=running
  CLI-->>U: print session_id and return
```

## Lifecycle: attach / reattach

```mermaid
sequenceDiagram
  participant U as User
  participant CLI as kra agent attach
  participant B as Broker
  participant H as Session history buffer
  participant PTY as Session PTY

  U->>CLI: kra agent attach --session <id>
  CLI->>B: attach request
  B->>B: register attachment (paused)
  B-->>CLI: attach accepted + stream open
  B->>H: read buffered output from offset 0
  B-->>CLI: replay buffered output
  B->>H: drain catch-up tail while paused
  B->>B: unpause attachment
  CLI<->>B: stdin/stdout stream
  B<->>PTY: input/output relay
```

## Attach Replay Model (implemented baseline)

- broker stores PTY stdout as append-only in-memory bytes per session
- on `attach`, broker performs:
  - register target attachment in `paused` mode
  - replay full buffered output to rebuild terminal-visible state
  - drain catch-up bytes produced during replay
  - switch attachment to live relay mode
- this design avoids output gaps between replay and live stream for the attaching client

Notes:

- replay source is memory owned by broker process (not persisted to disk)
- when broker exits, replay history is lost; session is already ended at that point
- large/long sessions increase broker memory usage in this baseline

## Attach Scope Resolution

- inside `workspaces/<id>/repos/<repo-key>/...`:
  - candidates are same `workspace + repo`
- inside `workspaces/<id>/...`:
  - candidates are same workspace
- at `KRA_ROOT` root:
  - error (scope too broad)
- outside `KRA_ROOT`:
  - error

## Runtime State

Current process state axis:

- `running`
- `idle`
- `exited`
- `unknown`

Snapshot updates are atomic and increment session `seq`.

## Deferred (AGENT-100)

- writer lease / takeover protocol
- dangerous key confirmation
- append-only event log (`events/<session-id>.jsonl`)
- launch abstraction (`--launch default|resume|continue`)
- attach/input ownership fields in snapshot
