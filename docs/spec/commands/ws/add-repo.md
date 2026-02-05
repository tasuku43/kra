# `gionx ws add-repo <workspace-id> <repo>`

## Purpose

Add a repository to a workspace as a Git worktree.

## Inputs

- `workspace-id`: an existing workspace ID
- `repo`: `gion`-style repo spec (`git@...` / `https://...`)

## Behavior (MVP)

- Normalize `repo` into `repoKey` using `gion-core/repospec`
- Determine `alias` from the repo URL tail:
  - `git@github.com:tasuku43/sample-frontend.git` -> `sample-frontend`
  - `git@github.com:tasuku43/sugoroku.git` -> `sugoroku`
  - alias overrides are not supported in the MVP
  - if `alias` conflicts within the same workspace, return an error
- Ensure a bare repo exists in the repo pool for `repoKey`
  - always `fetch` (prefetch should start as soon as `repo` is known to overlap user input time)
- Prompt for the branch name
  - prefill the input with `<workspace-id>/` but allow deleting it
  - accept any branch name that passes `git check-ref-format` (no extra rules)
  - if the remote branch exists, check it out (track it)
  - otherwise, create a new branch from the default branch
  - if the same branch is already checked out by another worktree, error (Git worktree constraint)
- Create the worktree at `GIONX_ROOT/workspaces/<id>/repos/<alias>`
- Record the binding in the global state store

