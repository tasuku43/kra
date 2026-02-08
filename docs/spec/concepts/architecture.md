---
title: "Architecture"
status: planned
---

# Architecture (target)

## Goal

Move from CLI-centric implementation to layered architecture so launcher flows and direct command flows share
the same application use cases.

## Layers

- `internal/cli`
  - argument parsing
  - command routing
  - exit code mapping
  - minimal human-readable output orchestration
- `internal/app`
  - command-level use cases (`init`, `context`, `repo/*`, `ws/*`)
  - orchestration only; no direct terminal dependencies
- `internal/domain`
  - pure business rules (workspace selection policy, lifecycle transition rules, validation policy)
  - no IO calls
- `internal/infra`
  - adapters for git/statestore/paths/registry/filesystem/time/process
  - explicit interfaces consumed by `app`
- `internal/ui`
  - selector/prompt/rendering abstractions used by human-interactive use cases
  - no command routing

## Structural rules

- `cli` must not import `statestore`, `paths`, or `gitutil` directly after migration.
- `cli` calls `app` use cases through explicit request/response structs.
- `app` depends on interfaces only; concrete adapters live in `infra` and `ui`.
- Launcher (`gionx ws`) and direct operations (`ws go`, `ws close`, `ws add-repo`, ...) must execute through
  the same `app` use case path to avoid behavior drift.

## Migration strategy

1. Add package skeleton (`app`, `domain`, `infra`, `ui`) and dependency direction tests.
2. Introduce shared workspace operation use case APIs in `internal/app/ws`.
3. Migrate commands in slices while preserving behavior:
  - `ws` family
  - `init/context`
  - `repo` family
  - `shell`
4. Remove direct infra calls from `cli`.
5. Mark this spec `implemented` only when all command groups run through `app`.

## Definition of done (architecture migration)

- Command handlers in `internal/cli` are thin wrappers over `app` use cases.
- Shared behavior parity is verified by integration tests for:
  - launcher vs direct command execution
  - `--select` vs direct id
- No layering violations according to architecture guard tests.

