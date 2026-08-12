---
title: "cmux Workspace Group integration"
status: implemented
---

# cmux Workspace Group integration

## Purpose

Use cmux's native sidebar hierarchy as the runtime UI for a KRA root without making cmux the source of truth
for KRA workspace lifecycle.

## Runtime model

- One KRA root maps to one cmux Workspace Group in one cmux window.
- The managed `KRA_ROOT` workspace is the group anchor and starts at `KRA_ROOT`.
- Each managed active KRA workspace runtime is a child of that group.
- The group name is the root directory basename; the group title may be reconciled, but KRA does not change
  group color, icon, pin, or collapse preferences.
- Workspace Groups are window-scoped. KRA persists the group, window, and anchor UUIDs separately from the
  per-workspace mapping entries.
- A fresh managed group is created only after the group-list response supplies a stable window UUID; an ephemeral
  window ref is never persisted or used as its identity.

## Capability and fallback

- Group support requires `workspace.group.list`, `workspace.group.create`, `workspace.group.add`,
  `workspace.group.remove`, `workspace.group.set_anchor`, `workspace.group.ungroup`, and
  `workspace.group.focus`.
- Capability discovery is method-based, not version-string-based.
- When these methods are absent, `root open` and `ws open` retain the ungrouped workspace behavior and report
  that group integration was skipped. A missing Todo capability is independent from group support.

## Reconciliation

- `root open` creates or reuses the `KRA_ROOT` workspace and ensures that it is the managed group anchor.
- `ws open` first ensures the root group, then creates a new workspace atomically in the group when possible or
  adds the reused/relinked workspace to it.
- A live mapped group is authoritative for user-controlled presentation fields and foreign members.
- If a mapped KRA workspace is ungrouped, KRA re-adds it.
- If a mapped KRA workspace belongs to another group, KRA reports `cmux_group_conflict` and does not move it.
- Foreign group members are preserved.
- A stale group mapping is replaced only after enumerating every cmux window by stable UUID and proving the old
  group UUID is absent from all of them. Window enumeration or any per-window list failure is fail-closed. Name
  matching is never used to take ownership of a foreign group.
- If the saved window is stale but the same group UUID is visible in another window, KRA updates the window UUID
  and reuses the group instead of creating a replacement.
- Legacy `KRA_ROOT` adoption resolves the existing root runtime's caller window and creates the group in that
  window; KRA does not attempt to adopt a workspace into an unrelated ambient window.

## Failure safety

- Group and workspace mapping updates share the root cmux mapping transaction lock.
- A newly-created group/anchor is provisional until mapping persistence succeeds.
- A newly-created child workspace is provisional until group membership, mandatory workspace setup, and mapping
  persistence succeed.
- Typed workspace creation supplies the saved window UUID and a UUID `operation_id`. If the first response is
  lost while the caller is still active, KRA retries once with the same ID; cmux's durable at-most-once contract
  returns the existing workspace or refuses a duplicate. If both responses fail, the error carries the operation,
  window, group, cwd, and title for manual inspection.
- Workspace Group creation has no ownership token. An error response is therefore causally ambiguous: KRA reports
  the requested window/name and never guesses ownership from a post-error list or cleans up by name.
- Compensation closes only KRA-created provisional runtimes and never deletes a reused group or a foreign/user
  workspace. KRA never invokes destructive group deletion with `close_workspaces`.
- cmux group removal is workspace-scoped rather than compare-and-remove-by-group. KRA therefore does not remove
  an adopted reused workspace as rollback: an existing managed group keeps the convergent membership, while a
  newly-created provisional group is dissolved non-destructively so every reused member is preserved.
- When cleanup cannot complete, the error includes every provisional runtime/group identity that may remain.

## Mapping schema

Schema version 2 adds an optional root group record:

```json
{
  "version": 2,
  "root_group": {
    "cmux_window_id": "UUID",
    "cmux_group_id": "UUID",
    "anchor_workspace_id": "UUID",
    "name_snapshot": "kra",
    "created_at": "2026-08-13T00:00:00Z",
    "last_used_at": "2026-08-13T00:00:00Z"
  },
  "workspaces": {}
}
```

Version 1 is migrated in memory to version 2 with an empty group record and saved on the next mutation.
