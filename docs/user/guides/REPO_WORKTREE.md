---
title: "Repo pool and worktree guide"
status: implemented
---
# Repo pool and worktree guide

This guide explains repository registration and per-task attachment.

## Two-step model

`kra` separates repository registration from workspace attachment:

1. `kra repo add ...` registers repositories into the current root's repo pool.
2. `kra ws add-repo --id <id>` attaches selected repositories as worktrees in that workspace.

This keeps active workspace context minimal and task-scoped.

```text
Repo Pool (KRA_ROOT/.kra/repo-pool)    Picked Workspace (KRA_ROOT)
---------------------------------      -----------------------------
backend.git   (bare)   ---\            workspaces/PROJ-1236/
frontend.git  (bare)   ----+--------->   └─ repos/
infra.git     (bare)   ---/                 ├─ backend/   (worktree)
                                            ├─ frontend/  (worktree)
                                            └─ infra/     (worktree)
```

## Where data is stored

- root-local pool: `<KRA_ROOT>/.kra/repo-pool/`
- workspace attachments: `workspaces/<id>/repos/<alias>/`

Each KRA root has an independent physical pool. This keeps the pool available when an agent
isolation tool mounts only `KRA_ROOT`. Run commands from the mounted root (or a descendant) so
root discovery works even when the host's `$KRA_HOME` is not mounted. Existing users can run
`kra root migrate --apply` once to copy or merge the legacy `$KRA_HOME/repo-pool/` into the current
root; the legacy pool is not deleted. Migration refuses to copy while the legacy pool has linked
worktrees; close those workspaces first. New worktrees use relative Git links, and migration
normalizes existing root-local links for the same host/isolation portability.

All pool and worktree mutations use the shared lock at
`<KRA_ROOT>/.kra/state/operations/repo-pool.lock`, so host and isolated processes sharing the
mounted root are serialized safely.

## Typical multi-repo flow

```sh
kra repo add git@github.com:org/backend.git git@github.com:org/frontend.git
kra ws add-repo --id TASK-1234
kra ws remove-repo --id TASK-1234
```

`ws add-repo` lets you choose per-repo branch inputs (`base_ref`, `branch`) so branch context remains explicit.

## Alias and branch notes

- alias must be unique within the workspace
- `base_ref` is used as the branch starting point
- `branch` defines the task-scoped working branch

## Why this model helps

- add repositories only when needed
- avoid mixing temporary task outputs into long-lived repo roots
- make task resume easier because repo set is explicit per workspace

## Related docs

- Command overview: `docs/user/guides/COMMANDS.md`
- `repo add` contract: `docs/dev/spec/commands/repo/add.md`
- `ws add-repo` contract: `docs/dev/spec/commands/ws/add-repo.md`
- `ws remove-repo` contract: `docs/dev/spec/commands/ws/remove-repo.md`
