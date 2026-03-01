---
title: "`kra template validate`"
status: implemented
---

# `kra template validate`

## Purpose

Validate workspace templates under the current root before `ws create`.

## Inputs

- `--name <template>` (optional)
  - when omitted, validate all entries under `<current-root>/templates/`
  - when provided, validate only the specified template

## Behavior

- Resolve current root via existing root detection policy.
- Reuse the same template validation rules as `ws create`.
- Validation rules:
  - template name must follow workspace ID rules
  - reserved top-level paths are forbidden (`repos/`, `.git/`, `.kra.meta.json`)
  - symlinks are forbidden
  - unsupported special file types are forbidden
- Collect and print all violations (not fail-fast on first violation).

## Exit policy

- zero violations: `exitOK`
- one or more violations: `exitErr`

## Errors

- templates directory missing
- no templates found (when validating all)
- specified template missing
  - show available template names when possible

