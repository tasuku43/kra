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
                  ├─ .cmux/
                  │  └─ dock.json
                  ├─ tasks.md
                  ├─ notes/
                  └─ artifacts/

PROJ-1235  --->   workspaces/PROJ-1235/ --->  workspace: PROJ-1235
                  ├─ repos/
                  │  └─ backend/
                  ├─ .cmux/
                  │  └─ dock.json
                  ├─ tasks.md
                  ├─ notes/
                  └─ artifacts/

PROJ-1236  --->   workspaces/PROJ-1236/ --->  workspace: PROJ-1236
                  ├─ repos/
                  │  ├─ api/
                  │  ├─ web/
                  │  └─ infra/
                  ├─ .cmux/
                  │  └─ dock.json
                  ├─ tasks.md
                  ├─ notes/
                  └─ artifacts/
```

## Dock task view

KRA_ROOT can also have `.cmux/dock.json`. The root Dock is useful for seeing open tasks across active workspaces:

```json
{
  "controls": [
    {
      "id": "kra-tasks",
      "title": "Tasks",
      "command": "kra ws task view --all --todo-only --watch --refresh 2s",
      "cwd": ".",
      "height": 420
    }
  ]
}
```

New default workspaces include `.cmux/dock.json`. cmux reads this project Dock config from the workspace root and can show a right-sidebar `Tasks` control.

Default `.cmux/dock.json`:

```json
{
  "controls": [
    {
      "id": "kra-tasks",
      "title": "Tasks",
      "command": "kra ws task view --current --watch --refresh 2s",
      "cwd": ".",
      "height": 420
    }
  ]
}
```

The command runs:

```sh
kra ws task view --current --watch --refresh 2s
```

When `kra` is made available by shell startup files, generated Dock configs may prefix the command with the detected shell init file. For example, zsh users may see:

```sh
source ~/.zshrc; kra ws task view --current --watch --refresh 2s
```

This makes the Dock a constantly visible view of `<workspace>/tasks.md`. The task source of truth remains `<workspace>/tasks.md`; `kra ws task sync` is deprecated and no longer refreshes cmux task pills. The project Dock config may trigger a cmux trust prompt the first time the workspace is opened.

`kra init` does not overwrite an existing `templates/default/`, and template changes are not applied retroactively by init. For an existing root and active workspaces, run:

```sh
kra root migrate
kra root migrate --apply
```

The migration adds missing `.cmux/dock.json`, `templates/default/.cmux/dock.json`, `templates/default/tasks.md`, `workspaces/<id>/.cmux/dock.json`, and `workspaces/<id>/tasks.md` without overwriting custom files.

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
