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
  - `~/.gionx/config.yaml`
  - if missing, treat as empty config
  - bootstrap policy:
    - first run of a state-changing command creates a commented scaffold file
      (read-only commands do not create it)
- Root-local config:
  - `<GIONX_ROOT>/.gionx/config.yaml`
  - lifecycle/bootstrap is defined by `docs/spec/commands/init.md`

Global path may be overridden with `$GIONX_HOME`:
- `<GIONX_HOME>/config.yaml`
- `<GIONX_HOME>/state/current-context`
- `<GIONX_HOME>/state/root-registry.json`
- `<GIONX_HOME>/repo-pool/`

## Merge precedence

Resolve values in this order:

1. CLI flag/input
2. root config (`<root>/.gionx/config.yaml`)
3. global config (`~/.gionx/config.yaml`)
4. command default

For string values, empty/whitespace-only values are treated as "unset".

## Schema (MVP)

```yaml
workspace:
  defaults:
    template: default

integration:
  jira:
    defaults:
      space: SRE
      project: APP
      type: sprint # sprint | jql
```

Notes:
- `integration.jira.defaults.space` and `integration.jira.defaults.project` are aliases for the same scope concept.
- Only one of them may be active at a time.

## Validation rules

- `integration.jira.defaults.type` must be one of:
  - `sprint`
  - `jql`
- `integration.jira.defaults.space` and `integration.jira.defaults.project` must not be combined.
- Invalid config must fail command execution with a clear path + reason.

## Error handling

- Parse errors should include the config file path.
- Validation errors should include concrete key names (for quick fix).

## Scaffold comments

Generated root/global config files should include a short header comment that explains:

- precedence order (`CLI > root > global > default`)
- empty-string handling ("unset")
