---
title: "`gionx state` registry foundation"
status: implemented
---

# `gionx state` registry foundation

## Purpose

Provide a stable foundation for state-store hygiene when `GIONX_ROOT`-scoped DB files increase over time.

## Background

- `gionx` now uses one SQLite file per `GIONX_ROOT`.
- As roots are created and abandoned, orphaned DB files can remain under XDG data paths.
- We need a canonical registry to make future `state` subcommands (`list`, `gc`) reliable.

## Scope (foundation only)

- Define a registry file managed by `gionx`:
  - location: `XDG_DATA_HOME/gionx/registry.json`
  - format: JSON
- Each entry tracks one root-scoped DB:
  - `root_path` (absolute, canonical path)
  - `state_db_path`
  - `first_seen_at` (unix epoch)
  - `last_used_at` (unix epoch)
- Registry updates happen on state store open paths used by current commands (`init`, `ws create`, `ws list`).

Implemented command integration:
- `gionx init`
- `gionx ws create`
- `gionx ws list`

## Invariants

- `root_path` is unique in the registry.
- `state_db_path` must match path resolution rules for that `root_path`.
- `last_used_at` is monotonic non-decreasing per entry.
- Missing registry file is treated as empty and lazily created.

## Out of scope (follow-up specs)

- User-facing `gionx state list`
- User-facing `gionx state gc`
- Automatic deletion policy
- Registry lock strategy beyond MVP local atomic write

## Error handling (MVP)

- If registry read fails due to malformed JSON, command fails with a clear recovery hint.
- Registry writes must be atomic (write temp + rename) to avoid partial files.
- Permission errors should include the concrete path in the message.

## Testing expectations

- Create-on-first-use behavior.
- Update `last_used_at` on repeated command runs.
- Corrupt registry handling path.
- Concurrent-ish overwrite safety via atomic replace semantics.
