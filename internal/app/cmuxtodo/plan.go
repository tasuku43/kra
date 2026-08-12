package cmuxtodo

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rivo/uniseg"
	"github.com/tasuku43/kra/internal/cmuxmap"
)

type mutationKind uint8

const (
	mutationAdd mutationKind = iota + 1
	mutationEdit
	mutationState
)

type mutation struct {
	kind       mutationKind
	taskID     string
	itemID     string
	text       string
	state      TodoState
	origin     string
	beforeText string
	before     TodoState
}

type baseTarget struct {
	taskID string
	itemID string
	status TaskStatus
	state  TodoState
	text   string
}

type reconcilePlan struct {
	mutations  []mutation
	pulls      []StatusChange
	base       []baseTarget
	conflicts  []Conflict
	truncated  []string
	foreign    int
	capacity   bool
	operations []Operation
}

func buildPlan(tasks []Task, remote TodoList, base cmuxmap.TodoMapping, prefer Preference) (reconcilePlan, error) {
	normalizedTasks, truncated, err := validateTasks(tasks)
	if err != nil {
		return reconcilePlan{}, err
	}
	remoteByID, err := validateRemote(remote)
	if err != nil {
		return reconcilePlan{}, err
	}
	baseByTask, ownedIDs, err := validateBase(base)
	if err != nil {
		return reconcilePlan{}, err
	}

	plan := reconcilePlan{truncated: truncated}
	for id := range remoteByID {
		if _, managed := ownedIDs[id]; !managed {
			plan.foreign++
		}
	}

	localByID := make(map[string]Task, len(normalizedTasks))
	order := make(map[string]int, len(normalizedTasks))
	for i, task := range normalizedTasks {
		localByID[task.ID] = task
		order[task.ID] = i
	}

	// A local deletion is intentionally not projected as a remote deletion.
	// Removing an existing Todo UUID cannot be compensated because cmux does
	// not let clients restore a caller-provided UUID.
	for _, item := range base.Items {
		if _, exists := localByID[item.KRATaskID]; exists {
			continue
		}
		if _, exists := remoteByID[item.CMUXItemID]; exists {
			plan.conflicts = append(plan.conflicts, Conflict{
				TaskID: item.KRATaskID,
				Kind:   ConflictLocalDeleted,
				Detail: "KRA task was deleted while its managed cmux Todo item still exists; delete that Todo item manually and retry",
			})
		}
		// If both sides deleted the task, omitting it from the next base is a
		// converged no-op.
	}

	byPriority := append([]Task(nil), normalizedTasks...)
	sort.SliceStable(byPriority, func(i, j int) bool {
		pi, pj := taskPriority(byPriority[i].Status), taskPriority(byPriority[j].Status)
		if pi != pj {
			return pi < pj
		}
		return order[byPriority[i].ID] < order[byPriority[j].ID]
	})

	for _, task := range byPriority {
		baseItem, hadBase := baseByTask[task.ID]
		if !hadBase {
			text, _ := projectText(task)
			state := projectState(task.Status)
			plan.mutations = append(plan.mutations, mutation{
				kind:   mutationAdd,
				taskID: task.ID,
				text:   text,
				state:  state,
				origin: "agent",
			})
			plan.operations = append(plan.operations, Operation{
				Kind:   OperationAdd,
				TaskID: task.ID,
				To:     fmt.Sprintf("%s | %s", state, text),
			})
			plan.base = append(plan.base, baseTarget{
				taskID: task.ID,
				status: task.Status,
				state:  state,
				text:   text,
			})
			continue
		}

		remoteItem, remoteExists := remoteByID[baseItem.CMUXItemID]
		if !remoteExists {
			if prefer != PreferKRA {
				detail := "managed cmux Todo item was deleted"
				if prefer == PreferCMUX {
					detail += "; --prefer cmux cannot delete the authoritative KRA task"
				}
				plan.conflicts = append(plan.conflicts, Conflict{
					TaskID: task.ID,
					Kind:   ConflictRemoteDeleted,
					Detail: detail,
				})
				continue
			}
			text, _ := projectText(task)
			state := projectState(task.Status)
			plan.mutations = append(plan.mutations, mutation{
				kind:   mutationAdd,
				taskID: task.ID,
				text:   text,
				state:  state,
				origin: "agent",
			})
			plan.operations = append(plan.operations, Operation{
				Kind:       OperationAdd,
				TaskID:     task.ID,
				CMUXItemID: baseItem.CMUXItemID,
				From:       "deleted",
				To:         fmt.Sprintf("%s | %s", state, text),
			})
			plan.base = append(plan.base, baseTarget{
				taskID: task.ID,
				status: task.Status,
				state:  state,
				text:   text,
			})
			continue
		}

		finalStatus, stateConflict := resolveState(task, baseItem, remoteItem, prefer)
		if stateConflict != nil {
			plan.conflicts = append(plan.conflicts, *stateConflict)
			continue
		}
		if finalStatus != task.Status {
			plan.pulls = append(plan.pulls, StatusChange{TaskID: task.ID, From: task.Status, To: finalStatus})
			plan.operations = append(plan.operations, Operation{
				Kind:       OperationPullStatus,
				TaskID:     task.ID,
				CMUXItemID: remoteItem.ID,
				From:       string(task.Status),
				To:         string(finalStatus),
			})
		}

		finalTask := task
		finalTask.Status = finalStatus
		finalText, _ := projectText(finalTask)
		textConflict := resolveTextConflict(task, baseItem, remoteItem, prefer)
		if textConflict != nil {
			plan.conflicts = append(plan.conflicts, *textConflict)
			continue
		}

		if remoteItem.Text != finalText {
			plan.mutations = append(plan.mutations, mutation{
				kind:       mutationEdit,
				taskID:     task.ID,
				itemID:     remoteItem.ID,
				text:       finalText,
				beforeText: remoteItem.Text,
			})
			plan.operations = append(plan.operations, Operation{
				Kind:       OperationEdit,
				TaskID:     task.ID,
				CMUXItemID: remoteItem.ID,
				From:       remoteItem.Text,
				To:         finalText,
			})
		}

		finalState := projectState(finalStatus)
		if remoteItem.State != finalState {
			plan.mutations = append(plan.mutations, mutation{
				kind:   mutationState,
				taskID: task.ID,
				itemID: remoteItem.ID,
				state:  finalState,
				before: remoteItem.State,
			})
			plan.operations = append(plan.operations, Operation{
				Kind:       OperationSetState,
				TaskID:     task.ID,
				CMUXItemID: remoteItem.ID,
				From:       string(remoteItem.State),
				To:         string(finalState),
			})
		}
		plan.base = append(plan.base, baseTarget{
			taskID: task.ID,
			itemID: remoteItem.ID,
			status: finalStatus,
			state:  finalState,
			text:   finalText,
		})
	}

	adds := 0
	for _, op := range plan.mutations {
		if op.kind == mutationAdd {
			adds++
		}
	}
	if len(remote.Items)+adds > MaxTodoItems {
		plan.capacity = true
		plan.conflicts = append(plan.conflicts, Conflict{
			Kind: ConflictCapacity,
			Detail: fmt.Sprintf(
				"cmux Todo has %d live items and reconciliation needs %d additions (maximum %d); foreign items are preserved",
				len(remote.Items), adds, MaxTodoItems,
			),
		})
	}

	return plan, nil
}

func resolveState(task Task, base cmuxmap.TodoItemBase, remote TodoItem, prefer Preference) (TaskStatus, *Conflict) {
	baseStatus := TaskStatus(base.KRAStatus)
	baseState := TodoState(base.CMUXState)
	localChanged := task.Status != baseStatus
	remoteChanged := remote.State != baseState

	if !remoteChanged {
		return task.Status, nil
	}
	remoteStatus := statusFromRemote(remote.State)
	if !localChanged {
		return remoteStatus, nil
	}
	if projectState(task.Status) == remote.State {
		return task.Status, nil
	}
	switch prefer {
	case PreferKRA:
		return task.Status, nil
	case PreferCMUX:
		return remoteStatus, nil
	default:
		return task.Status, &Conflict{
			TaskID: task.ID,
			Kind:   ConflictState,
			Detail: fmt.Sprintf(
				"KRA changed %s -> %s while cmux changed %s -> %s",
				baseStatus, task.Status, baseState, remote.State,
			),
		}
	}
}

func resolveTextConflict(task Task, base cmuxmap.TodoItemBase, remote TodoItem, prefer Preference) *Conflict {
	localText, _ := projectText(task)
	localChanged := localText != base.Text
	remoteChanged := remote.Text != base.Text
	if !remoteChanged || (localChanged && localText == remote.Text) {
		return nil
	}
	if prefer == PreferKRA {
		return nil
	}
	detail := fmt.Sprintf("managed text changed in cmux from %q to %q", base.Text, remote.Text)
	if prefer == PreferCMUX {
		detail += "; --prefer cmux cannot overwrite KRA task identity or title"
	}
	return &Conflict{TaskID: task.ID, Kind: ConflictText, Detail: detail}
}

func validateTasks(tasks []Task) ([]Task, []string, error) {
	out := make([]Task, len(tasks))
	truncated := make([]string, 0)
	seen := make(map[string]struct{}, len(tasks))
	for i, task := range tasks {
		task.ID = strings.TrimSpace(task.ID)
		task.Title = strings.TrimSpace(task.Title)
		if task.ID == "" {
			return nil, nil, fmt.Errorf("%w: task at index %d has no id", ErrInvalidDocument, i)
		}
		if strings.ContainsAny(task.ID, "\r\n") {
			return nil, nil, fmt.Errorf("%w: task %q id contains a newline", ErrInvalidDocument, task.ID)
		}
		if _, duplicate := seen[task.ID]; duplicate {
			return nil, nil, fmt.Errorf("%w: duplicate task id %q", ErrInvalidDocument, task.ID)
		}
		seen[task.ID] = struct{}{}
		if task.Title == "" {
			return nil, nil, fmt.Errorf("%w: task %q has no title", ErrInvalidDocument, task.ID)
		}
		if strings.ContainsAny(task.Title, "\r\n") {
			return nil, nil, fmt.Errorf("%w: task %q title contains a newline", ErrInvalidDocument, task.ID)
		}
		if !task.Status.Valid() {
			return nil, nil, fmt.Errorf("%w: task %q has invalid status %q", ErrInvalidDocument, task.ID, task.Status)
		}
		text, wasTruncated := projectText(task)
		if characterCount(projectPrefix(task)) > MaxTodoText || text == "" {
			return nil, nil, fmt.Errorf("%w: task %q id cannot fit in cmux Todo text", ErrInvalidDocument, task.ID)
		}
		if wasTruncated {
			truncated = append(truncated, task.ID)
		}
		out[i] = task
	}
	return out, truncated, nil
}

func validateRemote(remote TodoList) (map[string]TodoItem, error) {
	items := make(map[string]TodoItem, len(remote.Items))
	for i, item := range remote.Items {
		item.ID = strings.TrimSpace(item.ID)
		item.Text = strings.TrimSpace(item.Text)
		item.Origin = strings.TrimSpace(item.Origin)
		if item.ID == "" {
			return nil, fmt.Errorf("invalid cmux Todo response: item at index %d has no id", i)
		}
		if _, duplicate := items[item.ID]; duplicate {
			return nil, fmt.Errorf("invalid cmux Todo response: duplicate item id %q", item.ID)
		}
		if item.Text == "" {
			return nil, fmt.Errorf("invalid cmux Todo response: item %q has no text", item.ID)
		}
		if !item.State.Valid() {
			return nil, fmt.Errorf("invalid cmux Todo response: item %q has invalid state %q", item.ID, item.State)
		}
		if characterCount(item.Text) > MaxTodoText {
			return nil, fmt.Errorf("invalid cmux Todo response: item %q exceeds %d characters", item.ID, MaxTodoText)
		}
		items[item.ID] = item
	}
	return items, nil
}

func validateBase(base cmuxmap.TodoMapping) (map[string]cmuxmap.TodoItemBase, map[string]struct{}, error) {
	byTask := make(map[string]cmuxmap.TodoItemBase, len(base.Items))
	ownedIDs := make(map[string]struct{}, len(base.Items))
	for _, item := range base.Items {
		item.KRATaskID = strings.TrimSpace(item.KRATaskID)
		item.CMUXItemID = strings.TrimSpace(item.CMUXItemID)
		item.KRAStatus = strings.TrimSpace(item.KRAStatus)
		item.CMUXState = strings.TrimSpace(item.CMUXState)
		item.Text = strings.TrimSpace(item.Text)
		status := TaskStatus(item.KRAStatus)
		state := TodoState(item.CMUXState)
		if item.KRATaskID == "" || item.CMUXItemID == "" || item.Text == "" {
			return nil, nil, fmt.Errorf("%w: base contains an incomplete item", ErrInvalidBase)
		}
		if !status.Valid() || !state.Valid() {
			return nil, nil, fmt.Errorf("%w: task %q has invalid status/state", ErrInvalidBase, item.KRATaskID)
		}
		if characterCount(item.Text) > MaxTodoText {
			return nil, nil, fmt.Errorf("%w: task %q text exceeds %d characters", ErrInvalidBase, item.KRATaskID, MaxTodoText)
		}
		if projectState(status) != state {
			return nil, nil, fmt.Errorf("%w: task %q has inconsistent KRA status %q and cmux state %q", ErrInvalidBase, item.KRATaskID, status, state)
		}
		if _, duplicate := byTask[item.KRATaskID]; duplicate {
			return nil, nil, fmt.Errorf("%w: duplicate task id %q", ErrInvalidBase, item.KRATaskID)
		}
		if _, duplicate := ownedIDs[item.CMUXItemID]; duplicate {
			return nil, nil, fmt.Errorf("%w: duplicate cmux item id %q", ErrInvalidBase, item.CMUXItemID)
		}
		byTask[item.KRATaskID] = item
		ownedIDs[item.CMUXItemID] = struct{}{}
	}
	return byTask, ownedIDs, nil
}

func projectState(status TaskStatus) TodoState {
	switch status {
	case TaskDoing:
		return TodoInProgress
	case TaskDone:
		return TodoCompleted
	default:
		return TodoPending
	}
}

func statusFromRemote(state TodoState) TaskStatus {
	switch state {
	case TodoInProgress:
		return TaskDoing
	case TodoCompleted:
		return TaskDone
	default:
		return TaskTodo
	}
}

func projectPrefix(task Task) string {
	prefix := task.ID + " "
	if task.Status == TaskBlocked {
		prefix = "⛔ " + prefix
	}
	return prefix
}

func projectText(task Task) (string, bool) {
	text := projectPrefix(task) + task.Title
	if characterCount(text) <= MaxTodoText {
		return text, false
	}
	graphemes := uniseg.NewGraphemes(text)
	var out strings.Builder
	for i := 0; i < MaxTodoText && graphemes.Next(); i++ {
		out.WriteString(graphemes.Str())
	}
	return out.String(), true
}

func characterCount(value string) int {
	count := 0
	graphemes := uniseg.NewGraphemes(value)
	for graphemes.Next() {
		count++
	}
	return count
}

func taskPriority(status TaskStatus) int {
	switch status {
	case TaskDoing:
		return 0
	case TaskBlocked:
		return 1
	case TaskTodo:
		return 2
	case TaskDone:
		return 3
	default:
		return 4
	}
}
