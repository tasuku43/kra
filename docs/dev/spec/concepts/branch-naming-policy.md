---
title: "Branch Naming Policy"
status: implemented
---

# Branch Naming Policy

## Purpose

Standardize workspace branch names for `ws add-repo` while keeping per-repo override flexibility.

## Config model

Add config key:

```yaml
workspace:
  branch:
    template: "feature/{{workspace_id}}"
```

Supported placeholders (phase 1):

- `{{workspace_id}}`
- `{{repo_key}}` (for example `owner/repo`)
- `{{repo_name}}` (for example `repo`)

## Precedence

1. CLI `--branch`
2. workspace branch template (`workspace.branch.template`)
3. fallback current behavior (`<workspace-id>`)

## Validation

- rendered branch name must pass `git check-ref-format`
- unsafe or empty render result is rejected with `invalid_argument`
- `/` normalization policy follows existing add-repo rules

## Behavior in `ws add-repo`

- interactive input default value uses rendered template result
- JSON mode default `branch` uses rendered template when `--branch` omitted
- policy is default-generation only in phase 1 (no hard enforcement on user-provided branch value)
- template render context is deterministic and does not use shell/env interpolation

## Non-goals (phase 1)

- conditional templates (`if/else`)
- per-repo persistent branch rule files
- backfilling existing `repos_restore.branch`
