---
title: "kra CLI specs"
status: implemented
---

# kra specs

This directory contains incremental specifications for `kra`.
Implementation should reference these specs. When behavior changes, update the spec first.

## Metadata rules (mirrors gion)

- Required: `title`, `status`.
- Optional: `pending` (YAML array of short tokens/ids for unimplemented pieces). If `pending` is non-empty,
  treat the spec as needing work even when `status: implemented`.
- Additional optional fields (e.g., `title`, `since`) are allowed.
- Use YAML frontmatter at the top of each spec.

### `status` values

- `planned`: spec-first discussion; not implemented yet.
- `implemented`: implemented and considered current.

## Index

- Core
  - `../backlog/README.md`: Implementation backlog index (dependencies, parallelism, spec mapping)
  - `core/DATA_MODEL.md`: State store data model (tables, keys, constraints)
  - `core/AGENTS.md`: AGENTS.md generation and conventions
- Concepts
  - `concepts/layout.md`: KRA_ROOT layout and Git tracking policy
  - `concepts/state-store.md`: Optional/rebuildable root index and registry
  - `concepts/config.md`: Global/root user config model and precedence
  - `concepts/branch-naming-policy.md`: Branch naming template policy for workspace repo operations
  - `concepts/fs-source-of-truth.md`: FS=SoT and index-store downgrade policy (planned)
  - `concepts/workspace-meta-json.md`: `.kra.meta.json` schema and atomic update rules (planned)
  - `concepts/workspace-template.md`: root-local workspace template model and validation
  - `concepts/ui-color.md`: CLI/TUI semantic color token policy
  - `concepts/architecture.md`: Layered architecture (`cli/app/domain/infra/ui`) and migration rules
  - `concepts/agent-skillpack.md`: project-local non-intrusive skillpack model
  - `concepts/workspace-lifecycle.md`: Workspace lifecycle state machine and transition policy
  - `concepts/output-contract.md`: Shared machine-readable output envelope and error code policy
  - `concepts/worklog-insight.md`: workspace-local approved insight capture model
  - `concepts/agent-runtime.md`: PTY-based agent runtime architecture and session state model (v3 draft)
- Commands
  - `commands/bootstrap/agent-skills.md`: `kra bootstrap agent-skills` + `init --bootstrap` integration
  - `commands/agent/activity.md`: `kra agent` runtime activity tracking (v3 draft)
  - `commands/agent/logs.md`: `kra agent logs` retirement (v3 draft)
  - `commands/agent/run.md`: `kra agent run` (PTY + interactive target selection)
  - `commands/agent/attach.md`: internal attach stream primitive (manager/foreground reuse)
  - `commands/agent/stop.md`: `kra agent stop` (session-oriented)
  - `commands/doctor-fix.md`: `kra doctor --fix --plan|--apply` staged remediation
  - `commands/doctor.md`: `kra doctor`
  - `commands/context.md`: `kra context`
  - `commands/init.md`: `kra init`
  - `commands/repo/add.md`: `kra repo add`
  - `commands/repo/discover.md`: `kra repo discover`
  - `commands/repo/remove.md`: `kra repo remove`
  - `commands/repo/gc.md`: `kra repo gc`
  - `commands/template/validate.md`: `kra template validate`
  - `commands/shell/init.md`: `kra shell init`
  - `commands/state/registry.md`: `kra state` foundation (registry)
  - `commands/ws/selector.md`: Shared inline selector UI for workspace actions
  - `commands/ws/select.md`: Unified human launcher (`ws select` / context-aware `ws`)
  - `commands/ws/create.md`: `kra ws create`
  - `commands/ws/select-multi.md`: `kra ws select --multi`
  - `commands/ws/import/jira.md`: `kra ws import jira`
  - `commands/ws/dashboard.md`: `kra ws dashboard`
  - `commands/ws/add-repo.md`: `kra ws --act add-repo`
  - `commands/ws/dry-run.md`: `kra ws --act <close|reopen|purge> --dry-run`
  - `commands/ws/lock.md`: `kra ws lock` / `kra ws unlock`
  - `commands/ws/insight.md`: `kra ws insight add`
  - `commands/ws/remove-repo.md`: `kra ws --act remove-repo`
  - `commands/ws/list.md`: `kra ws list`
  - `commands/ws/go.md`: `kra ws --act go`
  - `commands/ws/close.md`: `kra ws --act close`
  - `commands/ws/reopen.md`: `kra ws --act reopen`
  - `commands/ws/purge.md`: `kra ws --act purge`

- Development
  - `../dev/TESTING.md`: Testing principles (developer guidance)
  - `testing/ui-regression.md`: CLI human UI regression testing policy (E2E + component golden)
