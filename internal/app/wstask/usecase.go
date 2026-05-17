package wstask

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tasuku43/kra/internal/core/cmuxstyle"
)

type Status string

const (
	StatusTodo    Status = "todo"
	StatusDoing   Status = "doing"
	StatusBlocked Status = "blocked"
	StatusDone    Status = "done"
)

type TaskState string

const (
	TaskStateEmpty   TaskState = "empty"
	TaskStateTodo    TaskState = "todo"
	TaskStateDoing   TaskState = "doing"
	TaskStateBlocked TaskState = "blocked"
	TaskStateDone    TaskState = "done"
)

type SummaryKind string

const (
	SummaryEmpty   SummaryKind = "empty"
	SummaryInvalid SummaryKind = "invalid"
	SummaryCounts  SummaryKind = "counts"
)

type Item struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      Status `json:"status"`
	Description string `json:"description"`
}

type Counts struct {
	Total   int `json:"total"`
	Doing   int `json:"doing"`
	Blocked int `json:"blocked"`
	Todo    int `json:"todo"`
	Done    int `json:"done"`
}

type Overview struct {
	Summary   SummaryKind `json:"summary"`
	TaskState TaskState   `json:"task_state,omitempty"`
	Counts    Counts      `json:"counts,omitempty"`
	Items     []Item      `json:"items,omitempty"`
	Warning   string      `json:"warning,omitempty"`
}

type OverviewResult struct {
	Path     string
	Overview Overview
}

type ListResult struct {
	Path     string
	Overview Overview
}

type ViewGroup struct {
	Status Status
	Title  string
	Items  []Item
}

type ViewModel struct {
	WorkspaceID  string
	Path         string
	CurrentState string
	Next         string
	Items        []Item
	Workspaces   []ViewWorkspace
	Groups       []ViewGroup
	Empty        bool
}

type ViewWorkspace struct {
	ID           string
	Title        string
	CurrentState string
	Next         string
	Items        []Item
}

type AddResult struct {
	Path string
	Task Item
}

type Change struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	PreviousStatus Status `json:"previous_status"`
	Status         Status `json:"status"`
	Changed        bool   `json:"changed"`
}

type TransitionResult struct {
	Path string
	Task Change
}

type SyncState string

const (
	SyncStateApplied SyncState = "applied"
	SyncStateSkipped SyncState = "skipped"
)

type SyncStatusEntry struct {
	Key   string
	Value string
	Icon  string
	Color string
}

type SyncResult struct {
	State       SyncState `json:"state"`
	Targets     int       `json:"targets"`
	SetCount    int       `json:"set"`
	ClearCount  int       `json:"cleared"`
	WarningText string    `json:"warning,omitempty"`
}

type DocumentSnapshot struct {
	Path    string
	Exists  bool
	Content string
}

type Port interface {
	Load(root string, workspaceID string, scope string) (DocumentSnapshot, error)
	Save(snapshot DocumentSnapshot, content string) error
}

type SyncPort interface {
	ListCMUXWorkspaceIDs(root string, workspaceID string) ([]string, error)
	ListStatuses(ctx context.Context, cmuxWorkspaceID string) ([]SyncStatusEntry, error)
	SetStatus(ctx context.Context, cmuxWorkspaceID string, entry SyncStatusEntry) error
	ClearStatus(ctx context.Context, cmuxWorkspaceID string, key string) error
}

type Service struct {
	port     Port
	syncPort SyncPort
}

var (
	ErrWorkspaceNotFound = errors.New("workspace not found")
	ErrWorkspaceArchived = errors.New("workspace is archived")
	ErrTaskNotFound      = errors.New("task not found")
	ErrConflict          = errors.New("workspace task conflict")
)

var (
	taskHeadingPattern = regexp.MustCompile(`^### (TASK-(\d+))\s+(.+?)\s*$`)
	taskStatusPattern  = regexp.MustCompile(`^status:\s*(todo|doing|blocked|done)\s*$`)
	taskIDPattern      = regexp.MustCompile(`^TASK-(\d+)$`)
)

func NewService(port Port, syncPorts ...SyncPort) *Service {
	var syncPort SyncPort
	if len(syncPorts) > 0 {
		syncPort = syncPorts[0]
	}
	return &Service{port: port, syncPort: syncPort}
}

func (s *Service) Overview(root string, workspaceID string, scope string) (OverviewResult, error) {
	snapshot, err := s.port.Load(root, workspaceID, scope)
	if err != nil {
		return OverviewResult{}, err
	}
	doc, err := parseDocument(snapshot)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return OverviewResult{
				Path: snapshot.Path,
				Overview: Overview{
					Summary: SummaryInvalid,
					Warning: err.Error(),
				},
			}, nil
		}
		return OverviewResult{}, err
	}
	return OverviewResult{
		Path:     snapshot.Path,
		Overview: buildOverview(doc.items(), ""),
	}, nil
}

func (s *Service) List(root string, workspaceID string, scope string) (ListResult, error) {
	doc, snapshot, err := s.loadDocument(root, workspaceID, scope)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{
		Path:     snapshot.Path,
		Overview: buildOverview(doc.items(), ""),
	}, nil
}

func (s *Service) View(root string, workspaceID string, scope string) (ViewModel, error) {
	doc, snapshot, err := s.loadDocument(root, workspaceID, scope)
	if err != nil {
		return ViewModel{}, err
	}
	overview := buildOverview(doc.items(), "")
	return ViewModel{
		WorkspaceID:  workspaceID,
		Path:         snapshot.Path,
		CurrentState: doc.currentState(),
		Next:         doc.next(),
		Items:        append([]Item{}, overview.Items...),
		Groups: []ViewGroup{
			{Status: StatusDoing, Title: "Doing", Items: overview.ItemsByStatus(StatusDoing)},
			{Status: StatusBlocked, Title: "Blocked", Items: overview.ItemsByStatus(StatusBlocked)},
			{Status: StatusTodo, Title: "Todo", Items: overview.ItemsByStatus(StatusTodo)},
			{Status: StatusDone, Title: "Done", Items: overview.ItemsByStatus(StatusDone)},
		},
		Empty: overview.Counts.Total == 0,
	}, nil
}

func (s *Service) Add(root string, workspaceID string, title string, description string) (AddResult, error) {
	doc, snapshot, err := s.loadDocument(root, workspaceID, "active")
	if err != nil {
		return AddResult{}, err
	}
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		return AddResult{}, fmt.Errorf("title is required")
	}
	item := Item{
		ID:          nextTaskID(doc.items()),
		Title:       trimmedTitle,
		Status:      StatusTodo,
		Description: normalizeDescription(description),
	}
	doc.ensureTasksSection()
	doc.appendTask(item)
	if err := s.port.Save(snapshot, doc.render()); err != nil {
		return AddResult{}, err
	}
	return AddResult{Path: snapshot.Path, Task: item}, nil
}

func ParseStatus(raw string) (Status, error) {
	switch Status(strings.TrimSpace(raw)) {
	case StatusTodo:
		return StatusTodo, nil
	case StatusDoing:
		return StatusDoing, nil
	case StatusBlocked:
		return StatusBlocked, nil
	case StatusDone:
		return StatusDone, nil
	default:
		return "", fmt.Errorf("unsupported task status: %q", strings.TrimSpace(raw))
	}
}

func AllowedTransitions(current Status) []Status {
	switch current {
	case StatusTodo:
		return []Status{StatusDoing, StatusBlocked, StatusDone}
	case StatusDoing:
		return []Status{StatusTodo, StatusBlocked, StatusDone}
	case StatusBlocked:
		return []Status{StatusTodo, StatusDoing, StatusDone}
	case StatusDone:
		return []Status{StatusTodo}
	default:
		return nil
	}
}

func (s *Service) Status(root string, workspaceID string, taskID string, next Status) (TransitionResult, error) {
	return s.transition(root, workspaceID, taskID, next)
}

func (s *Service) Start(root string, workspaceID string, taskID string) (TransitionResult, error) {
	return s.Status(root, workspaceID, taskID, StatusDoing)
}

func (s *Service) Block(root string, workspaceID string, taskID string) (TransitionResult, error) {
	return s.Status(root, workspaceID, taskID, StatusBlocked)
}

func (s *Service) Done(root string, workspaceID string, taskID string) (TransitionResult, error) {
	return s.Status(root, workspaceID, taskID, StatusDone)
}

func (s *Service) Sync(ctx context.Context, root string, workspaceID string) (SyncResult, error) {
	if s.syncPort == nil {
		return SyncResult{
			State:       SyncStateSkipped,
			WarningText: "cmux task sync is not configured",
		}, nil
	}

	doc, _, err := s.loadDocument(root, workspaceID, "active")
	if err != nil {
		return SyncResult{}, err
	}
	targets, err := s.syncPort.ListCMUXWorkspaceIDs(root, workspaceID)
	if err != nil {
		return SyncResult{}, err
	}
	if len(targets) == 0 {
		return SyncResult{
			State:       SyncStateSkipped,
			WarningText: "no mapped cmux workspace",
		}, nil
	}

	desired := desiredSyncEntries(doc.items())
	result := SyncResult{
		State:   SyncStateApplied,
		Targets: len(targets),
	}
	for _, target := range targets {
		existing, err := s.syncPort.ListStatuses(ctx, target)
		if err != nil {
			return SyncResult{}, err
		}
		existingTaskKeys := make([]string, 0, len(existing))
		for _, entry := range existing {
			if strings.HasPrefix(strings.TrimSpace(entry.Key), "task:") {
				existingTaskKeys = append(existingTaskKeys, strings.TrimSpace(entry.Key))
			}
		}
		sort.Strings(existingTaskKeys)
		for _, key := range existingTaskKeys {
			if err := s.syncPort.ClearStatus(ctx, target, key); err != nil {
				return SyncResult{}, err
			}
			result.ClearCount++
		}
		// cmux inserts newly created status pills at the head, so replay in reverse
		// to preserve the declaration order in the rendered sidebar.
		for i := len(desired) - 1; i >= 0; i-- {
			if err := s.syncPort.SetStatus(ctx, target, desired[i]); err != nil {
				return SyncResult{}, err
			}
			result.SetCount++
		}
	}
	return result, nil
}

func (s *Service) transition(root string, workspaceID string, taskID string, next Status) (TransitionResult, error) {
	doc, snapshot, err := s.loadDocument(root, workspaceID, "active")
	if err != nil {
		return TransitionResult{}, err
	}
	targetID := strings.TrimSpace(taskID)
	if targetID == "" {
		return TransitionResult{}, fmt.Errorf("%w: %s", ErrTaskNotFound, targetID)
	}
	task, ok := doc.findTask(targetID)
	if !ok {
		return TransitionResult{}, fmt.Errorf("%w: %s", ErrTaskNotFound, targetID)
	}
	previous := task.Status
	changed, err := applyTransition(task, next)
	if err != nil {
		return TransitionResult{}, err
	}
	if changed {
		if err := s.port.Save(snapshot, doc.render()); err != nil {
			return TransitionResult{}, err
		}
	}
	return TransitionResult{
		Path: snapshot.Path,
		Task: Change{
			ID:             task.ID,
			Title:          task.Title,
			PreviousStatus: previous,
			Status:         task.Status,
			Changed:        changed,
		},
	}, nil
}

func applyTransition(task *Item, next Status) (bool, error) {
	if task.Status == next {
		return false, nil
	}
	for _, allowed := range AllowedTransitions(task.Status) {
		if allowed == next {
			task.Status = next
			return true, nil
		}
	}
	return false, newConflict("cannot transition %s from %s to %s", task.ID, task.Status, next)
}

func (s *Service) loadDocument(root string, workspaceID string, scope string) (*parsedDocument, DocumentSnapshot, error) {
	snapshot, err := s.port.Load(root, workspaceID, scope)
	if err != nil {
		return nil, DocumentSnapshot{}, err
	}
	doc, err := parseDocument(snapshot)
	if err != nil {
		return nil, DocumentSnapshot{}, err
	}
	return doc, snapshot, nil
}

func (o Overview) SummaryText() string {
	switch o.Summary {
	case "", SummaryEmpty:
		return "tasks: empty"
	case SummaryInvalid:
		return "tasks: invalid"
	default:
		return fmt.Sprintf(
			"tasks: %d total (doing %d, blocked %d, todo %d, done %d)",
			o.Counts.Total,
			o.Counts.Doing,
			o.Counts.Blocked,
			o.Counts.Todo,
			o.Counts.Done,
		)
	}
}

func (o Overview) ItemsByStatus(status Status) []Item {
	out := make([]Item, 0, len(o.Items))
	for _, item := range o.Items {
		if item.Status == status {
			out = append(out, item)
		}
	}
	return out
}

func buildOverview(items []Item, warning string) Overview {
	if len(items) == 0 {
		return Overview{
			Summary:   SummaryEmpty,
			TaskState: TaskStateEmpty,
			Counts:    Counts{},
			Items:     nil,
			Warning:   warning,
		}
	}

	counts := Counts{Total: len(items)}
	hasDoing := false
	hasDone := false
	hasUnfinished := false
	allUnfinishedBlocked := true
	for _, item := range items {
		switch item.Status {
		case StatusDoing:
			counts.Doing++
			hasDoing = true
			hasUnfinished = true
			allUnfinishedBlocked = false
		case StatusBlocked:
			counts.Blocked++
			hasUnfinished = true
		case StatusDone:
			counts.Done++
			hasDone = true
		default:
			counts.Todo++
			hasUnfinished = true
			allUnfinishedBlocked = false
		}
	}

	taskState := TaskStateTodo
	switch {
	case counts.Total == 0:
		taskState = TaskStateEmpty
	case counts.Done == counts.Total:
		taskState = TaskStateDone
	case allUnfinishedBlocked:
		taskState = TaskStateBlocked
	case hasDoing || (hasDone && hasUnfinished):
		taskState = TaskStateDoing
	default:
		taskState = TaskStateTodo
	}

	return Overview{
		Summary:   SummaryCounts,
		TaskState: taskState,
		Counts:    counts,
		Items:     cloneItems(items),
		Warning:   warning,
	}
}

type parsedDocument struct {
	path              string
	exists            bool
	before            []string
	currentStateLines []string
	hasCurrentState   bool
	nextLines         []string
	hasNext           bool
	segments          []sectionSegment
	after             []string
	hasTasksSection   bool
}

type sectionSegment struct {
	isTask bool
	task   Item
	raw    []string
}

func parseDocument(snapshot DocumentSnapshot) (*parsedDocument, error) {
	lines := splitNormalizedLines(snapshot.Content)
	doc := &parsedDocument{
		path:   snapshot.Path,
		exists: snapshot.Exists,
	}
	currentStateStart := -1
	nextStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "## Current State" {
			currentStateStart = i
		}
		if strings.TrimSpace(line) == "## Next" {
			nextStart = i
		}
	}
	sectionStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "## Tasks" {
			sectionStart = i
			break
		}
	}
	if sectionStart < 0 {
		doc.before = cloneLines(lines)
		if currentStateStart >= 0 {
			currentStateEnd := len(lines)
			for i := currentStateStart + 1; i < len(lines); i++ {
				if isLevel2Heading(lines[i]) {
					currentStateEnd = i
					break
				}
			}
			doc.before = cloneLines(lines[:currentStateStart])
			doc.currentStateLines = trimBlankLines(lines[currentStateStart+1 : currentStateEnd])
			doc.after = cloneLines(lines[currentStateEnd:])
			doc.hasCurrentState = true
		}
		if nextStart >= 0 {
			doc.consumeNext(lines, nextStart, len(lines))
		}
		return doc, nil
	}
	doc.before = cloneLines(lines[:sectionStart])
	if currentStateStart >= 0 && currentStateStart < sectionStart {
		currentStateEnd := sectionEndInRange(lines, currentStateStart, sectionStart)
		doc.currentStateLines = trimBlankLines(lines[currentStateStart+1 : currentStateEnd])
		doc.hasCurrentState = true
	}
	if nextStart >= 0 && nextStart < sectionStart {
		doc.consumeNext(lines, nextStart, sectionStart)
	}
	for _, r := range managedSectionRangesBeforeTasks(lines, sectionStart, currentStateStart, nextStart) {
		doc.before = append(doc.before[:r.start], doc.before[r.end:]...)
	}
	sectionEnd := len(lines)
	for i := sectionStart + 1; i < len(lines); i++ {
		if isLevel2Heading(lines[i]) {
			sectionEnd = i
			break
		}
	}
	segments, err := parseSectionBody(lines[sectionStart+1 : sectionEnd])
	if err != nil {
		return nil, err
	}
	doc.segments = segments
	doc.after = cloneLines(lines[sectionEnd:])
	doc.hasTasksSection = true
	return doc, nil
}

func parseSectionBody(lines []string) ([]sectionSegment, error) {
	segments := make([]sectionSegment, 0, 8)
	seenIDs := map[string]struct{}{}
	freeformStart := 0
	i := 0

	flushFreeform := func(end int) {
		if end <= freeformStart {
			return
		}
		raw := cloneLines(lines[freeformStart:end])
		if len(raw) == 0 {
			return
		}
		segments = append(segments, sectionSegment{raw: raw})
	}

	for i < len(lines) {
		if !strings.HasPrefix(lines[i], "### TASK-") {
			i++
			continue
		}
		flushFreeform(i)

		next := i + 1
		for next < len(lines) && !strings.HasPrefix(lines[next], "### TASK-") {
			next++
		}
		end := next
		for end > i && strings.TrimSpace(lines[end-1]) == "" {
			end--
		}
		item, err := parseTaskBlock(lines[i:end])
		if err != nil {
			return nil, err
		}
		if _, ok := seenIDs[item.ID]; ok {
			return nil, newConflict("duplicate task id: %s", item.ID)
		}
		seenIDs[item.ID] = struct{}{}
		segments = append(segments, sectionSegment{isTask: true, task: item})
		freeformStart = end
		i = next
	}

	flushFreeform(len(lines))
	return segments, nil
}

type lineRange struct {
	start int
	end   int
}

func managedSectionRangesBeforeTasks(lines []string, limit int, starts ...int) []lineRange {
	ranges := make([]lineRange, 0, len(starts))
	for _, start := range starts {
		if start < 0 || start >= limit {
			continue
		}
		ranges = append(ranges, lineRange{start: start, end: sectionEndInRange(lines, start, limit)})
	}
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].start > ranges[j].start
	})
	return ranges
}

func sectionEndInRange(lines []string, start int, limit int) int {
	end := limit
	for i := start + 1; i < limit; i++ {
		if isLevel2Heading(lines[i]) {
			end = i
			break
		}
	}
	return end
}

func parseTaskBlock(lines []string) (Item, error) {
	if len(lines) == 0 {
		return Item{}, newConflict("empty task block")
	}
	matches := taskHeadingPattern.FindStringSubmatch(strings.TrimSpace(lines[0]))
	if len(matches) != 4 {
		return Item{}, newConflict("invalid task heading: %s", strings.TrimSpace(lines[0]))
	}
	title := strings.TrimSpace(matches[3])
	if title == "" {
		return Item{}, newConflict("invalid task heading: %s", strings.TrimSpace(lines[0]))
	}
	if len(lines) < 2 {
		return Item{}, newConflict("missing status for %s", matches[1])
	}
	statusMatches := taskStatusPattern.FindStringSubmatch(strings.TrimSpace(lines[1]))
	if len(statusMatches) != 2 {
		return Item{}, newConflict("invalid status for %s", matches[1])
	}
	description := ""
	if len(lines) > 2 {
		description = normalizeDescription(strings.Join(lines[2:], "\n"))
	}
	return Item{
		ID:          matches[1],
		Title:       title,
		Status:      Status(statusMatches[1]),
		Description: description,
	}, nil
}

func trimBlankLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return cloneLines(lines[start:end])
}

func (d *parsedDocument) currentState() string {
	return strings.TrimSpace(strings.Join(d.currentStateLines, "\n"))
}

func (d *parsedDocument) next() string {
	return strings.TrimSpace(strings.Join(d.nextLines, "\n"))
}

func (d *parsedDocument) consumeNext(lines []string, nextStart int, limit int) {
	nextEnd := limit
	for i := nextStart + 1; i < limit; i++ {
		if isLevel2Heading(lines[i]) {
			nextEnd = i
			break
		}
	}
	d.nextLines = trimBlankLines(lines[nextStart+1 : nextEnd])
	d.hasNext = true
}

func (d *parsedDocument) ensureTasksSection() {
	if d.hasTasksSection {
		return
	}
	d.hasTasksSection = true
	d.segments = nil
	d.after = nil
	if !d.hasCurrentState {
		d.hasCurrentState = true
		d.currentStateLines = []string{"This workspace has not recorded current state yet."}
	}
	if !d.hasNext {
		d.hasNext = true
		d.nextLines = []string{"Record the next concrete step here before handing off or stopping."}
	}
	if len(d.before) == 0 {
		d.before = []string{"# Workspace"}
	}
	if len(d.before) > 0 && strings.TrimSpace(d.before[len(d.before)-1]) != "" {
		d.before = append(d.before, "")
	}
}

func (d *parsedDocument) appendTask(item Item) {
	if len(d.segments) > 0 {
		last := &d.segments[len(d.segments)-1]
		switch {
		case last.isTask:
			d.segments = append(d.segments, sectionSegment{raw: []string{""}})
		case len(last.raw) > 0 && strings.TrimSpace(last.raw[len(last.raw)-1]) != "":
			last.raw = append(last.raw, "")
		}
	}
	d.segments = append(d.segments, sectionSegment{isTask: true, task: item})
}

func (d *parsedDocument) findTask(taskID string) (*Item, bool) {
	for i := range d.segments {
		if !d.segments[i].isTask {
			continue
		}
		if d.segments[i].task.ID == taskID {
			return &d.segments[i].task, true
		}
	}
	return nil, false
}

func (d *parsedDocument) items() []Item {
	items := make([]Item, 0, len(d.segments))
	for _, segment := range d.segments {
		if segment.isTask {
			items = append(items, segment.task)
		}
	}
	return items
}

func (d *parsedDocument) render() string {
	lines := make([]string, 0, len(d.before)+len(d.after)+8)
	lines = append(lines, d.before...)
	if d.hasCurrentState {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "## Current State", "")
		lines = append(lines, d.currentStateLines...)
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
	}
	if d.hasNext {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "## Next", "")
		lines = append(lines, d.nextLines...)
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
	}
	if d.hasTasksSection {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "## Tasks")
		body := renderSectionSegments(d.segments)
		if len(body) > 0 && strings.TrimSpace(body[0]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, body...)
		lines = append(lines, d.after...)
	}
	out := strings.Join(lines, "\n")
	if strings.TrimSpace(out) == "" && !d.hasTasksSection {
		return ""
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

func renderSectionSegments(segments []sectionSegment) []string {
	lines := make([]string, 0, len(segments)*4)
	prevTask := false
	for _, segment := range segments {
		if segment.isTask {
			if prevTask && len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
				lines = append(lines, "")
			}
			lines = append(lines, renderTaskLines(segment.task)...)
			prevTask = true
			continue
		}
		lines = append(lines, segment.raw...)
		prevTask = false
	}
	return lines
}

func renderTaskLines(item Item) []string {
	lines := []string{
		fmt.Sprintf("### %s %s", item.ID, item.Title),
		fmt.Sprintf("status: %s", item.Status),
	}
	if description := normalizeDescription(item.Description); description != "" {
		lines = append(lines, "")
		lines = append(lines, strings.Split(description, "\n")...)
	}
	return lines
}

func nextTaskID(items []Item) string {
	maxValue := 0
	width := 3
	for _, item := range items {
		matches := taskIDPattern.FindStringSubmatch(item.ID)
		if len(matches) != 2 {
			continue
		}
		value, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		if value > maxValue {
			maxValue = value
		}
		if len(matches[1]) > width {
			width = len(matches[1])
		}
	}
	next := maxValue + 1
	if digits := len(strconv.Itoa(next)); digits > width {
		width = digits
	}
	return fmt.Sprintf("TASK-%0*d", width, next)
}

func desiredSyncEntries(items []Item) []SyncStatusEntry {
	out := make([]SyncStatusEntry, 0, len(items))
	for _, item := range items {
		entry, ok := syncEntryForItem(item)
		if !ok {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func syncEntryForItem(item Item) (SyncStatusEntry, bool) {
	switch item.Status {
	case StatusTodo:
		return SyncStatusEntry{
			Key:   "task:" + item.ID,
			Value: "○ " + item.ID + " " + item.Title,
			Icon:  cmuxstyle.TaskStatusIcon,
			Color: cmuxstyle.TaskStatusTodoColor,
		}, true
	case StatusDoing:
		return SyncStatusEntry{
			Key:   "task:" + item.ID,
			Value: "● " + item.ID + " " + item.Title,
			Icon:  cmuxstyle.TaskStatusIcon,
			Color: cmuxstyle.TaskStatusDoingColor,
		}, true
	case StatusBlocked:
		return SyncStatusEntry{
			Key:   "task:" + item.ID,
			Value: "▲ " + item.ID + " " + item.Title,
			Icon:  cmuxstyle.TaskStatusIcon,
			Color: cmuxstyle.TaskStatusBlockedColor,
		}, true
	case StatusDone:
		return SyncStatusEntry{
			Key:   "task:" + item.ID,
			Value: "✔ " + item.ID + " " + item.Title,
			Icon:  cmuxstyle.TaskStatusIcon,
			Color: cmuxstyle.TaskStatusDoneColor,
		}, true
	default:
		return SyncStatusEntry{}, false
	}
}

func splitNormalizedLines(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.TrimSuffix(normalized, "\n")
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
}

func isLevel2Heading(line string) bool {
	return strings.HasPrefix(line, "## ") && !strings.HasPrefix(line, "###")
}

func normalizeDescription(description string) string {
	trimmed := strings.ReplaceAll(description, "\r\n", "\n")
	trimmed = strings.Trim(trimmed, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return ""
	}
	return trimmed
}

func cloneItems(items []Item) []Item {
	if len(items) == 0 {
		return nil
	}
	out := make([]Item, len(items))
	copy(out, items)
	return out
}

func cloneLines(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, len(lines))
	copy(out, lines)
	return out
}

func newConflict(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, args...))
}
