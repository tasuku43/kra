---
title: "`kra state` registry foundation"
status: implemented
---

# `kra state` registry foundation

## Purpose

Provide a stable foundation for root discovery and cross-root metadata scan.

## Background

- `kra` keeps canonical workspace metadata under each root filesystem.
- Cross-root operations (e.g. `repo gc`) still need a compact list of known roots.
- We need a canonical registry to make future `state`/`context` workflows reliable.

## Scope (foundation only)

- Define a registry file managed by `kra`:
  - location: `~/.kra/state/root-registry.json`
  - format: JSON
- Each entry tracks one known root:
  - `root_path` (absolute, canonical path)
  - `first_seen_at` (unix epoch)
  - `last_used_at` (unix epoch)
- Registry updates happen on root-touch paths used by current commands (`init`, `ws create`, `ws list`, `repo *`).

Implemented command integration:
- `kra init`
- `kra ws create`
- `kra ws list`

## Invariants

- `root_path` is unique in the registry.
- `last_used_at` is monotonic non-decreasing per entry.
- Missing registry file is treated as empty and lazily created.

## Out of scope (follow-up specs)

- User-facing `kra state list`
- User-facing `kra state gc`
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
