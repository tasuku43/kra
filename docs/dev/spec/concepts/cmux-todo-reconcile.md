---
title: "cmux native Todo reconciliation"
status: implemented
---

# cmux native Todo reconciliation

## Purpose

Expose KRA tasks in each mapped cmux workspace's native Todo checklist while preserving both KRA's structured
task model and Todo items owned directly by the user.

## Sources and ownership

- KRA task source order is `workspace.yaml`, then legacy `workspace.md`; root inbox uses root `workspace.md`.
- KRA remains authoritative for task ID, title, description, dependencies, declaration order, and persistence.
- cmux native Todo is an interaction projection. Only checklist state is eligible for pull-back.
- KRA never pins `workspace.status`; cmux keeps its automatic agent/PR/git inference.
- Checklist items not recorded in the KRA sync base are foreign and are always preserved.

## Projection

- Managed item text is `<task-id> <title>`; `blocked` additionally uses the visible prefix `⛔ `.
- Status mapping:
  - `todo` -> `pending`
  - `doing` -> `in-progress`
  - `blocked` -> `pending` plus the `⛔ ` prefix
  - `done` -> `completed`
- Description and `depends_on` are not projected because cmux Todo has no equivalent fields.
- Text is capped by the cmux 500-character contract; truncation preserves the task ID and is reported.
- cmux permits 50 items. Foreign items are retained first; managed candidates are ordered `doing`, `blocked`,
  `todo`, `done`, preserving file order within each state. If every KRA task cannot fit, reconciliation fails
  without changing either side or the base.

## Three-way base

Ignored runtime state stores one base per KRA workspace/task and cmux item UUID:

```json
{
  "kra_task_id": "TASK-001",
  "cmux_item_id": "UUID",
  "kra_status": "todo",
  "cmux_state": "pending",
  "text": "TASK-001 build parser"
}
```

- Local-only state change: push to the existing cmux UUID.
- cmux-only state change: pull the mapped state into the KRA document.
- Same effective change on both sides: accept and advance the base.
- Different changes on both sides, managed remote deletion, or managed text identity drift: return a conflict and
  leave local document, live Todo, and base unchanged.
- `--prefer kra` resolves conflicts by pushing; `--prefer cmux` resolves state conflicts by pulling. Remote
  deletion never deletes a KRA task; `--prefer kra` recreates it and `--prefer cmux` is invalid for deletion.
- Deleting a managed KRA task while its cmux item still exists is a conflict: KRA cannot recreate the same cmux
  UUID if base persistence later fails, so neither side nor the base is changed. When both sides deleted the
  item, its base entry is dropped.
- Newly added local tasks create managed Todo items with `origin=agent`; newly observed foreign Todo items are not
  adopted implicitly.

## Command boundary

- `kra ws task sync` performs reconciliation for one or all mapped active workspaces.
- `kra task sync` performs reconciliation for the root inbox against the group anchor.
- Todo mutation is explicit: task `add` and `status` do not silently mutate cmux.
- `root open` and `ws open` never mutate native Todo. Initial projection also requires explicit `task sync` or
  `ws task sync`.
- Invalid local task documents fail closed before reading or mutating live Todo.
- Capability discovery is method-based. Because cmux v0.64.22 under-advertises Todo methods, an absent
  `workspace.todo.list` capability is followed by a read-only list probe; failure stops before mutation.

## Consistency and recovery

- Reconciliation uses the root mapping lock and re-loads both mapping/base and the local document after locking.
- Live Todo is re-read immediately before each mutation. Because cmux v0.64.22 has no revision/CAS token, KRA
  uses granular item mutations and verifies the full checklist after mutation.
- Every Todo list response must identify the requested workspace. Missing or different workspace IDs fail closed
  before the next mutation.
- If a mutation RPC fails, only an unchanged before image proves that no mutation occurred. Any changed image,
  including the exact intended result, is causally ambiguous; KRA does not undo it because the change may belong
  to a user or another agent, and reports manual repair instead.
- A mutation RPC that returns success with a payload inconsistent with the requested item is also a recovery
  failure. The live effect is not inferred from the response and no remote undo is attempted.
- KRA does not call the full-list `workspace.todo.set` replacement API; doing so could erase foreign items and
  cannot be safely compensated with their original UUIDs.
- Local pull uses the task document CAS path and is rolled back with the resulting revision when later persistence
  fails. cmux has no conditional Todo mutation/CAS, so remote recovery is read-only: KRA audits whether each field
  is already back at its before image but never issues a second remote mutation to restore it. Any remaining remote
  change is preserved and reported with its item UUID for manual inspection.
- cmux does not expose Todo attachments in list/mutation payloads. A successfully added item is therefore never
  auto-removed during compensation: even unchanged visible fields cannot prove that no user attached content.
  KRA preserves the item and reports its UUID for manual inspection if later persistence fails. A successful add
  response with an empty or already-live UUID is also recovery failure: KRA cannot establish ownership and never
  guesses an item by matching visible text.
- Base is advanced only after local and remote verification succeeds.
- After `ws close` has closed (or confirmed the absence of) the mapped runtime, KRA removes that workspace's
  Todo base together with its workspace mapping. A close failure retains both for a later best-effort retry.
