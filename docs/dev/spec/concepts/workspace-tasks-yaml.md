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
      Read the existing workspace task parser, task commands, and kra serve board implementation.
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

These are silently ignored for forward compatibility but will not be parsed.

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

When only `workspace.md` exists:

- All commands operate on `workspace.md` as before (no breaking changes).

## kra serve integration

The serve API payload extends each workspace with `task_graph`:

```json
{
  "workspaces": [
    {
      "id": "PROJ-2314",
      "title": "Workspace title",
      "progress": { "done": 1, "total": 3, "percent": 33 },
      "tasks": { "todo": [], "doing": [], "blocked": [], "done": [] },
      "task_graph": {
        "nodes": [
          { "id": "TASK-001", "title": "...", "status": "done" }
        ],
        "edges": [
          { "from": "TASK-001", "to": "TASK-002" }
        ],
        "diagnostics": { "errors": [], "warnings": [] }
      }
    }
  ]
}
```

The browser renders:

- A task dependency graph view (SVG-based, read-only).
- Each node shows: ID, title, status.
- Each edge is a directed arrow from dependency to dependent.
- Clicking a node highlights it and its upstream dependency path.
- Unfinished blockers are displayed in the focus panel.

The existing status board remains as a secondary/fallback view.

## Non-goals

- Browser-side task editing (out of scope).
- Dependency add/remove CLI commands (direct editing + validate is the workflow).
- tasks/*.yaml per-task files (task state centralized in workspace.yaml).
.verification evidence, timestamps, labels, assignees, priorities, events.

## Related docs

- workspace-tasks.md: existing `workspace.md` contract (legacy fallback).
- serve.md: `kra serve` specification with graph view notes above.
