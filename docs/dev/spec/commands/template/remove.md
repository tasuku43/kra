---
title: "`kra template remove`"
status: implemented
---

# `kra template remove`

## Purpose

Remove one workspace template under the current root.

## Inputs

- command aliases:
  - `kra template remove`
  - `kra template rm`
- `--name <template>` (optional)
- `<template>` positional (optional)
- when both omitted, prompt for template name

## Behavior

- Resolve current root via existing root detection policy.
- Resolve template name in this order:
  1. `--name`
  2. positional `<template>`
  3. interactive prompt (`Inputs:` -> `name: `)
- Validate template name with workspace ID rules.
- Resolve target path as `<current-root>/templates/<name>/`.
- Fail when target template does not exist.
- Fail when target path is not a directory.
- Remove target template directory recursively.
- Commit policy (inside current root git worktree):
  - stage only `templates/<name>/` (`git add -A -- templates/<name>`)
  - verify every staged path is under the allowlisted prefix
  - if any staged path is outside allowlist, abort without committing
  - commit message: `template-remove: <name>`
- Print human result summary on success.

## Exit policy

- success: `exitOK`
- invalid argument: `exitUsage`
- runtime failure: `exitErr`

## Errors

- cannot resolve current root
- invalid template name
- template not found
- path is not a directory
- filesystem remove failure
