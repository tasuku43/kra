---
title: "cmux integration guide"
status: implemented
---

# cmux integration guide

This guide explains how `kra` works with `cmux` in task-driven workflows.

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
                  ├─ notes/
                  └─ artifacts/

PROJ-1235  --->   workspaces/PROJ-1235/ --->  workspace: PROJ-1235
                  ├─ repos/
                  │  └─ backend/
                  ├─ notes/
                  └─ artifacts/

PROJ-1236  --->   workspaces/PROJ-1236/ --->  workspace: PROJ-1236
                  ├─ repos/
                  │  ├─ api/
                  │  ├─ web/
                  │  └─ infra/
                  ├─ notes/
                  └─ artifacts/
```

## Open behavior

`kra ws open --id <id>`:

- selects or creates the mapped `cmux` workspace when mapping is missing
- reuses existing mapping when the mapped `cmux` workspace is reachable

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
kra ws open --id TASK-1234
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
