package cmux

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

type Client interface {
	Capabilities(ctx context.Context) (cmuxctl.Capabilities, error)
	CreateWorkspaceWithCommand(ctx context.Context, command string) (string, error)
	CloseWorkspace(ctx context.Context, workspace string) error
	RenameWorkspace(ctx context.Context, workspace string, title string) error
	SelectWorkspace(ctx context.Context, workspace string) error
	Notify(ctx context.Context, opts cmuxctl.NotifyOptions) error
	SendText(ctx context.Context, workspace string, surface string, text string) error
	ListPanes(ctx context.Context, workspace string) ([]cmuxctl.Pane, error)
	ListWorkspaces(ctx context.Context) ([]cmuxctl.Workspace, error)
	Identify(ctx context.Context, workspace string, surface string) (map[string]any, error)
}

type MappingStore interface {
	AcquireLock(ctx context.Context) (func(), error)
	Load() (cmuxmap.File, error)
	Save(cmuxmap.File) error
}

type NewClientFunc func() Client
type NewStoreFunc func(root string) MappingStore

type Service struct {
	NewClient NewClientFunc
	NewStore  NewStoreFunc
	Now       func() time.Time
	Debugf    func(string, ...any)
}

func NewService(newClient NewClientFunc, newStore NewStoreFunc) *Service {
	return &Service{
		NewClient: newClient,
		NewStore:  newStore,
		Now:       time.Now,
	}
}

type OpenTarget struct {
	WorkspaceID   string
	WorkspacePath string
	Title         string
}

type OpenResultItem struct {
	WorkspaceID     string
	WorkspacePath   string
	CMUXWorkspaceID string
	Ordinal         int
	Title           string
	ReusedExisting  bool
	Command         string
	CommandExecuted bool

	groupID              string
	groupMembershipAdded bool
}

type OpenFailure struct {
	WorkspaceID string
	Code        string
	Message     string
}

type OpenResult struct {
	Results      []OpenResultItem
	Failures     []OpenFailure
	GroupWarning string
}

type openOptions struct {
	SelectWorkspace bool
	Command         string
	Group           *cmuxmap.GroupMapping
	GroupAnchorNew  bool
}

func (s *Service) Open(ctx context.Context, root string, targets []OpenTarget, concurrency int, multi bool, command string) (OpenResult, string, string) {
	if s.NewClient == nil || s.NewStore == nil {
		return OpenResult{}, "internal_error", "cmux service is not initialized"
	}
	client := s.NewClient()
	caps, err := client.Capabilities(ctx)
	if err != nil {
		return OpenResult{}, "cmux_capability_missing", fmt.Sprintf("read cmux capabilities: %v", err)
	}
	for _, method := range []string{"workspace.create", "workspace.rename"} {
		if _, ok := caps.Methods[method]; !ok {
			return OpenResult{}, "cmux_capability_missing", fmt.Sprintf("cmux capability missing: %s", method)
		}
	}

	store := s.NewStore(root)
	releaseLock, err := store.AcquireLock(ctx)
	if err != nil {
		return OpenResult{}, "state_write_failed", fmt.Sprintf("acquire cmux mapping lock: %v", err)
	}
	locked := true
	defer func() {
		if locked {
			releaseLock()
		}
	}()
	mapping, err := store.Load()
	if err != nil {
		return OpenResult{}, "state_write_failed", fmt.Sprintf("load cmux mapping: %v", err)
	}
	groupTxn, groupCode, groupMessage := s.ensureRootGroup(ctx, root, client, caps, &mapping)
	if groupCode != "" {
		return OpenResult{}, groupCode, groupMessage
	}

	result := OpenResult{
		Results:      make([]OpenResultItem, 0, len(targets)),
		Failures:     make([]OpenFailure, 0),
		GroupWarning: groupTxn.Warning,
	}
	if multi && concurrency > 1 {
		result = s.openConcurrent(ctx, targets, concurrency, &mapping, command, groupTxn)
	} else {
		result = s.openSequential(ctx, client, targets, &mapping, command, groupTxn)
	}
	result.GroupWarning = groupTxn.Warning
	if len(result.Results) > 0 || groupTxn.Changed {
		if err := store.Save(mapping); err != nil {
			msg := fmt.Sprintf("save cmux mapping: %v", err)
			msg = appendOpenFailures(msg, result.Failures)
			msg = s.compensateOpenTransaction(ctx, client, result.Results, groupTxn, msg)
			return OpenResult{}, "state_write_failed", msg
		}
	}
	releaseLock()
	locked = false
	if len(result.Results) > 0 {
		for _, item := range result.Results {
			s.notifyWorkspaceOpened(ctx, client, item)
		}
	}
	return result, "", ""
}

func (s *Service) openSequential(ctx context.Context, client Client, targets []OpenTarget, mapping *cmuxmap.File, command string, groupTxn groupTransaction) OpenResult {
	res := OpenResult{
		Results:  make([]OpenResultItem, 0, len(targets)),
		Failures: make([]OpenFailure, 0),
	}
	var mapMu sync.Mutex
	for _, target := range targets {
		item, code, msg := s.openOne(ctx, client, target, mapping, &mapMu, openOptions{Command: command, Group: groupMappingForOpen(groupTxn), GroupAnchorNew: groupTxn.CreatedAnchorID != ""})
		if code != "" {
			res.Failures = append(res.Failures, OpenFailure{WorkspaceID: target.WorkspaceID, Code: code, Message: msg})
			return res
		}
		res.Results = append(res.Results, item)
	}
	return res
}

func (s *Service) openConcurrent(ctx context.Context, targets []OpenTarget, concurrency int, mapping *cmuxmap.File, command string, groupTxn groupTransaction) OpenResult {
	type task struct {
		index  int
		target OpenTarget
	}
	type outItem struct {
		index int
		item  OpenResultItem
		fail  *OpenFailure
	}
	tasks := make([]task, 0, len(targets))
	for i, target := range targets {
		tasks = append(tasks, task{index: i, target: target})
	}
	jobs := make(chan task)
	out := make(chan outItem, len(tasks))
	var wg sync.WaitGroup
	var mapMu sync.Mutex
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := s.NewClient()
			for job := range jobs {
				item, code, msg := s.openOne(ctx, client, job.target, mapping, &mapMu, openOptions{Command: command, Group: groupMappingForOpen(groupTxn), GroupAnchorNew: groupTxn.CreatedAnchorID != ""})
				if code != "" {
					out <- outItem{
						index: job.index,
						fail: &OpenFailure{
							WorkspaceID: job.target.WorkspaceID,
							Code:        code,
							Message:     msg,
						},
					}
					continue
				}
				out <- outItem{index: job.index, item: item}
			}
		}()
	}
	go func() {
		for _, t := range tasks {
			jobs <- t
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()
	collected := make([]outItem, 0, len(tasks))
	for o := range out {
		collected = append(collected, o)
	}
	sort.Slice(collected, func(i, j int) bool { return collected[i].index < collected[j].index })
	result := OpenResult{
		Results:  make([]OpenResultItem, 0, len(tasks)),
		Failures: make([]OpenFailure, 0),
	}
	for _, c := range collected {
		if c.fail != nil {
			result.Failures = append(result.Failures, *c.fail)
			continue
		}
		result.Results = append(result.Results, c.item)
	}
	return result
}

func (s *Service) EnsureWorkspace(ctx context.Context, root string, target OpenTarget, selectWorkspace bool) (OpenResultItem, string, string) {
	if s.NewClient == nil || s.NewStore == nil {
		return OpenResultItem{}, "internal_error", "cmux service is not initialized"
	}
	client := s.NewClient()
	caps, err := client.Capabilities(ctx)
	if err != nil {
		return OpenResultItem{}, "cmux_capability_missing", fmt.Sprintf("read cmux capabilities: %v", err)
	}
	required := []string{"workspace.create", "workspace.rename"}
	if selectWorkspace {
		required = append(required, "workspace.select")
	}
	for _, method := range required {
		if _, ok := caps.Methods[method]; !ok {
			return OpenResultItem{}, "cmux_capability_missing", fmt.Sprintf("cmux capability missing: %s", method)
		}
	}

	store := s.NewStore(root)
	releaseLock, err := store.AcquireLock(ctx)
	if err != nil {
		return OpenResultItem{}, "state_write_failed", fmt.Sprintf("acquire cmux mapping lock: %v", err)
	}
	defer releaseLock()
	mapping, err := store.Load()
	if err != nil {
		return OpenResultItem{}, "state_write_failed", fmt.Sprintf("load cmux mapping: %v", err)
	}
	groupTxn, groupCode, groupMessage := s.ensureRootGroup(ctx, root, client, caps, &mapping)
	if groupCode != "" {
		return OpenResultItem{}, groupCode, groupMessage
	}
	var mapMu sync.Mutex
	item, code, msg := s.openOne(ctx, client, target, &mapping, &mapMu, openOptions{
		SelectWorkspace: selectWorkspace,
		Group:           groupMappingForOpen(groupTxn),
		GroupAnchorNew:  groupTxn.CreatedAnchorID != "",
	})
	if code != "" {
		if groupTxn.Changed {
			if err := store.Save(mapping); err != nil {
				msg = s.compensateOpenTransaction(ctx, client, nil, groupTxn, fmt.Sprintf("%s; save cmux group mapping: %v", msg, err))
			}
		}
		return OpenResultItem{}, code, msg
	}
	if err := store.Save(mapping); err != nil {
		msg := fmt.Sprintf("save cmux mapping: %v", err)
		msg = s.compensateOpenTransaction(ctx, client, []OpenResultItem{item}, groupTxn, msg)
		return OpenResultItem{}, "state_write_failed", msg
	}
	return item, "", ""
}

func (s *Service) openOne(ctx context.Context, client Client, target OpenTarget, mapping *cmuxmap.File, mapMu *sync.Mutex, opts openOptions) (item OpenResultItem, code string, msg string) {
	mapMu.Lock()
	previousMapping, previousMappingExists := mapping.Workspaces[target.WorkspaceID]
	previousMapping = cloneWorkspaceMapping(previousMapping)
	mapMu.Unlock()
	createdWorkspaceID := ""
	defer func() {
		if code == "" {
			return
		}
		restoreWorkspaceMapping(mapping, mapMu, target.WorkspaceID, previousMapping, previousMappingExists)
		if createdWorkspaceID != "" {
			msg = s.compensateCreatedWorkspace(ctx, client, createdWorkspaceID, msg)
		}
	}()

	if opts.Group != nil && target.WorkspaceID == rootMappingWorkspaceID {
		anchorID := strings.TrimSpace(opts.Group.AnchorWorkspaceID)
		if anchorID == "" {
			return OpenResultItem{}, "cmux_group_setup_failed", "cmux root group anchor is empty"
		}
		if groups, ok := client.(workspaceGroupClient); ok {
			if err := groups.FocusWorkspaceGroup(ctx, opts.Group.CMUXGroupID); err != nil {
				return OpenResultItem{}, "cmux_group_focus_failed", fmt.Sprintf("focus cmux root group: %v", err)
			}
		} else if opts.SelectWorkspace {
			if err := client.SelectWorkspace(ctx, anchorID); err != nil {
				return OpenResultItem{}, "cmux_select_failed", fmt.Sprintf("select cmux root workspace: %v", err)
			}
		}
		return OpenResultItem{
			WorkspaceID:     target.WorkspaceID,
			WorkspacePath:   target.WorkspacePath,
			CMUXWorkspaceID: anchorID,
			Ordinal:         1,
			Title:           opts.Group.NameSnapshot,
			ReusedExisting:  !opts.GroupAnchorNew,
			groupID:         opts.Group.CMUXGroupID,
		}, "", ""
	}

	// 1:1 policy: if mapping already exists and runtime workspace is reachable, switch to it.
	mapMu.Lock()
	existingEntries := append([]cmuxmap.Entry{}, mapping.Workspaces[target.WorkspaceID].Entries...)
	mapMu.Unlock()
	if len(existingEntries) > 0 {
		existing, reachable, identifyCode, identifyMessage := firstReachableWorkspaceEntry(ctx, client, existingEntries)
		if identifyCode != "" {
			return OpenResultItem{}, identifyCode, identifyMessage
		}
		if reachable {
			if opts.SelectWorkspace {
				item := s.reuseExistingWorkspace(target, existing, mapping, mapMu)
				selected := false
				if err := client.SelectWorkspace(ctx, existing.CMUXWorkspaceID); err != nil {
					if IsNotFoundError(err) {
						relinked, code, msg := s.tryRelinkWorkspaceByTitle(ctx, client, target, existing, mapping, mapMu, opts)
						if code != "" {
							return OpenResultItem{}, code, msg
						}
						if relinked != nil {
							added, groupCode, groupMessage := s.ensureWorkspaceGroupMembership(ctx, client, opts.Group, relinked.CMUXWorkspaceID)
							if groupCode != "" {
								return OpenResultItem{}, groupCode, groupMessage
							}
							if added {
								relinked.groupMembershipAdded = true
								relinked.groupID = opts.Group.CMUXGroupID
							}
							return *relinked, "", ""
						}
						// runtime entry disappeared between identify and select; recreate below.
						resetWorkspaceCMUXMapping(mapping, mapMu, target.WorkspaceID)
					} else {
						return OpenResultItem{}, "cmux_select_failed", fmt.Sprintf("select cmux workspace: %v", err)
					}
				} else {
					selected = true
				}
				if selected {
					added, groupCode, groupMessage := s.ensureWorkspaceGroupMembership(ctx, client, opts.Group, item.CMUXWorkspaceID)
					if groupCode != "" {
						return OpenResultItem{}, groupCode, groupMessage
					}
					if added {
						item.groupMembershipAdded = true
						item.groupID = opts.Group.CMUXGroupID
					}
					return item, "", ""
				}
			} else {
				item := s.reuseExistingWorkspace(target, existing, mapping, mapMu)
				if code, msg := s.runOpenCommandInExisting(ctx, client, item, opts.Command); code != "" {
					return OpenResultItem{}, code, msg
				}
				added, groupCode, groupMessage := s.ensureWorkspaceGroupMembership(ctx, client, opts.Group, item.CMUXWorkspaceID)
				if groupCode != "" {
					return OpenResultItem{}, groupCode, groupMessage
				}
				if added {
					item.groupMembershipAdded = true
					item.groupID = opts.Group.CMUXGroupID
				}
				item = withOpenCommandResult(item, opts.Command)
				return item, "", ""
			}
		} else {
			relinked, code, msg := s.tryRelinkWorkspaceByTitle(ctx, client, target, existingEntries[0], mapping, mapMu, opts)
			if code != "" {
				return OpenResultItem{}, code, msg
			}
			if relinked != nil {
				added, groupCode, groupMessage := s.ensureWorkspaceGroupMembership(ctx, client, opts.Group, relinked.CMUXWorkspaceID)
				if groupCode != "" {
					return OpenResultItem{}, groupCode, groupMessage
				}
				if added {
					relinked.groupMembershipAdded = true
					relinked.groupID = opts.Group.CMUXGroupID
				}
				return *relinked, "", ""
			}
			// stale mapping entry: clear and recreate with ordinal reset.
			resetWorkspaceCMUXMapping(mapping, mapMu, target.WorkspaceID)
		}
	}

	mapMu.Lock()
	ordinal, err := cmuxmap.AllocateOrdinal(mapping, target.WorkspaceID)
	mapMu.Unlock()
	if err != nil {
		return OpenResultItem{}, "state_write_failed", fmt.Sprintf("allocate cmux ordinal: %v", err)
	}
	cmuxTitle, err := cmuxmap.FormatWorkspaceTitle(target.WorkspaceID, target.Title, ordinal)
	if err != nil {
		return OpenResultItem{}, "cmux_rename_failed", fmt.Sprintf("format cmux workspace title: %v", err)
	}
	cmuxWorkspaceID := ""
	if opts.Group != nil {
		creator, ok := client.(workspaceOptionsClient)
		if !ok {
			return OpenResultItem{}, "cmux_create_failed", "atomic cmux workspace create is unavailable"
		}
		created, err := createWorkspaceWithIdempotencyRetry(ctx, creator, cmuxctl.WorkspaceCreateOptions{
			OperationID:    newOperationID(),
			Title:          cmuxTitle,
			CWD:            target.WorkspacePath,
			Focus:          false,
			WindowID:       opts.Group.CMUXWindowID,
			GroupID:        opts.Group.CMUXGroupID,
			GroupPlacement: "end",
		})
		if err != nil {
			return OpenResultItem{}, "cmux_create_failed", fmt.Sprintf("create grouped cmux workspace: %v", err)
		}
		createdWorkspaceID = chooseWorkspaceCreateHandle(created)
		cmuxWorkspaceID = strings.TrimSpace(created.WorkspaceID)
		if cmuxWorkspaceID == "" {
			return OpenResultItem{}, "cmux_create_failed", "create grouped cmux workspace: response omitted stable workspace id"
		}
		if !workspaceCreateResultMatchesGroup(created, opts.Group.CMUXGroupID) {
			returnedGroup := firstNonEmpty(created.GroupID, created.GroupRef, "<empty>")
			return OpenResultItem{}, "cmux_group_setup_failed", fmt.Sprintf("created cmux workspace joined unexpected group %s (wanted %s)", returnedGroup, opts.Group.CMUXGroupID)
		}
		if created.WindowID != "" && opts.Group.CMUXWindowID != "" && strings.TrimSpace(created.WindowID) != strings.TrimSpace(opts.Group.CMUXWindowID) {
			return OpenResultItem{}, "cmux_group_setup_failed", fmt.Sprintf("created cmux workspace joined unexpected window %s (wanted %s)", created.WindowID, opts.Group.CMUXWindowID)
		}
	} else {
		cmuxWorkspaceID, err = client.CreateWorkspaceWithCommand(ctx, workspaceCDCommand(target.WorkspacePath))
		if err != nil {
			return OpenResultItem{}, "cmux_create_failed", fmt.Sprintf("create cmux workspace: %v", err)
		}
		createdWorkspaceID = cmuxWorkspaceID
		if err := client.RenameWorkspace(ctx, cmuxWorkspaceID, cmuxTitle); err != nil {
			return OpenResultItem{}, "cmux_rename_failed", fmt.Sprintf("rename cmux workspace: %v", err)
		}
	}
	item = OpenResultItem{
		WorkspaceID:     target.WorkspaceID,
		WorkspacePath:   target.WorkspacePath,
		CMUXWorkspaceID: cmuxWorkspaceID,
		Ordinal:         ordinal,
		Title:           cmuxTitle,
		ReusedExisting:  false,
		Command:         strings.TrimSpace(opts.Command),
		CommandExecuted: strings.TrimSpace(opts.Command) != "",
		groupID:         firstGroupID(opts.Group),
	}
	if code, msg := s.runOpenCommand(ctx, client, item, opts.Command); code != "" {
		return OpenResultItem{}, code, msg
	}
	if opts.SelectWorkspace {
		if err := client.SelectWorkspace(ctx, cmuxWorkspaceID); err != nil {
			return OpenResultItem{}, "cmux_select_failed", fmt.Sprintf("select cmux workspace: %v", err)
		}
	}
	now := s.Now().UTC().Format(time.RFC3339)
	mapMu.Lock()
	ws := mapping.Workspaces[target.WorkspaceID]
	ws.Entries = []cmuxmap.Entry{{
		CMUXWorkspaceID: cmuxWorkspaceID,
		Ordinal:         ordinal,
		TitleSnapshot:   cmuxTitle,
		CreatedAt:       now,
		LastUsedAt:      now,
	}}
	mapping.Workspaces[target.WorkspaceID] = ws
	mapMu.Unlock()
	return item, "", ""
}

func cloneWorkspaceMapping(in cmuxmap.WorkspaceMapping) cmuxmap.WorkspaceMapping {
	out := in
	out.Entries = append([]cmuxmap.Entry{}, in.Entries...)
	return out
}

func restoreWorkspaceMapping(mapping *cmuxmap.File, mapMu *sync.Mutex, workspaceID string, previous cmuxmap.WorkspaceMapping, existed bool) {
	mapMu.Lock()
	defer mapMu.Unlock()
	if !existed {
		delete(mapping.Workspaces, workspaceID)
		return
	}
	mapping.Workspaces[workspaceID] = cloneWorkspaceMapping(previous)
}

const cmuxCleanupTimeout = 5 * time.Second

func (s *Service) compensateCreatedWorkspace(ctx context.Context, client Client, workspaceID string, primaryMessage string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return primaryMessage
	}
	base := ctx
	if base == nil {
		base = context.Background()
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(base), cmuxCleanupTimeout)
	defer cancel()
	if err := client.CloseWorkspace(cleanupCtx, workspaceID); err != nil && !IsNotFoundError(err) {
		s.debugf("cmux provisional workspace cleanup failed cmux=%s err=%v", workspaceID, err)
		return fmt.Sprintf("%s; cleanup failed for cmux workspace %s: %v; workspace may remain open; close it manually", primaryMessage, workspaceID, err)
	}
	s.debugf("cmux provisional workspace cleanup ok cmux=%s", workspaceID)
	return primaryMessage
}

func (s *Service) compensateCreatedResults(ctx context.Context, results []OpenResultItem, primaryMessage string) string {
	client := s.NewClient()
	message := primaryMessage
	for _, item := range results {
		if item.ReusedExisting {
			continue
		}
		message = s.compensateCreatedWorkspace(ctx, client, item.CMUXWorkspaceID, message)
	}
	return message
}

func appendOpenFailures(primaryMessage string, failures []OpenFailure) string {
	message := primaryMessage
	for _, failure := range failures {
		message = fmt.Sprintf("%s; target %s failed (%s): %s", message, failure.WorkspaceID, failure.Code, failure.Message)
	}
	return message
}

func firstReachableWorkspaceEntry(ctx context.Context, client Client, entries []cmuxmap.Entry) (cmuxmap.Entry, bool, string, string) {
	for _, entry := range entries {
		payload, err := client.Identify(ctx, entry.CMUXWorkspaceID, "")
		if err != nil {
			if IsNotFoundError(err) {
				continue
			}
			return cmuxmap.Entry{}, false, "cmux_identify_failed", fmt.Sprintf("identify cmux workspace: %v", err)
		}
		switch identifyWorkspaceHandleState(payload, entry.CMUXWorkspaceID) {
		case 1:
			return entry, true, "", ""
		case -1:
			continue
		default:
			return cmuxmap.Entry{}, false, "cmux_identify_failed", fmt.Sprintf("identify cmux workspace: could not verify requested workspace: %s", entry.CMUXWorkspaceID)
		}
	}
	return cmuxmap.Entry{}, false, "", ""
}

func (s *Service) reuseExistingWorkspace(target OpenTarget, existing cmuxmap.Entry, mapping *cmuxmap.File, mapMu *sync.Mutex) OpenResultItem {
	now := s.Now().UTC().Format(time.RFC3339)
	mapMu.Lock()
	ws := mapping.Workspaces[target.WorkspaceID]
	ws.Entries = []cmuxmap.Entry{existing}
	ws.Entries[0].LastUsedAt = now
	mapping.Workspaces[target.WorkspaceID] = ws
	mapMu.Unlock()
	return OpenResultItem{
		WorkspaceID:     target.WorkspaceID,
		WorkspacePath:   target.WorkspacePath,
		CMUXWorkspaceID: existing.CMUXWorkspaceID,
		Ordinal:         existing.Ordinal,
		Title:           existing.TitleSnapshot,
		ReusedExisting:  true,
	}
}

func (s *Service) tryRelinkWorkspaceByTitle(ctx context.Context, client Client, target OpenTarget, existing cmuxmap.Entry, mapping *cmuxmap.File, mapMu *sync.Mutex, opts openOptions) (*OpenResultItem, string, string) {
	expectedTitle, err := cmuxmap.FormatWorkspaceTitle(target.WorkspaceID, target.Title, existing.Ordinal)
	if err != nil {
		return nil, "state_write_failed", fmt.Sprintf("format cmux workspace title: %v", err)
	}
	runtime, err := client.ListWorkspaces(ctx)
	if err != nil {
		return nil, "cmux_list_failed", fmt.Sprintf("list cmux workspaces: %v", err)
	}
	match, ok := findWorkspaceByAnyExactTitle(runtime, expectedTitle, existing.TitleSnapshot)
	if !ok {
		return nil, "", ""
	}
	entry := existing
	entry.CMUXWorkspaceID = chooseCMUXWorkspaceHandle(match)
	entry.TitleSnapshot = strings.TrimSpace(match.Title)
	if entry.TitleSnapshot == "" {
		entry.TitleSnapshot = expectedTitle
	}
	if opts.SelectWorkspace {
		item := s.reuseExistingWorkspace(target, entry, mapping, mapMu)
		if err := client.SelectWorkspace(ctx, entry.CMUXWorkspaceID); err != nil {
			if IsNotFoundError(err) {
				return nil, "", ""
			}
			return nil, "cmux_select_failed", fmt.Sprintf("select cmux workspace: %v", err)
		}
		return &item, "", ""
	}
	item := s.reuseExistingWorkspace(target, entry, mapping, mapMu)
	if code, msg := s.runOpenCommandInExisting(ctx, client, item, opts.Command); code != "" {
		return nil, code, msg
	}
	item = withOpenCommandResult(item, opts.Command)
	return &item, "", ""
}

func withOpenCommandResult(item OpenResultItem, command string) OpenResultItem {
	command = strings.TrimSpace(command)
	item.Command = command
	item.CommandExecuted = command != ""
	return item
}

func (s *Service) runOpenCommandInExisting(ctx context.Context, client Client, item OpenResultItem, command string) (string, string) {
	return s.runOpenCommand(ctx, client, item, command)
}

func (s *Service) runOpenCommand(ctx context.Context, client Client, item OpenResultItem, command string) (string, string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", ""
	}
	surface, code, msg := s.selectedSurfaceIDForWorkspace(ctx, client, item, "command")
	if code != "" {
		return code, msg
	}
	if err := client.SendText(ctx, item.CMUXWorkspaceID, surface, command+"\n"); err != nil {
		return "cmux_send_command_failed", fmt.Sprintf("send open command: %v", err)
	}
	s.debugf("cmux open command sent workspace=%s cmux=%s surface=%s command=%q", item.WorkspaceID, item.CMUXWorkspaceID, surface, command)
	return "", ""
}

func (s *Service) notifyWorkspaceOpened(ctx context.Context, client Client, item OpenResultItem) {
	title := "kra workspace opened"
	subtitle := strings.TrimSpace(item.WorkspaceID)
	body := strings.TrimSpace(item.Title)
	if body == "" {
		body = strings.TrimSpace(item.WorkspacePath)
	}
	surface, _, _ := s.selectedSurfaceIDForWorkspace(ctx, client, item, "notify")
	s.debugf("cmux open notify before-select workspace=%s cmux=%s surface=%s title=%q subtitle=%q", item.WorkspaceID, item.CMUXWorkspaceID, surface, title, subtitle)
	if err := client.Notify(ctx, cmuxctl.NotifyOptions{
		Title:     title,
		Subtitle:  subtitle,
		Body:      body,
		Workspace: item.CMUXWorkspaceID,
		Surface:   surface,
	}); err != nil {
		s.debugf("cmux open notify failed workspace=%s cmux=%s surface=%s err=%v", item.WorkspaceID, item.CMUXWorkspaceID, surface, err)
		return
	}
	s.debugf("cmux open notify ok workspace=%s cmux=%s surface=%s", item.WorkspaceID, item.CMUXWorkspaceID, surface)
}

func (s *Service) selectedSurfaceIDForWorkspace(ctx context.Context, client Client, item OpenResultItem, purpose string) (string, string, string) {
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", "context_canceled", ctx.Err().Error()
			case <-time.After(75 * time.Millisecond):
			}
		}
		panes, err := client.ListPanes(ctx, item.CMUXWorkspaceID)
		if err != nil {
			s.debugf("cmux open %s list panes failed workspace=%s cmux=%s attempt=%d err=%v", purpose, item.WorkspaceID, item.CMUXWorkspaceID, attempt+1, err)
			return "", "cmux_list_panes_failed", fmt.Sprintf("list panes for open %s: %v", purpose, err)
		}
		surface := selectedSurfaceID(panes)
		s.debugf("cmux open %s list panes workspace=%s cmux=%s attempt=%d surface=%s", purpose, item.WorkspaceID, item.CMUXWorkspaceID, attempt+1, surface)
		if surface != "" {
			return surface, "", ""
		}
	}
	return "", "cmux_surface_not_found", fmt.Sprintf("selected surface not found for open %s", purpose)
}

func selectedSurfaceID(panes []cmuxctl.Pane) string {
	for _, pane := range panes {
		if !pane.Focused {
			continue
		}
		if id := strings.TrimSpace(pane.SelectedSurfaceID); id != "" {
			return id
		}
	}
	for _, pane := range panes {
		if id := strings.TrimSpace(pane.SelectedSurfaceID); id != "" {
			return id
		}
	}
	return ""
}

func workspaceCDCommand(workspacePath string) string {
	return fmt.Sprintf("cd %s", shellQuoteCDPath(workspacePath))
}

func (s *Service) debugf(format string, args ...any) {
	if s.Debugf == nil {
		return
	}
	s.Debugf(format, args...)
}

func identifyFocusedSurfaceHandle(payload map[string]any) string {
	focused := nestedAnyMap(payload, "focused")
	if focused == nil {
		return ""
	}
	for _, key := range []string{"surface_ref", "surface_id"} {
		raw, ok := focused[key]
		if !ok {
			continue
		}
		value, _ := raw.(string)
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func findWorkspaceByAnyExactTitle(workspaces []cmuxctl.Workspace, titles ...string) (cmuxctl.Workspace, bool) {
	wanted := make(map[string]struct{}, len(titles))
	for _, title := range titles {
		if title = strings.TrimSpace(title); title != "" {
			wanted[title] = struct{}{}
		}
	}
	var match cmuxctl.Workspace
	found := false
	for _, workspace := range workspaces {
		if _, ok := wanted[strings.TrimSpace(workspace.Title)]; !ok {
			continue
		}
		if found {
			return cmuxctl.Workspace{}, false
		}
		match = workspace
		found = true
	}
	return match, found
}

func chooseCMUXWorkspaceHandle(workspace cmuxctl.Workspace) string {
	if id := strings.TrimSpace(workspace.ID); id != "" {
		return id
	}
	return strings.TrimSpace(workspace.Ref)
}

func shellQuoteSingle(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func shellEscapeForDoubleQuotes(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`")
	return replacer.Replace(s)
}

func shellQuoteCDPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil {
		if path == home {
			return `"$HOME"`
		}
		prefix := home + string(os.PathSeparator)
		if strings.HasPrefix(path, prefix) {
			suffix := strings.TrimPrefix(path, prefix)
			return `"$HOME/` + shellEscapeForDoubleQuotes(suffix) + `"`
		}
	}
	return shellQuoteSingle(path)
}

type ListRow struct {
	WorkspaceID string
	CMUXID      string
	Ordinal     int
	Title       string
	LastUsedAt  string
}

type ListResult struct {
	Rows            []ListRow
	RuntimeChecked  bool
	PrunedCount     int
	RuntimeWarnText string
}

func (s *Service) List(ctx context.Context, root string, workspaceID string) (ListResult, string, string) {
	store := s.NewStore(root)
	releaseLock, err := store.AcquireLock(ctx)
	if err != nil {
		return ListResult{}, "internal_error", fmt.Sprintf("acquire cmux mapping lock: %v", err)
	}
	defer releaseLock()
	mapping, err := store.Load()
	if err != nil {
		return ListResult{}, "internal_error", fmt.Sprintf("load cmux mapping: %v", err)
	}
	result := ListResult{
		Rows: make([]ListRow, 0),
	}
	client := s.NewClient()
	cmuxList, lerr := client.ListWorkspaces(ctx)
	if lerr != nil {
		result.RuntimeWarnText = fmt.Sprintf("list cmux workspaces: %v", lerr)
	} else {
		result.RuntimeChecked = true
		reconciled, _, pruned, recErr := ReconcileMappingWithRuntime(store, mapping, cmuxList, true)
		if recErr != nil {
			return ListResult{}, "internal_error", fmt.Sprintf("save cmux mapping: %v", recErr)
		}
		mapping = reconciled
		result.PrunedCount = pruned
		if len(cmuxList) == 0 {
			probePruned, probeErr := probeAndPruneByID(ctx, store, &mapping, client)
			result.PrunedCount += probePruned
			if probeErr != "" {
				result.RuntimeWarnText = probeErr
			}
		}
	}

	workspaceIDs := make([]string, 0, len(mapping.Workspaces))
	for wsID := range mapping.Workspaces {
		if workspaceID != "" && workspaceID != wsID {
			continue
		}
		workspaceIDs = append(workspaceIDs, wsID)
	}
	sort.Strings(workspaceIDs)
	for _, wsID := range workspaceIDs {
		ws := mapping.Workspaces[wsID]
		for _, e := range ws.Entries {
			result.Rows = append(result.Rows, ListRow{
				WorkspaceID: wsID,
				CMUXID:      e.CMUXWorkspaceID,
				Ordinal:     e.Ordinal,
				Title:       e.TitleSnapshot,
				LastUsedAt:  e.LastUsedAt,
			})
		}
	}
	return result, "", ""
}

type StatusRow struct {
	WorkspaceID string
	CMUXID      string
	Ordinal     int
	Title       string
	Exists      bool
}

type StatusResult struct {
	Rows []StatusRow
}

func (s *Service) Status(ctx context.Context, root string, workspaceID string) (StatusResult, string, string) {
	store := s.NewStore(root)
	mapping, err := store.Load()
	if err != nil {
		return StatusResult{}, "internal_error", fmt.Sprintf("load cmux mapping: %v", err)
	}
	runtime, err := s.NewClient().ListWorkspaces(ctx)
	if err != nil {
		return StatusResult{}, "cmux_list_failed", fmt.Sprintf("list cmux workspaces: %v", err)
	}
	_, exists, _, recErr := ReconcileMappingWithRuntime(store, mapping, runtime, false)
	if recErr != nil {
		return StatusResult{}, "internal_error", fmt.Sprintf("reconcile cmux mapping: %v", recErr)
	}
	workspaceIDs := make([]string, 0, len(mapping.Workspaces))
	for wsID := range mapping.Workspaces {
		if workspaceID != "" && workspaceID != wsID {
			continue
		}
		workspaceIDs = append(workspaceIDs, wsID)
	}
	sort.Strings(workspaceIDs)
	out := StatusResult{Rows: make([]StatusRow, 0)}
	for _, wsID := range workspaceIDs {
		ws := mapping.Workspaces[wsID]
		for _, e := range ws.Entries {
			out.Rows = append(out.Rows, StatusRow{
				WorkspaceID: wsID,
				CMUXID:      e.CMUXWorkspaceID,
				Ordinal:     e.Ordinal,
				Title:       e.TitleSnapshot,
				Exists:      exists[e.CMUXWorkspaceID],
			})
		}
	}
	return out, "", ""
}

func ReconcileMappingWithRuntime(store MappingStore, mapping cmuxmap.File, runtime []cmuxctl.Workspace, prune bool) (cmuxmap.File, map[string]bool, int, error) {
	exists := map[string]bool{}
	for _, row := range runtime {
		for _, handle := range []string{row.ID, row.Ref} {
			handle = strings.TrimSpace(handle)
			if handle != "" {
				exists[handle] = true
			}
		}
	}
	if !prune || len(exists) == 0 {
		return mapping, exists, 0, nil
	}
	prunedCount := 0
	for wsID, ws := range mapping.Workspaces {
		keep := make([]cmuxmap.Entry, 0, len(ws.Entries))
		for _, e := range ws.Entries {
			if exists[strings.TrimSpace(e.CMUXWorkspaceID)] {
				keep = append(keep, e)
				continue
			}
			prunedCount++
		}
		ws.Entries = keep
		mapping.Workspaces[wsID] = ws
	}
	if prunedCount > 0 {
		if err := store.Save(mapping); err != nil {
			return mapping, exists, prunedCount, err
		}
	}
	return mapping, exists, prunedCount, nil
}

func probeAndPruneByID(ctx context.Context, store MappingStore, mapping *cmuxmap.File, client Client) (int, string) {
	statusByID := map[string]int{}
	for _, ws := range mapping.Workspaces {
		for _, e := range ws.Entries {
			id := strings.TrimSpace(e.CMUXWorkspaceID)
			if id == "" {
				continue
			}
			if _, ok := statusByID[id]; ok {
				continue
			}
			payload, err := client.Identify(ctx, id, "")
			if err == nil {
				statusByID[id] = identifyWorkspaceHandleState(payload, id)
				continue
			}
			if IsNotFoundError(err) {
				statusByID[id] = -1
				continue
			}
			statusByID[id] = 0
		}
	}
	probeReachable := false
	for _, st := range statusByID {
		if st != 0 {
			probeReachable = true
			break
		}
	}
	if !probeReachable {
		return 0, "cmux probe could not verify any workspace; skipped stale pruning"
	}
	prunedCount := 0
	for wsID, ws := range mapping.Workspaces {
		keep := make([]cmuxmap.Entry, 0, len(ws.Entries))
		for _, e := range ws.Entries {
			id := strings.TrimSpace(e.CMUXWorkspaceID)
			st, ok := statusByID[id]
			if !ok || st >= 0 {
				keep = append(keep, e)
				continue
			}
			prunedCount++
		}
		ws.Entries = keep
		mapping.Workspaces[wsID] = ws
	}
	if prunedCount > 0 {
		if err := store.Save(*mapping); err != nil {
			return 0, fmt.Sprintf("save cmux mapping after probe prune: %v", err)
		}
	}
	return prunedCount, ""
}

func resetWorkspaceCMUXMapping(mapping *cmuxmap.File, mapMu *sync.Mutex, workspaceID string) {
	mapMu.Lock()
	defer mapMu.Unlock()
	ws := mapping.Workspaces[workspaceID]
	ws.Entries = nil
	ws.NextOrdinal = 1
	mapping.Workspaces[workspaceID] = ws
}

func identifyWorkspaceHandleState(payload map[string]any, handle string) int {
	handle = strings.TrimSpace(handle)
	if handle == "" || payload == nil {
		return 0
	}
	for _, candidate := range []map[string]any{payload, nestedAnyMap(payload, "caller")} {
		switch workspaceHandleState(candidate, handle) {
		case 1:
			return 1
		case -1:
			return -1
		}
	}
	if raw, ok := payload["caller"]; ok && raw == nil {
		return -1
	}
	return 0
}

func nestedAnyMap(payload map[string]any, key string) map[string]any {
	if payload == nil {
		return nil
	}
	raw, ok := payload[key]
	if !ok {
		return nil
	}
	child, _ := raw.(map[string]any)
	return child
}

func workspaceHandleState(payload map[string]any, handle string) int {
	if payload == nil {
		return 0
	}
	found := false
	for _, key := range []string{"workspace_ref", "workspace_id"} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		found = true
		value, _ := raw.(string)
		if strings.TrimSpace(value) == handle {
			return 1
		}
	}
	if found {
		return -1
	}
	return 0
}

func IsNotFoundError(err error) bool {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "not found") || strings.Contains(msg, "not_found") || strings.Contains(msg, "unknown workspace")
}

type SwitchWorkspaceCandidate struct {
	WorkspaceID string
	MappedCount int
}

type SwitchEntryCandidate struct {
	CMUXWorkspaceID string
	Ordinal         int
	Title           string
}

type SwitchSelector interface {
	SelectWorkspace(candidates []SwitchWorkspaceCandidate) (string, error)
	SelectEntry(workspaceID string, candidates []SwitchEntryCandidate) (string, error)
}

type SwitchResult struct {
	WorkspaceID     string
	CMUXWorkspaceID string
	Ordinal         int
	Title           string
}

func (s *Service) Switch(ctx context.Context, root string, workspaceID string, cmuxHandle string, nonInteractive bool, selector SwitchSelector) (SwitchResult, string, string) {
	if s.NewClient == nil || s.NewStore == nil {
		return SwitchResult{}, "internal_error", "cmux service is not initialized"
	}
	store := s.NewStore(root)
	releaseLock, err := store.AcquireLock(ctx)
	if err != nil {
		return SwitchResult{}, "state_write_failed", fmt.Sprintf("acquire cmux mapping lock: %v", err)
	}
	defer releaseLock()
	mapping, err := store.Load()
	if err != nil {
		return SwitchResult{}, "state_write_failed", fmt.Sprintf("load cmux mapping: %v", err)
	}

	client := s.NewClient()
	if runtime, rerr := client.ListWorkspaces(ctx); rerr == nil {
		reconciled, _, _, recErr := ReconcileMappingWithRuntime(store, mapping, runtime, true)
		if recErr != nil {
			return SwitchResult{}, "state_write_failed", fmt.Sprintf("reconcile cmux mapping: %v", recErr)
		}
		mapping = reconciled
	}

	wsID, entry, code, msg := resolveSwitchTarget(mapping, workspaceID, cmuxHandle, nonInteractive, selector)
	if code != "" {
		return SwitchResult{}, code, msg
	}
	if err := client.SelectWorkspace(ctx, entry.CMUXWorkspaceID); err != nil {
		return SwitchResult{}, "cmux_select_failed", fmt.Sprintf("select cmux workspace: %v", err)
	}

	ws := mapping.Workspaces[wsID]
	for i := range ws.Entries {
		if ws.Entries[i].CMUXWorkspaceID == entry.CMUXWorkspaceID {
			ws.Entries[i].LastUsedAt = s.Now().UTC().Format(time.RFC3339)
			entry = ws.Entries[i]
			break
		}
	}
	mapping.Workspaces[wsID] = ws
	if err := store.Save(mapping); err != nil {
		return SwitchResult{}, "state_write_failed", fmt.Sprintf("save cmux mapping: %v", err)
	}
	return SwitchResult{
		WorkspaceID:     wsID,
		CMUXWorkspaceID: entry.CMUXWorkspaceID,
		Ordinal:         entry.Ordinal,
		Title:           entry.TitleSnapshot,
	}, "", ""
}

func resolveSwitchTarget(mapping cmuxmap.File, workspaceID string, cmuxHandle string, nonInteractive bool, selector SwitchSelector) (string, cmuxmap.Entry, string, string) {
	workspaceID = strings.TrimSpace(workspaceID)
	cmuxHandle = strings.TrimSpace(cmuxHandle)

	if workspaceID != "" {
		ws, ok := mapping.Workspaces[workspaceID]
		if !ok || len(ws.Entries) == 0 {
			return "", cmuxmap.Entry{}, "cmux_not_mapped", fmt.Sprintf("no cmux mapping found for workspace: %s", workspaceID)
		}
		if cmuxHandle == "" {
			return resolveSwitchEntry(workspaceID, ws.Entries, nonInteractive, selector)
		}
		matches := filterSwitchEntries(ws.Entries, cmuxHandle)
		switch len(matches) {
		case 1:
			return workspaceID, matches[0], "", ""
		case 0:
			if nonInteractive {
				return "", cmuxmap.Entry{}, "cmux_not_mapped", fmt.Sprintf("cmux target not found in workspace %s: %s", workspaceID, cmuxHandle)
			}
			return resolveSwitchEntry(workspaceID, ws.Entries, nonInteractive, selector)
		default:
			if nonInteractive {
				return "", cmuxmap.Entry{}, "cmux_ambiguous_target", fmt.Sprintf("multiple cmux targets matched: %s", cmuxHandle)
			}
			return resolveSwitchEntry(workspaceID, matches, nonInteractive, selector)
		}
	}

	if cmuxHandle != "" {
		type matched struct {
			workspaceID string
			entry       cmuxmap.Entry
		}
		all := []matched{}
		for wsID, ws := range mapping.Workspaces {
			for _, e := range filterSwitchEntries(ws.Entries, cmuxHandle) {
				all = append(all, matched{workspaceID: wsID, entry: e})
			}
		}
		if len(all) == 1 {
			return all[0].workspaceID, all[0].entry, "", ""
		}
		if nonInteractive {
			if len(all) == 0 {
				return "", cmuxmap.Entry{}, "cmux_not_mapped", fmt.Sprintf("cmux target not found: %s", cmuxHandle)
			}
			return "", cmuxmap.Entry{}, "cmux_ambiguous_target", fmt.Sprintf("multiple cmux targets matched: %s", cmuxHandle)
		}
	}

	if nonInteractive {
		return "", cmuxmap.Entry{}, "non_interactive_selection_required", "switch requires --workspace/--cmux in --format json mode"
	}
	wsID, code, msg := selectSwitchWorkspace(mapping, selector)
	if code != "" {
		return "", cmuxmap.Entry{}, code, msg
	}
	return resolveSwitchEntry(wsID, mapping.Workspaces[wsID].Entries, nonInteractive, selector)
}

func selectSwitchWorkspace(mapping cmuxmap.File, selector SwitchSelector) (string, string, string) {
	ids := make([]string, 0, len(mapping.Workspaces))
	for wsID, ws := range mapping.Workspaces {
		if len(ws.Entries) > 0 {
			ids = append(ids, wsID)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return "", "cmux_not_mapped", "no cmux mappings available"
	}
	if len(ids) == 1 {
		return ids[0], "", ""
	}
	if selector == nil {
		return "", "non_interactive_selection_required", "interactive workspace selection requires a TTY"
	}
	candidates := make([]SwitchWorkspaceCandidate, 0, len(ids))
	for _, wsID := range ids {
		candidates = append(candidates, SwitchWorkspaceCandidate{
			WorkspaceID: wsID,
			MappedCount: len(mapping.Workspaces[wsID].Entries),
		})
	}
	selected, err := selector.SelectWorkspace(candidates)
	if err != nil {
		msg := strings.TrimSpace(err.Error())
		if msg == "" {
			msg = "interactive workspace selection failed"
		}
		return "", "cmux_not_mapped", msg
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "", "cmux_not_mapped", "cmux switch requires exactly one workspace selected"
	}
	if _, ok := mapping.Workspaces[selected]; !ok {
		return "", "cmux_not_mapped", fmt.Sprintf("selected workspace not found: %s", selected)
	}
	return selected, "", ""
}

func resolveSwitchEntry(workspaceID string, entries []cmuxmap.Entry, nonInteractive bool, selector SwitchSelector) (string, cmuxmap.Entry, string, string) {
	if len(entries) == 0 {
		return "", cmuxmap.Entry{}, "cmux_not_mapped", fmt.Sprintf("no cmux mapping found for workspace: %s", workspaceID)
	}
	if len(entries) == 1 {
		return workspaceID, entries[0], "", ""
	}
	if nonInteractive {
		return "", cmuxmap.Entry{}, "cmux_ambiguous_target", "multiple cmux mappings found; provide --cmux"
	}
	if selector == nil {
		return "", cmuxmap.Entry{}, "non_interactive_selection_required", "interactive cmux selection requires a TTY"
	}
	candidates := make([]SwitchEntryCandidate, 0, len(entries))
	for _, e := range entries {
		title := strings.TrimSpace(e.TitleSnapshot)
		if title == "" {
			title = fmt.Sprintf("ordinal=%d", e.Ordinal)
		}
		candidates = append(candidates, SwitchEntryCandidate{
			CMUXWorkspaceID: e.CMUXWorkspaceID,
			Ordinal:         e.Ordinal,
			Title:           title,
		})
	}
	selected, err := selector.SelectEntry(workspaceID, candidates)
	if err != nil {
		msg := strings.TrimSpace(err.Error())
		if msg == "" {
			msg = "interactive cmux selection failed"
		}
		return "", cmuxmap.Entry{}, "cmux_not_mapped", msg
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "", cmuxmap.Entry{}, "cmux_not_mapped", "cmux switch requires exactly one target selected"
	}
	for _, e := range entries {
		if e.CMUXWorkspaceID == selected {
			return workspaceID, e, "", ""
		}
	}
	return "", cmuxmap.Entry{}, "cmux_not_mapped", fmt.Sprintf("selected cmux target not found: %s", selected)
}

func filterSwitchEntries(entries []cmuxmap.Entry, handle string) []cmuxmap.Entry {
	handle = strings.TrimSpace(handle)
	out := make([]cmuxmap.Entry, 0, len(entries))
	for _, e := range entries {
		if e.CMUXWorkspaceID == handle {
			out = append(out, e)
			continue
		}
		if handle == fmt.Sprintf("workspace:%d", e.Ordinal) {
			out = append(out, e)
		}
	}
	return out
}
