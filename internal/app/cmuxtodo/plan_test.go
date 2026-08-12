package cmuxtodo

import (
	"errors"
	"strings"
	"testing"

	"github.com/tasuku43/kra/internal/cmuxmap"
)

func TestBuildPlanStateThreeWay(t *testing.T) {
	t.Parallel()
	base := todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 original")
	tests := []struct {
		name           string
		local          TaskStatus
		remote         TodoState
		prefer         Preference
		wantConflict   bool
		wantPull       TaskStatus
		wantRemote     TodoState
		wantStateWrite bool
	}{
		{name: "unchanged", local: TaskTodo, remote: TodoPending, wantRemote: TodoPending},
		{name: "local only", local: TaskDoing, remote: TodoPending, wantRemote: TodoInProgress, wantStateWrite: true},
		{name: "remote only", local: TaskTodo, remote: TodoCompleted, wantPull: TaskDone, wantRemote: TodoCompleted},
		{name: "same effective change", local: TaskDoing, remote: TodoInProgress, wantRemote: TodoInProgress},
		{name: "divergent", local: TaskDoing, remote: TodoCompleted, wantConflict: true},
		{name: "divergent prefer kra", local: TaskDoing, remote: TodoCompleted, prefer: PreferKRA, wantRemote: TodoInProgress, wantStateWrite: true},
		{name: "divergent prefer cmux", local: TaskDoing, remote: TodoCompleted, prefer: PreferCMUX, wantPull: TaskDone, wantRemote: TodoCompleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := buildPlan(
				[]Task{{ID: "TASK-001", Title: "original", Status: tt.local}},
				TodoList{Items: []TodoItem{{ID: "item-1", Text: "TASK-001 original", State: tt.remote, Origin: "agent"}}},
				cmuxmap.TodoMapping{CMUXWorkspaceID: "cmux-1", Items: []cmuxmap.TodoItemBase{base}},
				tt.prefer,
			)
			if err != nil {
				t.Fatalf("buildPlan() error: %v", err)
			}
			if got := hasConflict(plan.conflicts, ConflictState); got != tt.wantConflict {
				t.Fatalf("state conflict = %v, want %v; conflicts=%#v", got, tt.wantConflict, plan.conflicts)
			}
			if tt.wantConflict {
				return
			}
			if tt.wantPull == "" {
				if len(plan.pulls) != 0 {
					t.Fatalf("pulls = %#v, want none", plan.pulls)
				}
			} else if len(plan.pulls) != 1 || plan.pulls[0].To != tt.wantPull {
				t.Fatalf("pulls = %#v, want %q", plan.pulls, tt.wantPull)
			}
			gotWrite := false
			for _, mutation := range plan.mutations {
				if mutation.kind == mutationState {
					gotWrite = true
					if mutation.state != tt.wantRemote {
						t.Fatalf("remote state = %q, want %q", mutation.state, tt.wantRemote)
					}
				}
			}
			if gotWrite != tt.wantStateWrite {
				t.Fatalf("state write = %v, want %v; mutations=%#v", gotWrite, tt.wantStateWrite, plan.mutations)
			}
			if len(plan.base) != 1 || plan.base[0].state != tt.wantRemote {
				t.Fatalf("base = %#v, want state %q", plan.base, tt.wantRemote)
			}
		})
	}
}

func TestBuildPlanBlockedPendingAlias(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		baseStatus   TaskStatus
		localStatus  TaskStatus
		baseText     string
		remoteText   string
		wantEditText string
	}{
		{
			name:         "blocked to todo keeps pending and removes marker",
			baseStatus:   TaskBlocked,
			localStatus:  TaskTodo,
			baseText:     "⛔ TASK-001 work",
			remoteText:   "⛔ TASK-001 work",
			wantEditText: "TASK-001 work",
		},
		{
			name:         "todo to blocked keeps pending and adds marker",
			baseStatus:   TaskTodo,
			localStatus:  TaskBlocked,
			baseText:     "TASK-001 work",
			remoteText:   "TASK-001 work",
			wantEditText: "⛔ TASK-001 work",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := todoBase("TASK-001", "item-1", tt.baseStatus, tt.baseText)
			plan, err := buildPlan(
				[]Task{{ID: "TASK-001", Title: "work", Status: tt.localStatus}},
				TodoList{Items: []TodoItem{{ID: "item-1", Text: tt.remoteText, State: TodoPending, Origin: "agent"}}},
				cmuxmap.TodoMapping{Items: []cmuxmap.TodoItemBase{base}},
				PreferNone,
			)
			if err != nil {
				t.Fatalf("buildPlan() error: %v", err)
			}
			if len(plan.conflicts) != 0 || len(plan.pulls) != 0 {
				t.Fatalf("conflicts/pulls = %#v / %#v", plan.conflicts, plan.pulls)
			}
			if len(plan.mutations) != 1 || plan.mutations[0].kind != mutationEdit || plan.mutations[0].text != tt.wantEditText {
				t.Fatalf("mutations = %#v, want one edit to %q", plan.mutations, tt.wantEditText)
			}
			if plan.base[0].state != TodoPending || plan.base[0].status != tt.localStatus {
				t.Fatalf("base = %#v", plan.base[0])
			}
		})
	}
}

func TestBuildPlanTextOwnership(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		localTitle   string
		remoteText   string
		prefer       Preference
		wantConflict bool
		wantEdit     string
	}{
		{name: "unchanged", localTitle: "original", remoteText: "TASK-001 original"},
		{name: "local title push", localTitle: "local", remoteText: "TASK-001 original", wantEdit: "TASK-001 local"},
		{name: "remote drift conflicts", localTitle: "original", remoteText: "renamed remotely", wantConflict: true},
		{name: "same concurrent text converges", localTitle: "same", remoteText: "TASK-001 same"},
		{name: "divergent text conflicts", localTitle: "local", remoteText: "remote", wantConflict: true},
		{name: "prefer kra restores remote", localTitle: "original", remoteText: "remote", prefer: PreferKRA, wantEdit: "TASK-001 original"},
		{name: "prefer cmux cannot overwrite identity", localTitle: "original", remoteText: "remote", prefer: PreferCMUX, wantConflict: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 original")
			plan, err := buildPlan(
				[]Task{{ID: "TASK-001", Title: tt.localTitle, Status: TaskTodo}},
				TodoList{Items: []TodoItem{{ID: "item-1", Text: tt.remoteText, State: TodoPending, Origin: "agent"}}},
				cmuxmap.TodoMapping{Items: []cmuxmap.TodoItemBase{base}},
				tt.prefer,
			)
			if err != nil {
				t.Fatalf("buildPlan() error: %v", err)
			}
			if got := hasConflict(plan.conflicts, ConflictText); got != tt.wantConflict {
				t.Fatalf("text conflict = %v, want %v; conflicts=%#v", got, tt.wantConflict, plan.conflicts)
			}
			if tt.wantConflict {
				return
			}
			gotEdit := ""
			for _, mutation := range plan.mutations {
				if mutation.kind == mutationEdit {
					gotEdit = mutation.text
				}
			}
			if gotEdit != tt.wantEdit {
				t.Fatalf("edit text = %q, want %q; mutations=%#v", gotEdit, tt.wantEdit, plan.mutations)
			}
		})
	}
}

func TestBuildPlanDeletionPolicy(t *testing.T) {
	t.Parallel()
	base := todoBase("TASK-001", "item-1", TaskTodo, "TASK-001 original")
	tests := []struct {
		name         string
		local        []Task
		remote       []TodoItem
		prefer       Preference
		wantConflict ConflictKind
		wantAdd      bool
		wantBase     int
	}{
		{
			name:         "local deletion leaves managed remote untouched",
			remote:       []TodoItem{{ID: "item-1", Text: "TASK-001 original", State: TodoPending, Origin: "agent"}},
			wantConflict: ConflictLocalDeleted,
		},
		{
			name:     "both deleted drops base",
			wantBase: 0,
		},
		{
			name:         "remote deletion conflicts",
			local:        []Task{{ID: "TASK-001", Title: "original", Status: TaskTodo}},
			wantConflict: ConflictRemoteDeleted,
		},
		{
			name:         "remote deletion prefer cmux is still conflict",
			local:        []Task{{ID: "TASK-001", Title: "original", Status: TaskTodo}},
			prefer:       PreferCMUX,
			wantConflict: ConflictRemoteDeleted,
		},
		{
			name:     "remote deletion prefer kra recreates with new UUID",
			local:    []Task{{ID: "TASK-001", Title: "original", Status: TaskTodo}},
			prefer:   PreferKRA,
			wantAdd:  true,
			wantBase: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := buildPlan(
				tt.local,
				TodoList{Items: tt.remote},
				cmuxmap.TodoMapping{Items: []cmuxmap.TodoItemBase{base}},
				tt.prefer,
			)
			if err != nil {
				t.Fatalf("buildPlan() error: %v", err)
			}
			if tt.wantConflict != "" {
				if !hasConflict(plan.conflicts, tt.wantConflict) {
					t.Fatalf("conflicts = %#v, want %q", plan.conflicts, tt.wantConflict)
				}
				return
			}
			if len(plan.conflicts) != 0 {
				t.Fatalf("conflicts = %#v", plan.conflicts)
			}
			gotAdd := false
			for _, mutation := range plan.mutations {
				gotAdd = gotAdd || mutation.kind == mutationAdd
			}
			if gotAdd != tt.wantAdd || len(plan.base) != tt.wantBase {
				t.Fatalf("add/base = %v/%d, want %v/%d", gotAdd, len(plan.base), tt.wantAdd, tt.wantBase)
			}
		})
	}
}

func TestBuildPlanForeignCapacityAndPriority(t *testing.T) {
	t.Parallel()
	foreign := make([]TodoItem, 49)
	for i := range foreign {
		foreign[i] = TodoItem{ID: "foreign-" + string(rune('A'+i)), Text: "foreign", State: TodoPending, Origin: "user"}
	}
	tasks := []Task{
		{ID: "TODO", Title: "todo", Status: TaskTodo},
		{ID: "DONE", Title: "done", Status: TaskDone},
		{ID: "BLOCKED", Title: "blocked", Status: TaskBlocked},
		{ID: "DOING", Title: "doing", Status: TaskDoing},
	}
	plan, err := buildPlan(tasks, TodoList{Items: foreign}, cmuxmap.TodoMapping{}, PreferNone)
	if err != nil {
		t.Fatalf("buildPlan() error: %v", err)
	}
	if !plan.capacity || !hasConflict(plan.conflicts, ConflictCapacity) {
		t.Fatalf("capacity/conflicts = %v/%#v", plan.capacity, plan.conflicts)
	}
	if plan.foreign != 49 {
		t.Fatalf("foreign = %d, want 49", plan.foreign)
	}
	wantOrder := []string{"DOING", "BLOCKED", "TODO", "DONE"}
	if len(plan.mutations) != len(wantOrder) {
		t.Fatalf("mutations = %#v", plan.mutations)
	}
	for i, want := range wantOrder {
		if plan.mutations[i].taskID != want || plan.mutations[i].kind != mutationAdd {
			t.Fatalf("mutation[%d] = %#v, want add %s", i, plan.mutations[i], want)
		}
	}
}

func TestProjectionTextLimitPreservesTaskID(t *testing.T) {
	t.Parallel()
	task := Task{ID: "TASK-001", Title: strings.Repeat("界", 600), Status: TaskBlocked}
	plan, err := buildPlan([]Task{task}, TodoList{}, cmuxmap.TodoMapping{}, PreferNone)
	if err != nil {
		t.Fatalf("buildPlan() error: %v", err)
	}
	if len(plan.truncated) != 1 || plan.truncated[0] != task.ID {
		t.Fatalf("truncated = %#v", plan.truncated)
	}
	if len(plan.mutations) != 1 {
		t.Fatalf("mutations = %#v", plan.mutations)
	}
	text := plan.mutations[0].text
	if characterCount(text) != MaxTodoText {
		t.Fatalf("characters = %d, want %d", characterCount(text), MaxTodoText)
	}
	if !strings.HasPrefix(text, "⛔ TASK-001 ") {
		t.Fatalf("text does not preserve identity prefix: %q", text[:30])
	}
}

func TestProjectionUsesCMUXGraphemeCharacterLimit(t *testing.T) {
	t.Parallel()
	// The family emoji contains several code points but is one Swift Character,
	// matching cmux's String.prefix(500) behavior.
	family := "👨‍👩‍👧‍👦"
	task := Task{ID: "T", Title: strings.Repeat(family, MaxTodoText), Status: TaskTodo}
	text, truncated := projectText(task)
	if !truncated {
		t.Fatal("projectText() truncated = false")
	}
	if characterCount(text) != MaxTodoText || !strings.HasSuffix(text, family) {
		t.Fatalf("projection split a grapheme: characters=%d suffix=%v", characterCount(text), strings.HasSuffix(text, family))
	}
}

func TestValidateTasksFailClosedCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		tasks []Task
	}{
		{name: "empty id", tasks: []Task{{Title: "x", Status: TaskTodo}}},
		{name: "duplicate id", tasks: []Task{{ID: "A", Title: "x", Status: TaskTodo}, {ID: "A", Title: "y", Status: TaskDone}}},
		{name: "empty title", tasks: []Task{{ID: "A", Status: TaskTodo}}},
		{name: "newline title", tasks: []Task{{ID: "A", Title: "x\ny", Status: TaskTodo}}},
		{name: "invalid status", tasks: []Task{{ID: "A", Title: "x", Status: "wat"}}},
		{name: "identity exceeds cap", tasks: []Task{{ID: strings.Repeat("A", MaxTodoText+1), Title: "x", Status: TaskTodo}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := validateTasks(tt.tasks)
			if !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf("error = %v, want ErrInvalidDocument", err)
			}
		})
	}
}

func todoBase(taskID string, itemID string, status TaskStatus, text string) cmuxmap.TodoItemBase {
	return cmuxmap.TodoItemBase{
		KRATaskID:  taskID,
		CMUXItemID: itemID,
		KRAStatus:  string(status),
		CMUXState:  string(projectState(status)),
		Text:       text,
	}
}

func hasConflict(conflicts []Conflict, kind ConflictKind) bool {
	for _, conflict := range conflicts {
		if conflict.Kind == kind {
			return true
		}
	}
	return false
}
