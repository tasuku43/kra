---
title: "User Config"
status: implemented
---

# User Config

## Purpose

Provide a small YAML-based configuration surface to avoid repeating
frequently used command options.

## Config files

- Global config (optional):
  - `~/.kra/config.yaml`
  - if missing, treat as empty config
  - bootstrap policy:
    - first run of a state-changing command creates a commented scaffold file
      (read-only commands do not create it)
- Root-local config:
  - `<KRA_ROOT>/.kra/config.yaml`
  - lifecycle/bootstrap is defined by `docs/dev/spec/commands/init.md`

Global path may be overridden with `$KRA_HOME`:
- `<KRA_HOME>/config.yaml`
- `<KRA_HOME>/state/current-context`
- `<KRA_HOME>/state/root-registry.json`
- `<KRA_HOME>/repo-pool/`

## Merge precedence

Resolve values in this order:

1. CLI flag/input
2. root config (`<root>/.kra/config.yaml`)
3. global config (`~/.kra/config.yaml`)
4. command default

For string values, empty/whitespace-only values are treated as "unset".

## Schema (MVP)

```yaml
workspace:
  defaults:
    template: default
  close:
    empty_record_policy: require-confirmation # warn | require-confirmation
  branch:
    template: "feature/{{workspace_id}}"
  repo_presets:
    backend:
      repos:
        - org/api
        - org/web

integration:
  jira:
    defaults:
      space: DEMO
      type: sprint # sprint | jql
      sprint_selection: prompt # prompt | current
  import:
    defaults:
      target: both # jira | github-review | both
```

Notes:
- `integration.jira.defaults.space` and `integration.jira.defaults.project` are aliases for the same scope concept.
- Only one of them may be active at a time.
- `integration.jira.defaults.sprint_selection` is used when `--sprint` is selected but no sprint value is given.
- `integration.import.defaults.target` controls the default source set for `kra ws import all`.
- `workspace.close.empty_record_policy` controls how `ws close` reacts when a workspace has no substantive
  `notes/` or `artifacts/` record.
- `workspace.repo_presets` is a map keyed by preset name.
- `workspace.repo_presets.<name>.repos[]` stores `repo_key` values in user-selected order.
- Repo preset persistence is root-local by design (`<KRA_ROOT>/.kra/config.yaml`) to keep team/project intent explicit.

## Validation rules

- `integration.jira.defaults.type` must be one of:
  - `sprint`
  - `jql`
- `integration.jira.defaults.sprint_selection` must be one of:
  - `prompt`
  - `current`
- `integration.jira.defaults.space` and `integration.jira.defaults.project` must not be combined.
- `integration.import.defaults.target` must be one of:
  - `jira`
  - `github-review`
  - `both`
- `workspace.close.empty_record_policy` must be one of:
  - `warn`
  - `require-confirmation`
- `workspace.repo_presets.<name>.repos[]` must not be empty.
- Invalid config must fail command execution with a clear path + reason.

## Error handling

- Parse errors should include the config file path.
- Validation errors should include concrete key names (for quick fix).

## Scaffold comments

Generated root/global config files should include a short header comment that explains:

- precedence order (`CLI > root > global > default`)
- empty-string handling ("unset")
