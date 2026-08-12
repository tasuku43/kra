---
title: "cmux integration guide"
status: implemented
---

# cmux integration guide

This guide explains how `kra` works with `cmux` in task-driven workflows.

## Compatibility and socket access

The native Group and Todo paths are verified against cmux `v0.64.22` and are enabled by advertised/probed
methods rather than a version-string gate. cmux's default `cmuxOnly` socket mode accepts processes launched
under a cmux terminal, which is the normal KRA workflow. To invoke KRA from an external launcher, use cmux's
same-user `automation` mode or password authentication. Do not broaden access with `allowAll` just for KRA.

## Operating model

`kra` aligns three entities in one operating model:

- ticket id
- `kra` workspace (`workspaces/<id>/`)
- `cmux` workspace

This 1:1:1 mapping helps avoid context drift across tools.

### Conceptual mapping (ticket system -> filesystem -> cmux)

```text
Ticket System     Filesystem (KRA_ROOT)       cmux
---------------   --------------------------   -----------------------
PROJ-1234  --->   workspaces/PROJ-1234/ --->  workspace: PROJ-1234
                  ├─ repos/
                  │  (no repo attached)
                  ├─ workspace.md
                  ├─ notes/
                  └─ artifacts/

PROJ-1235  --->   workspaces/PROJ-1235/ --->  workspace: PROJ-1235
                  ├─ repos/
                  │  └─ backend/
                  ├─ workspace.md
                  ├─ notes/
                  └─ artifacts/

PROJ-1236  --->   workspaces/PROJ-1236/ --->  workspace: PROJ-1236
                  ├─ repos/
                  │  ├─ api/
                  │  ├─ web/
                  │  └─ infra/
                  ├─ workspace.md
                  ├─ notes/
                  └─ artifacts/
```

## Workspace Groups and native Todo

`kra root open` and `kra ws open` organize a KRA root and its managed workspaces as a cmux Workspace
Group when the installed cmux exposes the required group APIs. The root target is the managed group
anchor; workspace targets join the same group without overwriting user-customized group presentation.

Structured task state remains filesystem-authoritative. Run `kra ws task sync` explicitly to
reconcile KRA-managed tasks with the mapped cmux workspace's native Todo checklist. Foreign Todo
items are preserved. Use `--dry-run` to inspect the plan before applying it; `kra task sync` applies
the same contract to the root inbox on the managed group anchor. Opening a root or workspace never
mutates Todo implicitly.

kra no longer creates, installs, migrates, or removes cmux Dock controls. New roots, templates, and
workspaces do not receive `.cmux/dock.json`. Existing user-authored Dock files remain byte-for-byte
unchanged and outside kra's managed scope.

For an existing root and active workspaces, use:

```sh
kra root migrate
kra root migrate --apply
```

This migration only adds or converts workspace handoff files such as `workspace.md`; it does not
inspect or mutate Dock configuration.

## Open behavior

`kra ws open --id <id>`:

- creates the mapped `cmux` workspace when mapping is missing
- reuses existing mapping when the mapped `cmux` workspace is reachable
- sends a cmux notification from the target workspace
- does not automatically focus the target workspace; use the cmux notification action to jump to it
- accepts `--command "<cmd>"` to send a startup command to the target terminal
  - newly created cmux workspaces start in the workspace directory, then receive `<cmd>`
  - reused cmux workspaces receive `<cmd>` on their selected surface

In single-target open (`--id` or `--current`), if `cmux` capabilities are unavailable, `kra` falls back to directory-open behavior. With shell integration enabled, this can synchronize parent shell `cwd`.

## Close behavior

`kra ws close --id <id>`:

- archives workspace data as usual
- then closes mapped `cmux` workspace(s) on a best-effort basis
- does not roll back successful archive results even if `cmux` close fails

## Minimal recipe

```sh
kra ws create TASK-1234
kra repo add git@github.com:org/backend.git
kra ws add-repo --id TASK-1234
kra ws open --id TASK-1234 --command "claude"
# click/follow the cmux notification to jump to the workspace
# run your agent in this task workspace
kra ws close --id TASK-1234
```

## Related docs

- Command overview: `docs/user/guides/COMMANDS.md`
- Workspace task guide: `docs/user/guides/TASKS.md`
- Workspace docs viewer guide: `docs/user/guides/WORKSPACE_DOCS.md`
- Shell integration guide: `docs/user/guides/SHELL_INTEGRATION.md`
- Mapping concept: `docs/dev/spec/concepts/cmux-mapping.md`
- Open command contract: `docs/dev/spec/commands/ws/open.md`
