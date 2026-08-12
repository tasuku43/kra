package cmuxtodo

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
)

const compensationTimeout = 5 * time.Second

func (s *Service) Reconcile(ctx context.Context, input Request) (Result, error) {
	req, err := input.normalized()
	if err != nil {
		return Result{}, err
	}
	if s == nil || s.NewStore == nil || s.NewClient == nil || s.Documents == nil {
		return Result{}, fmt.Errorf("cmux Todo service is not initialized")
	}

	store := s.NewStore(req.Root)
	if store == nil {
		return Result{}, fmt.Errorf("cmux Todo mapping store is not initialized")
	}
	release, err := store.AcquireLock(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("acquire cmux Todo mapping lock: %w", err)
	}
	defer release()

	mapping, err := store.Load()
	if err != nil {
		return Result{}, fmt.Errorf("load cmux Todo mapping: %w", err)
	}
	cmuxWorkspaceID, err := resolveTarget(mapping, req)
	if err != nil {
		return Result{}, err
	}

	local, err := s.Documents.List(ctx, req.Root, req.WorkspaceID, req.Scope)
	if err != nil {
		return Result{}, fmt.Errorf("load KRA task document: %w", err)
	}
	// This validation is deliberately performed before the first live Todo
	// read. Invalid task documents are a fail-closed local error.
	normalizedTasks, _, err := validateTasks(local.Tasks)
	if err != nil {
		return Result{}, err
	}
	local.Tasks = normalizedTasks

	client := s.NewClient()
	if client == nil {
		return Result{}, fmt.Errorf("cmux Todo client is not initialized")
	}
	if checker, ok := client.(CapabilityChecker); ok {
		if err := checker.CheckCapabilities(ctx, cmuxWorkspaceID); err != nil {
			return Result{}, err
		}
	}
	remote, err := client.ListTodos(ctx, cmuxWorkspaceID)
	if err != nil {
		return Result{}, fmt.Errorf("list cmux Todo: %w", err)
	}
	if err := validateTodoListWorkspace(remote, cmuxWorkspaceID); err != nil {
		return Result{}, fmt.Errorf("list cmux Todo: %w", err)
	}

	base := mapping.Todos[req.WorkspaceID]
	if strings.TrimSpace(base.CMUXWorkspaceID) != cmuxWorkspaceID {
		// A replaced runtime has a distinct checklist. The old UUID ledger
		// must not cause accidental adoption of items in the new workspace.
		base = cmuxmap.TodoMapping{CMUXWorkspaceID: cmuxWorkspaceID, Items: []cmuxmap.TodoItemBase{}}
	}
	plan, err := buildPlan(local.Tasks, remote, base, req.Prefer)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		WorkspaceID:     req.WorkspaceID,
		CMUXWorkspaceID: cmuxWorkspaceID,
		DryRun:          req.DryRun,
		ForeignCount:    plan.foreign,
		ManagedCount:    len(plan.base),
		Truncated:       append([]string(nil), plan.truncated...),
		Conflicts:       append([]Conflict(nil), plan.conflicts...),
	}
	if len(plan.conflicts) > 0 {
		if plan.capacity {
			return result, ErrCapacity
		}
		return result, ErrConflict
	}
	result.Changed = len(plan.mutations) > 0 || len(plan.pulls) > 0
	result.Operations = append([]Operation(nil), plan.operations...)
	if req.DryRun {
		return result, nil
	}

	expectedRemote := cloneTodoList(remote)
	createdIDs := make(map[string]string)
	undos := make([]remoteUndo, 0, len(plan.mutations))
	for _, op := range plan.mutations {
		live, listErr := client.ListTodos(ctx, cmuxWorkspaceID)
		if listErr != nil {
			cause := fmt.Errorf("re-read cmux Todo before %s for %s: %w", op.kind, op.taskID, listErr)
			return failedResult(result, s.recover(ctx, req, client, cmuxWorkspaceID, cause, "", nil, undos))
		}
		if listErr := validateTodoListWorkspace(live, cmuxWorkspaceID); listErr != nil {
			cause := fmt.Errorf("re-read cmux Todo before %s for %s: %w", op.kind, op.taskID, listErr)
			return failedResult(result, s.recover(ctx, req, client, cmuxWorkspaceID, cause, "", nil, undos))
		}
		if !equalTodoLists(expectedRemote, live) {
			result.Conflicts = append(result.Conflicts, Conflict{
				TaskID: op.taskID,
				Kind:   ConflictConcurrentTodo,
				Detail: "live cmux Todo changed after planning",
			})
			cause := fmt.Errorf("%w: before %s for task %s", ErrConcurrentTodo, op.kind, op.taskID)
			return failedResult(result, s.recover(ctx, req, client, cmuxWorkspaceID, cause, "", nil, undos))
		}
		undo, applied, applyErr := applyRemoteMutation(ctx, client, cmuxWorkspaceID, op, &expectedRemote, createdIDs)
		if applied {
			undos = append(undos, undo)
		}
		if applyErr != nil {
			return failedResult(result, s.recover(ctx, req, client, cmuxWorkspaceID, applyErr, "", nil, undos))
		}
	}

	postLocal := local
	localApplied := false
	if len(plan.pulls) > 0 {
		postLocal, err = s.Documents.ApplyStatuses(
			ctx,
			req.Root,
			req.WorkspaceID,
			req.Scope,
			local.Revision,
			append([]StatusChange(nil), plan.pulls...),
		)
		if err != nil {
			cause := fmt.Errorf("apply KRA task statuses: %w", err)
			return failedResult(result, s.recover(ctx, req, client, cmuxWorkspaceID, cause, "", nil, undos))
		}
		localApplied = true
	}
	expectedTasks := applyStatusChanges(local.Tasks, plan.pulls)
	liveLocal, err := s.Documents.List(ctx, req.Root, req.WorkspaceID, req.Scope)
	if err != nil {
		cause := fmt.Errorf("verify KRA task document: %w", err)
		return failedResult(result, s.recoverLocal(ctx, req, client, cmuxWorkspaceID, cause, postLocal, localApplied, plan.pulls, undos))
	}
	if liveLocal.Revision != postLocal.Revision || !equalTasks(expectedTasks, liveLocal.Tasks) {
		cause := fmt.Errorf("verify KRA task document: document changed during reconciliation")
		return failedResult(result, s.recoverLocal(ctx, req, client, cmuxWorkspaceID, cause, postLocal, localApplied, plan.pulls, undos))
	}

	liveRemote, err := client.ListTodos(ctx, cmuxWorkspaceID)
	if err != nil {
		cause := fmt.Errorf("verify cmux Todo: %w", err)
		return failedResult(result, s.recoverLocal(ctx, req, client, cmuxWorkspaceID, cause, postLocal, localApplied, plan.pulls, undos))
	}
	if err := validateTodoListWorkspace(liveRemote, cmuxWorkspaceID); err != nil {
		cause := fmt.Errorf("verify cmux Todo: %w", err)
		return failedResult(result, s.recoverLocal(ctx, req, client, cmuxWorkspaceID, cause, postLocal, localApplied, plan.pulls, undos))
	}
	if !equalTodoLists(expectedRemote, liveRemote) {
		result.Conflicts = append(result.Conflicts, Conflict{
			Kind:   ConflictConcurrentTodo,
			Detail: "live cmux Todo changed before final verification",
		})
		cause := fmt.Errorf("%w: final verification", ErrConcurrentTodo)
		return failedResult(result, s.recoverLocal(ctx, req, client, cmuxWorkspaceID, cause, postLocal, localApplied, plan.pulls, undos))
	}

	finalBase, err := materializeBase(plan.base, createdIDs, liveRemote)
	if err != nil {
		return failedResult(result, s.recoverLocal(ctx, req, client, cmuxWorkspaceID, err, postLocal, localApplied, plan.pulls, undos))
	}
	if mapping.Todos == nil {
		mapping.Todos = make(map[string]cmuxmap.TodoMapping)
	}
	mapping.Todos[req.WorkspaceID] = cmuxmap.TodoMapping{
		CMUXWorkspaceID: cmuxWorkspaceID,
		Items:           finalBase,
		SyncedAt:        s.syncedAt(),
	}
	if err := store.Save(mapping); err != nil {
		cause := fmt.Errorf("save cmux Todo base: %w", err)
		return failedResult(result, s.recoverLocal(ctx, req, client, cmuxWorkspaceID, cause, postLocal, localApplied, plan.pulls, undos))
	}
	return result, nil
}

func failedResult(result Result, err error) (Result, error) {
	// Operations describe committed reconciliation work. Dry-run exposes the
	// plan, but a failed transaction is compensated and must not be reported as
	// pushed/created/pulled. RecoveryError carries any uncertain remainder.
	result.Changed = false
	result.Operations = nil
	return result, err
}

func resolveTarget(mapping cmuxmap.File, req Request) (string, error) {
	if req.Target == TargetRoot {
		if mapping.RootGroup == nil || strings.TrimSpace(mapping.RootGroup.AnchorWorkspaceID) == "" {
			return "", fmt.Errorf("%w: root cmux group anchor is missing", ErrNotMapped)
		}
		return strings.TrimSpace(mapping.RootGroup.AnchorWorkspaceID), nil
	}
	workspace, ok := mapping.Workspaces[req.WorkspaceID]
	if !ok || len(workspace.Entries) == 0 {
		return "", fmt.Errorf("%w: workspace %q", ErrNotMapped, req.WorkspaceID)
	}
	id := strings.TrimSpace(workspace.Entries[0].CMUXWorkspaceID)
	if id == "" {
		return "", fmt.Errorf("%w: workspace %q has an empty cmux id", ErrNotMapped, req.WorkspaceID)
	}
	return id, nil
}

func (s *Service) syncedAt() string {
	if s.Now != nil {
		return s.Now().UTC().Format(time.RFC3339)
	}
	return time.Now().UTC().Format(time.RFC3339)
}

type remoteUndo struct {
	kind       mutationKind
	taskID     string
	itemID     string
	beforeText string
	before     TodoState
}

func applyRemoteMutation(
	ctx context.Context,
	client Client,
	workspaceID string,
	op mutation,
	expected *TodoList,
	createdIDs map[string]string,
) (remoteUndo, bool, error) {
	switch op.kind {
	case mutationAdd:
		item, err := client.AddTodo(ctx, workspaceID, TodoInput{Text: op.text, State: op.state, Origin: op.origin})
		if err != nil {
			cause := fmt.Errorf("add cmux Todo for task %s: %w", op.taskID, err)
			return observeFailedMutation(ctx, client, workspaceID, op, *expected, cause)
		}
		item = normalizedTodoItem(item)
		if item.ID == "" {
			cause := fmt.Errorf("add cmux Todo for task %s returned an empty id", op.taskID)
			return remoteUndo{}, false, &RecoveryError{
				Cause: cause,
				Repair: []error{fmt.Errorf(
					"successful add for task %s returned no trustworthy item UUID; no deletion was attempted; inspect the cmux Todo list manually",
					op.taskID,
				)},
			}
		}
		if findTodo(expected.Items, item.ID) >= 0 {
			cause := fmt.Errorf("add cmux Todo for task %s returned duplicate id %q", op.taskID, item.ID)
			return remoteUndo{}, false, &RecoveryError{
				Cause: cause,
				Repair: []error{fmt.Errorf(
					"successful add for task %s returned no trustworthy new item UUID; no deletion was attempted; inspect the cmux Todo list manually",
					op.taskID,
				)},
			}
		}
		undo := remoteUndo{kind: mutationAdd, taskID: op.taskID, itemID: item.ID}
		if item.Text != op.text || item.State != op.state || item.Origin != op.origin {
			return undo, true, unexpectedMutationResponse(
				fmt.Errorf("add cmux Todo for task %s returned unexpected item %#v", op.taskID, item),
				op.taskID,
				item.ID,
			)
		}
		expected.Items = append(expected.Items, item)
		createdIDs[op.taskID] = item.ID
		return undo, true, nil

	case mutationEdit:
		index := findTodo(expected.Items, op.itemID)
		if index < 0 {
			return remoteUndo{}, false, fmt.Errorf("edit cmux Todo for task %s: item %q disappeared", op.taskID, op.itemID)
		}
		before := expected.Items[index]
		item, err := client.EditTodo(ctx, workspaceID, op.itemID, op.text)
		if err != nil {
			cause := fmt.Errorf("edit cmux Todo for task %s: %w", op.taskID, err)
			return observeFailedMutation(ctx, client, workspaceID, op, *expected, cause)
		}
		item = normalizedTodoItem(item)
		undo := remoteUndo{
			kind:       mutationEdit,
			taskID:     op.taskID,
			itemID:     op.itemID,
			beforeText: before.Text,
		}
		if item.ID != op.itemID || item.Text != op.text || item.State != before.State || item.Origin != before.Origin {
			return undo, true, unexpectedMutationResponse(
				fmt.Errorf("edit cmux Todo for task %s returned unexpected item %#v", op.taskID, item),
				op.taskID,
				op.itemID,
			)
		}
		expected.Items[index] = item
		return undo, true, nil

	case mutationState:
		index := findTodo(expected.Items, op.itemID)
		if index < 0 {
			return remoteUndo{}, false, fmt.Errorf("set cmux Todo state for task %s: item %q disappeared", op.taskID, op.itemID)
		}
		before := expected.Items[index]
		item, err := client.SetTodoState(ctx, workspaceID, op.itemID, op.state)
		if err != nil {
			cause := fmt.Errorf("set cmux Todo state for task %s: %w", op.taskID, err)
			return observeFailedMutation(ctx, client, workspaceID, op, *expected, cause)
		}
		item = normalizedTodoItem(item)
		undo := remoteUndo{
			kind:   mutationState,
			taskID: op.taskID,
			itemID: op.itemID,
			before: before.State,
		}
		if item.ID != op.itemID || item.State != op.state || item.Text != before.Text || item.Origin != before.Origin {
			return undo, true, unexpectedMutationResponse(
				fmt.Errorf("set cmux Todo state for task %s returned unexpected item %#v", op.taskID, item),
				op.taskID,
				op.itemID,
			)
		}
		expected.Items[index] = item
		return undo, true, nil
	default:
		return remoteUndo{}, false, fmt.Errorf("unknown cmux Todo mutation %d", op.kind)
	}
}

func unexpectedMutationResponse(cause error, taskID string, itemID string) error {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		itemID = "unknown"
	}
	return &RecoveryError{
		Cause: cause,
		Repair: []error{fmt.Errorf(
			"successful mutation for task %s returned an untrustworthy payload (item %s); no remote undo was attempted; inspect the cmux Todo list manually",
			taskID,
			itemID,
		)},
	}
}

// An RPC error does not prove that a local-socket mutation had no effect. A
// post-error read can prove only that the before image is still intact. Even
// an exact intended after image is causally ambiguous because cmux has no
// mutation revision/CAS token: a user or another agent may have produced it.
// Never undo that ambiguous state, because doing so could overwrite or delete
// a foreign mutation.
func observeFailedMutation(
	ctx context.Context,
	client Client,
	workspaceID string,
	op mutation,
	expected TodoList,
	cause error,
) (remoteUndo, bool, error) {
	inspectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), compensationTimeout)
	defer cancel()
	live, err := client.ListTodos(inspectCtx, workspaceID)
	if err != nil {
		return remoteUndo{}, false, &RecoveryError{
			Cause:  cause,
			Repair: []error{fmt.Errorf("inspect mutation effect for task %s: %w", op.taskID, err)},
		}
	}
	if err := validateTodoListWorkspace(live, workspaceID); err != nil {
		return remoteUndo{}, false, &RecoveryError{
			Cause:  cause,
			Repair: []error{fmt.Errorf("inspect mutation effect for task %s: %w", op.taskID, err)},
		}
	}
	if equalTodoLists(expected, live) {
		return remoteUndo{}, false, cause
	}
	return remoteUndo{}, false, &RecoveryError{
		Cause:  cause,
		Repair: []error{fmt.Errorf("cmux Todo changed after failed mutation for task %s; mutation ownership is causally ambiguous and no undo was attempted", op.taskID)},
	}
}

func (s *Service) recoverLocal(
	ctx context.Context,
	req Request,
	client Client,
	workspaceID string,
	cause error,
	postLocal TaskSnapshot,
	localApplied bool,
	pulls []StatusChange,
	undos []remoteUndo,
) error {
	expectedRevision := ""
	if localApplied {
		expectedRevision = postLocal.Revision
	}
	return s.recover(ctx, req, client, workspaceID, cause, expectedRevision, pulls, undos)
}

func (s *Service) recover(
	ctx context.Context,
	req Request,
	client Client,
	workspaceID string,
	cause error,
	localRevision string,
	pulls []StatusChange,
	undos []remoteUndo,
) error {
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), compensationTimeout)
	defer cancel()
	repairs := make([]error, 0)
	if localRevision != "" && len(pulls) > 0 {
		inverse := make([]StatusChange, len(pulls))
		for i, change := range pulls {
			inverse[i] = StatusChange{TaskID: change.TaskID, From: change.To, To: change.From}
		}
		if _, err := s.Documents.ApplyStatuses(
			recoveryCtx,
			req.Root,
			req.WorkspaceID,
			req.Scope,
			localRevision,
			inverse,
		); err != nil {
			repairs = append(repairs, fmt.Errorf("restore KRA task statuses: %w", err))
		}
	}
	for _, err := range auditRemoteRecovery(recoveryCtx, client, workspaceID, undos) {
		repairs = append(repairs, err)
	}
	if len(repairs) == 0 {
		return cause
	}
	if existing, ok := cause.(*RecoveryError); ok {
		combined := append([]error(nil), existing.Repair...)
		combined = append(combined, repairs...)
		return &RecoveryError{Cause: existing.Cause, Repair: combined}
	}
	return &RecoveryError{Cause: cause, Repair: repairs}
}

// auditRemoteRecovery verifies whether remote mutations have already returned
// to their before image. It deliberately performs no Todo mutation: cmux does
// not expose a revision or conditional mutation token, so any read-then-write
// rollback could overwrite an intervening user or agent change.
func auditRemoteRecovery(ctx context.Context, client Client, workspaceID string, undos []remoteUndo) []error {
	repairs := make([]error, 0)
	for i := len(undos) - 1; i >= 0; i-- {
		undo := undos[i]
		live, err := client.ListTodos(ctx, workspaceID)
		if err != nil {
			repairs = append(repairs, fmt.Errorf("inspect cmux Todo before restoring task %s: %w", undo.taskID, err))
			continue
		}
		if err := validateTodoListWorkspace(live, workspaceID); err != nil {
			repairs = append(repairs, fmt.Errorf("inspect cmux Todo before restoring task %s: %w", undo.taskID, err))
			continue
		}
		index := findTodo(live.Items, undo.itemID)
		switch undo.kind {
		case mutationAdd:
			if index < 0 {
				continue
			}
			// Todo list payloads omit user-owned attachments. Even when the
			// visible fields still match the added projection, removing this UUID could
			// discard an attachment added concurrently. Keep the item and make
			// the uncertain ownership explicit for manual repair.
			repairs = append(repairs, fmt.Errorf(
				"preserve added cmux Todo for task %s (item %s): attachments and concurrent ownership cannot be verified; inspect it manually",
				undo.taskID,
				undo.itemID,
			))

		case mutationEdit:
			if index < 0 {
				repairs = append(repairs, fmt.Errorf("restore cmux Todo text for task %s: item was deleted", undo.taskID))
				continue
			}
			current := normalizedTodoItem(live.Items[index])
			if current.Text == undo.beforeText {
				continue
			}
			// cmux has no conditional edit/CAS. A second RPC after this read
			// could overwrite a user edit made in between, even when the field
			// currently equals our intended after image. Preserve it and report
			// the stale base instead of attempting a racy undo.
			repairs = append(repairs, fmt.Errorf(
				"preserve changed cmux Todo text for task %s (item %s): cmux has no compare-and-set; inspect it manually",
				undo.taskID,
				undo.itemID,
			))

		case mutationState:
			if index < 0 {
				repairs = append(repairs, fmt.Errorf("restore cmux Todo state for task %s: item was deleted", undo.taskID))
				continue
			}
			current := normalizedTodoItem(live.Items[index])
			if current.State == undo.before {
				continue
			}
			// As with text, a read followed by set_state is not a CAS. Never
			// overwrite an interactive state change during failure recovery.
			repairs = append(repairs, fmt.Errorf(
				"preserve changed cmux Todo state for task %s (item %s): cmux has no compare-and-set; inspect it manually",
				undo.taskID,
				undo.itemID,
			))
		}
	}
	return repairs
}

func validateTodoListWorkspace(list TodoList, expected string) error {
	expected = strings.TrimSpace(expected)
	got := strings.TrimSpace(list.WorkspaceID)
	if got == "" {
		return fmt.Errorf("response omitted workspace id (expected %q)", expected)
	}
	if got != expected {
		return fmt.Errorf("response workspace %q does not match %q", got, expected)
	}
	return nil
}

func materializeBase(targets []baseTarget, createdIDs map[string]string, remote TodoList) ([]cmuxmap.TodoItemBase, error) {
	remoteByID, err := validateRemote(remote)
	if err != nil {
		return nil, err
	}
	items := make([]cmuxmap.TodoItemBase, 0, len(targets))
	for _, target := range targets {
		itemID := target.itemID
		if itemID == "" {
			itemID = createdIDs[target.taskID]
		}
		item, exists := remoteByID[itemID]
		if itemID == "" || !exists {
			return nil, fmt.Errorf("materialize cmux Todo base: task %s has no verified item", target.taskID)
		}
		if item.Text != target.text || item.State != target.state {
			return nil, fmt.Errorf("materialize cmux Todo base: task %s projection does not match verified item", target.taskID)
		}
		items = append(items, cmuxmap.TodoItemBase{
			KRATaskID:  target.taskID,
			CMUXItemID: itemID,
			KRAStatus:  string(target.status),
			CMUXState:  string(target.state),
			Text:       target.text,
		})
	}
	return items, nil
}

func applyStatusChanges(tasks []Task, changes []StatusChange) []Task {
	out := append([]Task(nil), tasks...)
	byID := make(map[string]TaskStatus, len(changes))
	for _, change := range changes {
		byID[change.TaskID] = change.To
	}
	for i := range out {
		if status, ok := byID[out[i].ID]; ok {
			out[i].Status = status
		}
	}
	return out
}

func equalTasks(expected []Task, actual []Task) bool {
	if len(expected) != len(actual) {
		return false
	}
	for i := range expected {
		got := actual[i]
		got.ID = strings.TrimSpace(got.ID)
		got.Title = strings.TrimSpace(got.Title)
		if expected[i] != got {
			return false
		}
	}
	return true
}

func cloneTodoList(in TodoList) TodoList {
	return TodoList{WorkspaceID: in.WorkspaceID, Items: append([]TodoItem(nil), in.Items...)}
}

func normalizedTodoItem(item TodoItem) TodoItem {
	item.ID = strings.TrimSpace(item.ID)
	item.Text = strings.TrimSpace(item.Text)
	item.Origin = strings.TrimSpace(item.Origin)
	return item
}

func equalTodoLists(a TodoList, b TodoList) bool {
	if len(a.Items) != len(b.Items) {
		return false
	}
	left := append([]TodoItem(nil), a.Items...)
	right := append([]TodoItem(nil), b.Items...)
	for i := range left {
		left[i] = normalizedTodoItem(left[i])
	}
	for i := range right {
		right[i] = normalizedTodoItem(right[i])
	}
	sort.Slice(left, func(i, j int) bool { return left[i].ID < left[j].ID })
	sort.Slice(right, func(i, j int) bool { return right[i].ID < right[j].ID })
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func findTodo(items []TodoItem, id string) int {
	for i := range items {
		if strings.TrimSpace(items[i].ID) == id {
			return i
		}
	}
	return -1
}

func (k mutationKind) String() string {
	switch k {
	case mutationAdd:
		return "add"
	case mutationEdit:
		return "edit"
	case mutationState:
		return "set_state"
	default:
		return fmt.Sprintf("mutation(%d)", k)
	}
}
