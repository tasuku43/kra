package wstask

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// YAMLWorkspace represents the top-level workspace.yaml structure.
type YAMLWorkspace struct {
	SchemaVersion int        `yaml:"schema_version"`
	Tasks         []YAMLTask `yaml:"tasks"`
}

// YAMLTask represents a single task entry in workspace.yaml.
type YAMLTask struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Status      Status   `yaml:"status"`
	Description string   `yaml:"description,omitempty"`
	DependsOn   []string `yaml:"depends_on"`
}

// ParseYAMLWorkspace parses raw YAML content into a YAMLWorkspace.
func ParseYAMLWorkspace(content []byte) (*YAMLWorkspace, error) {
	var ws YAMLWorkspace
	if err := yaml.Unmarshal(content, &ws); err != nil {
		return nil, fmt.Errorf("parse workspace.yaml: %w", err)
	}
	return &ws, nil
}

// Validate checks schema and graph invariants on the parsed YAMLWorkspace.
// It returns (Items, Diagnostics). Items are valid task items; diagnostics contain
// both fatal errors and workflow warnings.
func Validate(ws *YAMLWorkspace) ([]Item, Diagnostics) {
	var diags Diagnostics

	// 1. schema_version check
	if ws.SchemaVersion == 0 {
		diags.Errors = append(diags.Errors, "missing schema_version")
		return nil, diags
	}
	if ws.SchemaVersion != 1 {
		diags.Errors = append(diags.Errors, fmt.Sprintf("unsupported schema_version: %d", ws.SchemaVersion))
		return nil, diags
	}

	// 2. tasks must be a list (YAML unmarshal always produces a slice for lists)
	// If 'tasks' is absent in YAML, ws.Tasks will be nil/empty.
	if len(ws.Tasks) == 0 {
		return nil, diags
	}

	items := make([]Item, 0, len(ws.Tasks))

	// 3. Validate each task entry
	idSet := make(map[string]int) // id -> index in items (for duplicates detection)
	for i, t := range ws.Tasks {
		// id required
		if t.ID == "" {
			diags.Errors = append(diags.Errors, fmt.Sprintf("tasks[%d]: missing task ID", i))
			continue
		}

		// duplicate id
		if prevIdx, dup := idSet[t.ID]; dup {
			diags.Errors = append(diags.Errors, fmt.Sprintf("duplicate task ID: %s (first at tasks[%d])", t.ID, prevIdx))
			diags.Errors = append(diags.Errors, fmt.Sprintf("  duplicate at tasks[%d]", i))
			continue
		}
		idSet[t.ID] = i

		// title required and non-empty
		if t.Title == "" {
			diags.Errors = append(diags.Errors, fmt.Sprintf("task %s: missing or empty title", t.ID))
			continue
		}

		// status required and valid
		if t.Status == "" {
			diags.Errors = append(diags.Errors, fmt.Sprintf("task %s: missing status", t.ID))
			continue
		}
		if !isValidStatus(t.Status) {
			diags.Errors = append(diags.Errors, fmt.Sprintf("task %s: invalid status %q", t.ID, t.Status))
			continue
		}

		// depends_on must be a list
		if t.DependsOn == nil {
			t.DependsOn = []string{}
		}

		item := Item{
			ID:          t.ID,
			Title:       t.Title,
			Status:      t.Status,
			Description: t.Description,
		}
		items = append(items, item)
	}

	if len(diags.Errors) > 0 {
		return nil, diags
	}

	// 4. Dependency validation (only if no schema errors)
	idMap := make(map[string]struct{})
	for _, item := range items {
		idMap[item.ID] = struct{}{}
	}

	// Build adjacency list for cycle detection: edge from depends_on -> task
	// i.e., if TASK-B depends on TASK-A, edge is TASK-A -> TASK-B
	adjList := make(map[string][]string) // from -> [to]
	inDegree := make(map[string]int)     // in-degree for topo sort
	for _, item := range items {
		inDegree[item.ID] = 0
	}
	for _, t := range ws.Tasks {
		for _, dep := range t.DependsOn {
			if dep == "" {
				continue
			}
			if _, exists := idMap[dep]; !exists {
				diags.Errors = append(diags.Errors, fmt.Sprintf("task %s: depends_on references missing task ID %q", t.ID, dep))
				continue
			}
			if dep == t.ID {
				diags.Errors = append(diags.Errors, fmt.Sprintf("task %s: self-dependency", t.ID))
				continue
			}
			adjList[dep] = append(adjList[dep], t.ID)
			inDegree[t.ID]++
		}
	}

	if len(diags.Errors) > 0 {
		return nil, diags
	}

	// Cycle detection via Kahn's algorithm
	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	visitedCount := 0
	processed := make(map[string]bool)
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		processed[node] = true
		visitedCount++

		for _, neighbor := range adjList[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if visitedCount != len(items) {
		cycleNodes := make([]string, 0)
		for id := range inDegree {
			if !processed[id] {
				cycleNodes = append(cycleNodes, id)
			}
		}
		sort.Strings(cycleNodes)
		diags.Errors = append(diags.Errors, fmt.Sprintf("dependency cycle detected among tasks: %v", cycleNodes))
		return nil, diags
	}

	// 5. Workflow warnings (non-fatal)
	for _, item := range items {
		if item.Status == StatusDone {
			continue
		}
		for _, dep := range ws.Tasks[idSet[item.ID]].DependsOn {
			if dep == "" {
				continue
			}
			depItem, ok := findItemByID(items, dep)
			if !ok {
				continue
			}
			switch item.Status {
			case StatusDoing:
				if depItem.Status != StatusDone {
					diags.Warnings = append(diags.Warnings, fmt.Sprintf("task %s (doing) depends on %q (%s), which is not done", item.ID, dep, depItem.Status))
				}
			case StatusDone:
				if depItem.Status != StatusDone {
					diags.Warnings = append(diags.Warnings, fmt.Sprintf("task %s (done) depends on %q (%s), which is not done", item.ID, dep, depItem.Status))
				}
			case StatusBlocked:
				if depItem.Status == StatusDone {
					diags.Warnings = append(diags.Warnings, fmt.Sprintf("task %s (blocked) has no unfinished dependencies: all deps including %q are done", item.ID, dep))
				}
			}
		}
	}

	return items, diags
}

func findItemByID(items []Item, id string) (*Item, bool) {
	for i := range items {
		if items[i].ID == id {
			return &items[i], true
		}
	}
	return nil, false
}

func isValidStatus(s Status) bool {
	switch s {
	case StatusTodo, StatusDoing, StatusBlocked, StatusDone:
		return true
	}
	return false
}

// RenderYAML writes the workspace back to YAML format.
func RenderYAML(ws *YAMLWorkspace) ([]byte, error) {
	return yaml.Marshal(ws)
}

// Diagnostics holds validation results.
type Diagnostics struct {
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// HasErrors reports whether there are any fatal errors.
func (d *Diagnostics) HasErrors() bool {
	return len(d.Errors) > 0
}

// HasWarnings reports whether there are any workflow warnings.
func (d *Diagnostics) HasWarnings() bool {
	return len(d.Warnings) > 0
}
