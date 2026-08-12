---
title: "workspace.yaml task schema"
status: implemented
---

# workspace.yaml task schema

## Purpose

Define a strict, structured YAML-based task contract that serves as the preferred source of truth for
workspace task state. `workspace.md` remains supported as a legacy fallback.

## Scope

- Applies to active and archived workspace folders.
- Preferred task document path: `workspaces/<id>/workspace.yaml` or `archive/<id>/workspace.yaml`.
- Fallback path (legacy): `workspaces/<id>/workspace.md` or `archive/<id>/workspace.md`.
- The file is optional; absence means zero structured tasks.

## Loading precedence

1. If `<workspace>/workspace.yaml` exists, read tasks from it.
2. Otherwise, if `<workspace>/workspace.md` exists, fall back to legacy Markdown parsing.
3. If neither exists, return an empty task state with no errors.

## Schema versioning

- `schema_version` is required and must currently be `1`.
- Future versions must be additive; commands must not break on newer minor/patch schemas.
- Unsupported `schema_version` values are fatal validation errors.

## File shape

```yaml
schema_version: 1
tasks:
  - id: TASK-001
    title: Inspect current task implementation
    status: done
    description: |
      Read the existing workspace task parser, task commands, and native Todo projection.
    depends_on: []
```

### Required fields

| Field         | Type   | Required | Constraints                                       |
|---------------|--------|----------|---------------------------------------------------|
| `schema_version` | int  | Yes      | Must be `1` in this iteration                     |
| `tasks`        | list   | Yes      | Must be a YAML list (may be empty)                |
| `id`           | string | Yes      | Unique within the workspace                       |
| `title`        | string | Yes      | Non-empty                                         |
| `status`       | string | Yes      | One of: `todo`, `doing`, `blocked`, `done`        |
| `description`  | string | No*      | May be empty string                               |
| `depends_on`   | list   | Yes      | Must be a YAML list; entries must reference valid task IDs |

\* `description` is not required in the schema itself, but for compatibility with existing task
contracts it should be provided. The parser accepts absent/nil description as empty string.

### ID rules

- ID convention: `TASK-<nnn>` with zero-padded decimal identifier (e.g., `TASK-001`).
- Hand-edited IDs may extend the numeric base with alphanumeric suffix or hyphen-delimited segments,
  e.g. `TASK-001a` or `TASK-001-1`.
- ID must be unique within the workspace.
- Duplicate IDs are a fatal validation error.

### Status rules

Valid statuses:

| Status    | Meaning                              |
|-----------|--------------------------------------|
| `todo`    | Not started                          |
| `doing`   | Currently in progress                |
| `blocked` | Blocked by another task or external  |
| `done`    | Completed                            |

Invalid status is a fatal validation error.

### Dependency rules

- `depends_on` must be a YAML list (may be empty: `[]`).
- Each entry must reference an existing task ID in the same workspace.
- Missing dependency target is a fatal validation error.
- Self-dependency (`id == depends_on entry`) is a fatal validation error.
- Dependency cycles are a fatal validation error.

### Field aliases

Only the exact field names defined above are supported. The following aliases are **not** supported:
- `dependsOn` (camelCase)
- `dependencies` or `dependencies_on`
- `blocked_by` or `blocks`

These are ignored for forward compatibility and do not satisfy the required `depends_on` field, so a task that
uses only an alias fails validation.

### Order

- The order of `tasks[]` is the authoring/default display order.
- Graph layout may derive from dependencies, but status lists and fallback ordering preserve
  `tasks[]` order where possible.

## Task model

The YAML task model maps to the existing `wstask.Item` struct:

```go
type Item struct {
    ID          string `json:"id"`
    Title       string `json:"title"`
    Status      wstask.Status `json:"status"`
    Description string `json:"description"`
    DependsOn   []string `json:"depends_on"`
}
```

## CLI integration

### kra ws task validate

Validates the current workspace's task document (YAML or MD). Reports:

- **Fatal errors**: YAML parse failure, schema violations, graph errors.
- **Warnings**: Workflow issues (non-fatal).

Exit code: non-zero for fatal errors; zero for warnings-only.

### kra ws task list / view / add / status

When `workspace.yaml` exists in a workspace:

- `list`, `view`: read tasks from `workspace.yaml`.
- `add`: append to `workspace.yaml` (generates next `TASK-<nnn>` ID).
- `status`: update only the target task's status; preserve all other fields.
- Mutation commands must preserve every existing task's `id`, `title`, `description`, `depends_on`, and
  declaration order except for the field the command explicitly changes.
- Mutation commands must fail closed without writing the file when YAML parsing, schema validation, or
  dependency-graph validation reports a fatal error.
- An existing but empty `workspace.yaml`, a missing/null `tasks`, or a missing/null `depends_on` is invalid;
  empty lists must be written explicitly as `tasks: []` and `depends_on: []`.
- List, view, and cmux sync operations also fail closed on invalid YAML; sync must not clear remote task state
  when the local structured document is invalid.
- Task document saves use a per-document process lock and compare the locked current content with the loaded
  snapshot before an atomic replacement. Concurrent mutations from the same snapshot yield one success and one
  conflict instead of silently overwriting either update.

When only `workspace.md` exists:

- All commands operate on `workspace.md` as before (no breaking changes).

## cmux native Todo integration

- `workspace.yaml` remains authoritative for task identity, description, dependencies, and order.
- Native Todo projects ID, title, and state only; dependency graphs remain available through local task commands.
- Synchronization follows `cmux-todo-reconcile.md` and fails closed on schema or graph diagnostics.

## Non-goals

- Editing task structure from cmux Todo (out of scope; only state is pulled).
- Dependency add/remove CLI commands (direct editing + validate is the workflow).
- tasks/*.yaml per-task files (task state centralized in workspace.yaml).
.verification evidence, timestamps, labels, assignees, priorities, events.

## Related docs

- workspace-tasks.md: existing `workspace.md` contract (legacy fallback).
- cmux-todo-reconcile.md: native Todo projection and conflict rules.
