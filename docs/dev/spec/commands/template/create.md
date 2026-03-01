---
title: "`kra template create`"
status: implemented
---

# `kra template create`

## Purpose

Create a workspace template scaffold under the current root.

## Inputs

- `--name <template>` (optional)
- `--from <template>` (optional)
- `<template>` positional (optional)
- when both omitted, prompt for template name

## Behavior

- Resolve current root via existing root detection policy.
- Resolve template name in this order:
  1. `--name`
  2. positional `<template>`
  3. interactive prompt (`template name: `)
- Validate template name with workspace ID rules.
- Create scaffold at `<current-root>/templates/<name>/`:
  - `notes/`
  - `artifacts/`
  - `AGENTS.md` (default guidance content)
- when `--from <template>` is provided:
  - resolve source template from `<current-root>/templates/<template>/`
  - validate source template with shared template validation rules
  - copy source template contents as-is into `<current-root>/templates/<name>/`
- Fail when target template path already exists.
- Commit policy (inside current root git worktree):
  - stage only `templates/<name>/` (`git add -A -- templates/<name>`)
  - verify every staged path is under the allowlisted prefix
  - if any staged path is outside allowlist, abort without committing
  - commit message: `template-create: <name>`
- Print human result summary on success.

## Exit policy

- success: `exitOK`
- invalid argument: `exitUsage`
- runtime failure: `exitErr`

## Errors

- cannot resolve current root
- invalid template name
- target template already exists
- filesystem write failure
