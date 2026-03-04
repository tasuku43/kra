---
title: "Config (`config.yaml`) guide"
status: implemented
---

# Config (`config.yaml`) guide

This guide explains where `kra` reads config from, how precedence works, and which keys are currently supported.

## Config files and precedence

`kra` reads two config files:

- root-local: `<KRA_ROOT>/.kra/config.yaml`
- global: `$KRA_HOME/config.yaml` (default path: `~/.kra/config.yaml`)

Effective precedence (high -> low):

1. CLI flags
2. root-local config (`<KRA_ROOT>/.kra/config.yaml`)
3. global config (`$KRA_HOME/config.yaml`)
4. built-in defaults

Notes:

- Empty string values are treated as unset.
- Root-local config is the right place for team/project defaults.
- Global config is the right place for personal defaults shared across roots.

## Supported keys

```yaml
workspace:
  defaults:
    template: default
  branch:
    template: "feature/{{workspace_id}}/{{repo_name}}"
  repo_presets:
    backend:
      repos:
        - org/api
        - org/web

integration:
  jira:
    base_url: "https://jira.example.com"
    defaults:
      type: sprint # sprint | jql
      # choose one:
      space: APP
      # project: APP
```

Key behavior:

- `workspace.defaults.template`
  - Default template for `kra ws create` when `--template` is omitted.
- `workspace.branch.template`
  - Default branch name template for `kra ws add-repo`.
  - Placeholders: `{{workspace_id}}`, `{{repo_key}}`, `{{repo_name}}`.
- `workspace.repo_presets.<name>.repos[]`
  - Root-local reusable repository set for `kra ws add-repo --preset <name>`.
  - Values are `repo_key` strings in stored order.
- `integration.jira.base_url`
  - Jira base URL used by `kra ws create --jira` and `kra ws import jira`.
  - Must be an absolute URL.
- `integration.jira.defaults.type`
  - Default import mode for `kra ws import jira` when mode flags are omitted.
  - Allowed values: `sprint`, `jql`.
- `integration.jira.defaults.space` / `integration.jira.defaults.project`
  - Default Jira project scope for sprint mode.
  - Do not set both at once (it is treated as invalid).

## Environment variables (Jira)

Jira credentials are env-only:

- `KRA_JIRA_EMAIL`
- `KRA_JIRA_API_TOKEN`

Base URL can also come from env and has highest priority:

- `KRA_JIRA_BASE_URL`

## Example split (global + root)

Global (`~/.kra/config.yaml`):

```yaml
workspace:
  defaults:
    template: default

integration:
  jira:
    base_url: "https://jira.example.com"
    defaults:
      type: sprint
```

Root-local (`<KRA_ROOT>/.kra/config.yaml`):

```yaml
workspace:
  defaults:
    template: backend
  branch:
    template: "feat/{{workspace_id}}/{{repo_name}}"

integration:
  jira:
    defaults:
      space: TEST
```

With this setup:

- `kra ws create TASK-1234` uses template `backend` in this root.
- `kra ws add-repo --id TASK-1234` proposes branch names from `feat/{{workspace_id}}/{{repo_name}}`.
- `kra ws import jira` defaults to sprint mode scoped to `TEST` unless CLI/env overrides it.
- This means `kra ws import jira` can run with no mode/scope flags in the common case.
