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
	RenameWorkspace(ctx context.Context, workspace string, title string) error
	SelectWorkspace(ctx context.Context, workspace string) error
	ListWorkspaces(ctx context.Context) ([]cmuxctl.Workspace, error)
	Identify(ctx context.Context, workspace string, surface string) (map[string]any, error)
}

type NewClientFunc func() Client
type NewStoreFunc func(root string) cmuxmap.Store

type Service struct {
	NewClient NewClientFunc
	NewStore  NewStoreFunc
	Now       func() time.Time
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
}

type OpenFailure struct {
	WorkspaceID string
	Code        string
	Message     string
}

type OpenResult struct {
	Results  []OpenResultItem
	Failures []OpenFailure
}

func (s *Service) Open(ctx context.Context, root string, targets []OpenTarget, concurrency int, multi bool) (OpenResult, string, string) {
	if s.NewClient == nil || s.NewStore == nil {
		return OpenResult{}, "internal_error", "cmux service is not initialized"
	}
	client := s.NewClient()
	caps, err := client.Capabilities(ctx)
	if err != nil {
		return OpenResult{}, "cmux_capability_missing", fmt.Sprintf("read cmux capabilities: %v", err)
	}
	for _, method := range []string{"workspace.create", "workspace.rename", "workspace.select"} {
		if _, ok := caps.Methods[method]; !ok {
			return OpenResult{}, "cmux_capability_missing", fmt.Sprintf("cmux capability missing: %s", method)
		}
	}

	store := s.NewStore(root)
	mapping, err := store.Load()
	if err != nil {
		return OpenResult{}, "state_write_failed", fmt.Sprintf("load cmux mapping: %v", err)
	}

	result := OpenResult{
		Results:  make([]OpenResultItem, 0, len(targets)),
		Failures: make([]OpenFailure, 0),
	}
	if multi && concurrency > 1 {
		result = s.openConcurrent(ctx, targets, concurrency, &mapping)
	} else {
		result = s.openSequential(ctx, client, targets, &mapping)
	}
	if len(result.Results) > 0 {
		if err := store.Save(mapping); err != nil {
			return OpenResult{}, "state_write_failed", fmt.Sprintf("save cmux mapping: %v", err)
		}
	}
	return result, "", ""
}

func (s *Service) openSequential(ctx context.Context, client Client, targets []OpenTarget, mapping *cmuxmap.File) OpenResult {
	res := OpenResult{
		Results:  make([]OpenResultItem, 0, len(targets)),
		Failures: make([]OpenFailure, 0),
	}
	var mapMu sync.Mutex
	for _, target := range targets {
		item, code, msg := s.openOne(ctx, client, target, mapping, &mapMu)
		if code != "" {
			res.Failures = append(res.Failures, OpenFailure{WorkspaceID: target.WorkspaceID, Code: code, Message: msg})
			return res
		}
		res.Results = append(res.Results, item)
	}
	return res
}

func (s *Service) openConcurrent(ctx context.Context, targets []OpenTarget, concurrency int, mapping *cmuxmap.File) OpenResult {
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
				item, code, msg := s.openOne(ctx, client, job.target, mapping, &mapMu)
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

func (s *Service) openOne(ctx context.Context, client Client, target OpenTarget, mapping *cmuxmap.File, mapMu *sync.Mutex) (OpenResultItem, string, string) {
	// 1:1 policy: if mapping already exists and runtime workspace is reachable, switch to it.
	mapMu.Lock()
	existingEntries := append([]cmuxmap.Entry{}, mapping.Workspaces[target.WorkspaceID].Entries...)
	mapMu.Unlock()
	if len(existingEntries) > 0 {
		existing := existingEntries[0]
		_, ierr := client.Identify(ctx, existing.CMUXWorkspaceID, "")
		if ierr == nil {
			if err := client.SelectWorkspace(ctx, existing.CMUXWorkspaceID); err != nil {
				return OpenResultItem{}, "cmux_select_failed", fmt.Sprintf("select cmux workspace: %v", err)
			}
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
			}, "", ""
		}
		if !IsNotFoundError(ierr) {
			return OpenResultItem{}, "cmux_identify_failed", fmt.Sprintf("identify cmux workspace: %v", ierr)
		}
		// stale mapping entry: clear and recreate with ordinal reset.
		mapMu.Lock()
		ws := mapping.Workspaces[target.WorkspaceID]
		ws.Entries = nil
		ws.NextOrdinal = 1
		mapping.Workspaces[target.WorkspaceID] = ws
		mapMu.Unlock()
	}

	cmuxWorkspaceID, err := client.CreateWorkspaceWithCommand(ctx, fmt.Sprintf("cd %s", shellQuoteCDPath(target.WorkspacePath)))
	if err != nil {
		return OpenResultItem{}, "cmux_create_failed", fmt.Sprintf("create cmux workspace: %v", err)
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
	if err := client.RenameWorkspace(ctx, cmuxWorkspaceID, cmuxTitle); err != nil {
		return OpenResultItem{}, "cmux_rename_failed", fmt.Sprintf("rename cmux workspace: %v", err)
	}
	if err := client.SelectWorkspace(ctx, cmuxWorkspaceID); err != nil {
		return OpenResultItem{}, "cmux_select_failed", fmt.Sprintf("select cmux workspace: %v", err)
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
	return OpenResultItem{
		WorkspaceID:     target.WorkspaceID,
		WorkspacePath:   target.WorkspacePath,
		CMUXWorkspaceID: cmuxWorkspaceID,
		Ordinal:         ordinal,
		Title:           cmuxTitle,
		ReusedExisting:  false,
	}, "", ""
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

func ReconcileMappingWithRuntime(store cmuxmap.Store, mapping cmuxmap.File, runtime []cmuxctl.Workspace, prune bool) (cmuxmap.File, map[string]bool, int, error) {
	exists := map[string]bool{}
	for _, row := range runtime {
		id := strings.TrimSpace(row.ID)
		if id != "" {
			exists[id] = true
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

func probeAndPruneByID(ctx context.Context, store cmuxmap.Store, mapping *cmuxmap.File, client Client) (int, string) {
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
			_, err := client.Identify(ctx, id, "")
			if err == nil {
				statusByID[id] = 1
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

func IsNotFoundError(err error) bool {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "not found") || strings.Contains(msg, "unknown workspace")
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
