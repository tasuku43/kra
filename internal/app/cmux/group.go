package cmux

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

const rootMappingWorkspaceID = "KRA_ROOT"

var requiredWorkspaceGroupMethods = []string{
	"workspace.group.list",
	"workspace.group.create",
	"workspace.group.add",
	"workspace.group.remove",
	"workspace.group.set_anchor",
	"workspace.group.ungroup",
	"workspace.group.focus",
}

type workspaceGroupClient interface {
	ListWorkspaceGroups(ctx context.Context, window string) (cmuxctl.WorkspaceGroupList, error)
	CreateWorkspaceGroup(ctx context.Context, opts cmuxctl.WorkspaceGroupCreateOptions) (cmuxctl.WorkspaceGroup, error)
	AddWorkspaceToGroup(ctx context.Context, group string, workspace string) error
	RemoveWorkspaceFromGroup(ctx context.Context, group string, workspace string) error
	SetWorkspaceGroupAnchor(ctx context.Context, group string, workspace string) error
	UngroupWorkspaceGroup(ctx context.Context, group string) error
	FocusWorkspaceGroup(ctx context.Context, group string) error
}

type workspaceOptionsClient interface {
	CreateWorkspaceWithOptions(ctx context.Context, opts cmuxctl.WorkspaceCreateOptions) (cmuxctl.WorkspaceCreateResult, error)
}

type workspaceWindowClient interface {
	ListWindows(ctx context.Context) ([]cmuxctl.Window, error)
}

// cmux v0.64.22 durably accepts operation_id before constructing a workspace.
// Repeating the same ID returns the live workspace when creation succeeded, or
// an already_completed error without creating again when only the tombstone
// survived. One retry is therefore at-most-once even when the first response
// was lost.
func createWorkspaceWithIdempotencyRetry(
	ctx context.Context,
	creator workspaceOptionsClient,
	opts cmuxctl.WorkspaceCreateOptions,
) (cmuxctl.WorkspaceCreateResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	created, firstErr := creator.CreateWorkspaceWithOptions(ctx, opts)
	if firstErr == nil {
		return created, nil
	}

	operationID := strings.TrimSpace(opts.OperationID)
	if operationID == "" {
		return cmuxctl.WorkspaceCreateResult{}, firstErr
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return cmuxctl.WorkspaceCreateResult{}, fmt.Errorf(
			"workspace.create result is uncertain; operation_id=%s window_id=%s group_id=%s cwd=%q title=%q; first attempt: %v; caller context ended before retry: %v; inspect cmux workspaces and close any unintended provisional workspace manually",
			operationID,
			strings.TrimSpace(opts.WindowID),
			strings.TrimSpace(opts.GroupID),
			strings.TrimSpace(opts.CWD),
			strings.TrimSpace(opts.Title),
			firstErr,
			ctxErr,
		)
	}
	if !workspaceCreateRetryableError(firstErr) {
		return cmuxctl.WorkspaceCreateResult{}, firstErr
	}
	retryCtx, cancel := context.WithTimeout(ctx, cmuxCleanupTimeout)
	defer cancel()
	retried, retryErr := creator.CreateWorkspaceWithOptions(retryCtx, opts)
	if retryErr == nil {
		return retried, nil
	}

	return cmuxctl.WorkspaceCreateResult{}, fmt.Errorf(
		"workspace.create result is uncertain after an idempotent retry; operation_id=%s window_id=%s group_id=%s cwd=%q title=%q; first attempt: %v; retry: %v; inspect cmux workspaces and close any unintended provisional workspace manually",
		operationID,
		strings.TrimSpace(opts.WindowID),
		strings.TrimSpace(opts.GroupID),
		strings.TrimSpace(opts.CWD),
		strings.TrimSpace(opts.Title),
		firstErr,
		retryErr,
	)
}

func workspaceCreateRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"timed out",
		"timeout",
		"deadline exceeded",
		"connection refused",
		"connection reset",
		"connection aborted",
		"connection closed",
		"connection lost",
		"failed to connect",
		"unable to connect",
		"broken pipe",
		"unexpected eof",
		"decode cmux rpc workspace.create json output",
		"socket hang up",
		"socket closed",
		"socket is not connected",
		"transport error",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

type groupTransaction struct {
	Enabled               bool
	Changed               bool
	Warning               string
	Group                 cmuxmap.GroupMapping
	CreatedGroupID        string
	CreatedAnchorID       string
	CreatedRepairAnchorID string
	ReusedRootAnchor      bool
}

func supportsWorkspaceGroups(caps cmuxctl.Capabilities) bool {
	for _, method := range requiredWorkspaceGroupMethods {
		if _, ok := caps.Methods[method]; !ok {
			return false
		}
	}
	return true
}

func workspaceGroupCapabilityWarning(caps cmuxctl.Capabilities) string {
	missing := make([]string, 0)
	for _, method := range requiredWorkspaceGroupMethods {
		if _, ok := caps.Methods[method]; !ok {
			missing = append(missing, method)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "cmux Workspace Group integration skipped; missing: " + strings.Join(missing, ", ")
}

func (s *Service) ensureRootGroup(
	ctx context.Context,
	root string,
	client Client,
	caps cmuxctl.Capabilities,
	mapping *cmuxmap.File,
) (groupTransaction, string, string) {
	if !supportsWorkspaceGroups(caps) {
		return groupTransaction{Warning: workspaceGroupCapabilityWarning(caps)}, "", ""
	}
	groups, ok := client.(workspaceGroupClient)
	if !ok {
		return groupTransaction{Warning: "cmux Workspace Group integration skipped; adapter unavailable"}, "", ""
	}
	creator, ok := client.(workspaceOptionsClient)
	if !ok {
		return groupTransaction{Warning: "cmux Workspace Group integration skipped; atomic workspace create unavailable"}, "", ""
	}

	name := rootWorkspaceGroupName(root)
	var list cmuxctl.WorkspaceGroupList
	ambientListed := false
	if mapping.RootGroup != nil {
		mapped := *mapping.RootGroup
		window := strings.TrimSpace(mapped.CMUXWindowID)
		scannedWindows := make(map[string]struct{})
		recordScannedWindow := func(requested string, listed cmuxctl.WorkspaceGroupList) {
			for _, windowID := range []string{requested, listed.WindowID} {
				if windowID = strings.TrimSpace(windowID); windowID != "" {
					scannedWindows[windowID] = struct{}{}
				}
			}
		}
		var err error
		list, err = groups.ListWorkspaceGroups(ctx, window)
		ambientListed = window == ""
		if err == nil {
			recordScannedWindow(window, list)
			if live, found := findWorkspaceGroup(list.Groups, mapped.CMUXGroupID); found {
				return s.reconcileLiveRootGroup(ctx, root, client, groups, creator, mapping, list, live, mapped, name)
			}
		} else if ambientListed {
			return groupTransaction{}, "cmux_group_list_failed", fmt.Sprintf("list cmux workspace groups: %v", err)
		}
		if !ambientListed {
			list, err = groups.ListWorkspaceGroups(ctx, "")
			ambientListed = true
			if err != nil {
				return groupTransaction{}, "cmux_group_list_failed", fmt.Sprintf("list cmux workspace groups: %v", err)
			}
			recordScannedWindow("", list)
			if live, found := findWorkspaceGroup(list.Groups, mapped.CMUXGroupID); found {
				return s.reconcileLiveRootGroup(ctx, root, client, groups, creator, mapping, list, live, mapped, name)
			}
		}

		windows, ok := client.(workspaceWindowClient)
		if !ok {
			return groupTransaction{}, "cmux_group_list_failed", "enumerate cmux windows: adapter unavailable"
		}
		allWindows, err := windows.ListWindows(ctx)
		if err != nil {
			return groupTransaction{}, "cmux_group_list_failed", fmt.Sprintf("enumerate cmux windows: %v", err)
		}
		if len(allWindows) == 0 {
			return groupTransaction{}, "cmux_group_list_failed", "enumerate cmux windows: no windows returned"
		}
		for _, candidateWindow := range allWindows {
			windowID := strings.TrimSpace(candidateWindow.ID)
			if windowID == "" {
				return groupTransaction{}, "cmux_group_list_failed", "enumerate cmux windows: response omitted stable window id"
			}
			if _, scanned := scannedWindows[windowID]; scanned {
				continue
			}
			candidateList, err := groups.ListWorkspaceGroups(ctx, windowID)
			if err != nil {
				return groupTransaction{}, "cmux_group_list_failed", fmt.Sprintf("list cmux workspace groups in window %s: %v", windowID, err)
			}
			recordScannedWindow(windowID, candidateList)
			if live, found := findWorkspaceGroup(candidateList.Groups, mapped.CMUXGroupID); found {
				return s.reconcileLiveRootGroup(ctx, root, client, groups, creator, mapping, candidateList, live, mapped, name)
			}
		}
	}

	if !ambientListed {
		var err error
		list, err = groups.ListWorkspaceGroups(ctx, "")
		if err != nil {
			return groupTransaction{}, "cmux_group_list_failed", fmt.Sprintf("list cmux workspace groups: %v", err)
		}
	}
	// Runtime refs are presentation-only and can be renumbered. Never create a
	// managed group unless its owning window has a stable UUID that can be
	// persisted and used for later reconciliation.
	window := strings.TrimSpace(list.WindowID)
	legacyRoot, legacyWindowID, legacyReachable, code, message := reachableRootEntry(ctx, client, mapping)
	if code != "" {
		return groupTransaction{}, code, message
	}
	if legacyReachable {
		if strings.TrimSpace(list.WindowID) != legacyWindowID {
			rootWindowList, err := groups.ListWorkspaceGroups(ctx, legacyWindowID)
			if err != nil {
				return groupTransaction{}, "cmux_group_list_failed", fmt.Sprintf("list cmux workspace groups in legacy root window %s: %v", legacyWindowID, err)
			}
			if returnedWindowID := strings.TrimSpace(rootWindowList.WindowID); returnedWindowID != "" && returnedWindowID != legacyWindowID {
				return groupTransaction{}, "cmux_group_list_failed", fmt.Sprintf("list cmux workspace groups in legacy root window %s returned unexpected window %s", legacyWindowID, returnedWindowID)
			}
			rootWindowList.WindowID = legacyWindowID
			list = rootWindowList
		}
		window = legacyWindowID
		for _, liveGroup := range list.Groups {
			if !workspaceGroupContains(liveGroup, legacyRoot.CMUXWorkspaceID) {
				continue
			}
			foreignGroupID := chooseWorkspaceGroupHandle(liveGroup)
			if foreignGroupID == "" {
				foreignGroupID = "unknown"
			}
			return groupTransaction{}, "cmux_group_conflict", fmt.Sprintf(
				"cmux root workspace %s belongs to foreign group %s",
				legacyRoot.CMUXWorkspaceID,
				foreignGroupID,
			)
		}
	}
	if window == "" {
		return groupTransaction{}, "cmux_group_create_failed", "stable cmux workspace group window id could not be resolved"
	}
	children := []string(nil)
	if legacyReachable {
		children = []string{legacyRoot.CMUXWorkspaceID}
	}
	created, err := groups.CreateWorkspaceGroup(ctx, cmuxctl.WorkspaceGroupCreateOptions{
		Name:              name,
		CWD:               root,
		Window:            window,
		ChildWorkspaceIDs: children,
	})
	if err != nil {
		return groupTransaction{}, "cmux_group_create_failed", fmt.Sprintf(
			"create cmux workspace group in window %q with name %q: %v; result is uncertain and no cleanup was attempted because ownership could not be proven; inspect cmux Workspace Groups manually",
			window,
			name,
			err,
		)
	}
	created.WindowID = list.WindowID
	created.WindowRef = list.WindowRef
	groupHandle := chooseWorkspaceGroupHandle(created)
	groupID := strings.TrimSpace(created.ID)
	anchorID := strings.TrimSpace(created.AnchorWorkspaceID)
	if groupHandle != "" {
		if _, existed := findWorkspaceGroup(list.Groups, groupHandle); existed {
			return groupTransaction{}, "cmux_group_create_failed", fmt.Sprintf(
				"create cmux workspace group: response referenced pre-existing group %s; no foreign cleanup was attempted",
				groupHandle,
			)
		}
	}
	if anchorID != "" {
		for _, existingGroup := range list.Groups {
			if !workspaceGroupContains(existingGroup, anchorID) {
				continue
			}
			message := fmt.Sprintf(
				"create cmux workspace group: response reused anchor %s from pre-existing group %s; no foreign cleanup was attempted",
				anchorID,
				firstNonEmpty(chooseWorkspaceGroupHandle(existingGroup), "unknown"),
			)
			if groupHandle != "" {
				message = s.cleanupCreatedGroup(ctx, client, groups, groupHandle, "", message)
			}
			return groupTransaction{}, "cmux_group_create_failed", message
		}
	}
	if groupID == "" || anchorID == "" {
		message := "create cmux workspace group: response omitted group or anchor stable id"
		message = s.cleanupCreatedGroup(ctx, client, groups, groupHandle, anchorID, message)
		return groupTransaction{}, "cmux_group_create_failed", message
	}

	txn := groupTransaction{
		Enabled:         true,
		Changed:         true,
		CreatedGroupID:  groupID,
		CreatedAnchorID: anchorID,
	}
	if legacyReachable {
		if err := groups.SetWorkspaceGroupAnchor(ctx, groupID, legacyRoot.CMUXWorkspaceID); err != nil {
			message := s.cleanupCreatedGroup(ctx, client, groups, groupID, anchorID, fmt.Sprintf("set legacy root as cmux group anchor: %v", err))
			return groupTransaction{}, "cmux_group_setup_failed", message
		}
		if err := groups.RemoveWorkspaceFromGroup(ctx, groupID, anchorID); err != nil {
			message := s.cleanupCreatedGroup(ctx, client, groups, groupID, anchorID, fmt.Sprintf("remove provisional cmux group anchor: %v", err))
			return groupTransaction{}, "cmux_group_setup_failed", message
		}
		cleanupCtx, cancel := cleanupContext(ctx)
		if err := client.CloseWorkspace(cleanupCtx, anchorID); err != nil && !IsNotFoundError(err) {
			cancel()
			message := s.cleanupCreatedGroup(ctx, client, groups, groupID, anchorID, fmt.Sprintf("close provisional cmux group anchor %s: %v", anchorID, err))
			return groupTransaction{}, "cmux_group_setup_failed", message
		}
		cancel()
		anchorID = legacyRoot.CMUXWorkspaceID
		txn.CreatedAnchorID = ""
		txn.ReusedRootAnchor = true
	}

	now := s.Now().UTC().Format(time.RFC3339)
	txn.Group = cmuxmap.GroupMapping{
		CMUXWindowID:      firstNonEmpty(list.WindowID, created.WindowID),
		CMUXGroupID:       strings.TrimSpace(created.ID),
		AnchorWorkspaceID: anchorID,
		NameSnapshot:      strings.TrimSpace(created.Name),
		CreatedAt:         now,
		LastUsedAt:        now,
	}
	if txn.Group.CMUXGroupID == "" {
		txn.Group.CMUXGroupID = groupID
	}
	if txn.Group.NameSnapshot == "" {
		txn.Group.NameSnapshot = name
	}
	mapping.RootGroup = cloneGroupMapping(txn.Group)
	setRootAnchorMapping(mapping, txn.Group.AnchorWorkspaceID, txn.Group.NameSnapshot, now, legacyRoot)
	return txn, "", ""
}

func (s *Service) reconcileLiveRootGroup(
	ctx context.Context,
	root string,
	client Client,
	groups workspaceGroupClient,
	creator workspaceOptionsClient,
	mapping *cmuxmap.File,
	list cmuxctl.WorkspaceGroupList,
	live cmuxctl.WorkspaceGroup,
	mapped cmuxmap.GroupMapping,
	defaultName string,
) (groupTransaction, string, string) {
	groupID := strings.TrimSpace(live.ID)
	if groupID == "" {
		return groupTransaction{}, "cmux_group_list_failed", "managed cmux workspace group response omitted stable group id"
	}
	desiredAnchor := strings.TrimSpace(mapped.AnchorWorkspaceID)
	if desiredAnchor == "" {
		desiredAnchor = firstRootMappingID(mapping)
	}
	member := workspaceGroupContains(live, desiredAnchor)
	txn := groupTransaction{Enabled: true, Group: mapped}
	if desiredAnchor != "" && member && desiredAnchor != strings.TrimSpace(live.AnchorWorkspaceID) {
		if err := groups.SetWorkspaceGroupAnchor(ctx, groupID, desiredAnchor); err != nil {
			return groupTransaction{}, "cmux_group_setup_failed", fmt.Sprintf("restore cmux root group anchor: %v", err)
		}
		txn.Changed = true
	}
	if desiredAnchor == "" || !member {
		name := strings.TrimSpace(live.Name)
		if name == "" {
			name = defaultName
		}
		created, err := createWorkspaceWithIdempotencyRetry(ctx, creator, cmuxctl.WorkspaceCreateOptions{
			OperationID:    newOperationID(),
			Title:          name,
			CWD:            root,
			Focus:          false,
			WindowID:       firstNonEmpty(list.WindowID, live.WindowID, mapped.CMUXWindowID),
			GroupID:        groupID,
			GroupPlacement: "top",
		})
		if err != nil {
			return groupTransaction{}, "cmux_group_setup_failed", fmt.Sprintf("create replacement cmux root anchor: %v", err)
		}
		createdHandle := chooseWorkspaceCreateHandle(created)
		desiredAnchor = strings.TrimSpace(created.WorkspaceID)
		if desiredAnchor == "" || !workspaceCreateResultMatchesGroup(created, groupID) {
			returnedGroup := firstNonEmpty(created.GroupID, created.GroupRef, "<empty>")
			message := s.compensateCreatedWorkspace(ctx, client, createdHandle, fmt.Sprintf(
				"replacement cmux root anchor returned inconsistent group %s (wanted %s)",
				returnedGroup,
				groupID,
			))
			return groupTransaction{}, "cmux_group_setup_failed", message
		}
		if err := groups.SetWorkspaceGroupAnchor(ctx, groupID, desiredAnchor); err != nil {
			message := s.compensateCreatedWorkspace(ctx, client, desiredAnchor, fmt.Sprintf("set replacement cmux root anchor: %v", err))
			return groupTransaction{}, "cmux_group_setup_failed", message
		}
		txn.CreatedRepairAnchorID = desiredAnchor
		txn.Changed = true
	}

	now := s.Now().UTC().Format(time.RFC3339)
	if mapped.CreatedAt == "" {
		mapped.CreatedAt = now
	}
	mapped.CMUXWindowID = firstNonEmpty(list.WindowID, live.WindowID, mapped.CMUXWindowID)
	mapped.CMUXGroupID = strings.TrimSpace(live.ID)
	if mapped.CMUXGroupID == "" {
		mapped.CMUXGroupID = groupID
	}
	mapped.AnchorWorkspaceID = desiredAnchor
	mapped.NameSnapshot = strings.TrimSpace(live.Name)
	if mapped.NameSnapshot == "" {
		mapped.NameSnapshot = defaultName
	}
	mapped.LastUsedAt = now
	if mapping.RootGroup == nil || *mapping.RootGroup != mapped {
		txn.Changed = true
	}
	txn.Group = mapped
	mapping.RootGroup = cloneGroupMapping(mapped)
	setRootAnchorMapping(mapping, desiredAnchor, mapped.NameSnapshot, now, cmuxmap.Entry{})
	return txn, "", ""
}

func reachableRootEntry(ctx context.Context, client Client, mapping *cmuxmap.File) (cmuxmap.Entry, string, bool, string, string) {
	rootMapping := mapping.Workspaces[rootMappingWorkspaceID]
	if len(rootMapping.Entries) == 0 {
		return cmuxmap.Entry{}, "", false, "", ""
	}
	for _, entry := range rootMapping.Entries {
		payload, err := client.Identify(ctx, entry.CMUXWorkspaceID, "")
		if err != nil {
			if IsNotFoundError(err) {
				continue
			}
			return cmuxmap.Entry{}, "", false, "cmux_identify_failed", fmt.Sprintf("identify cmux root workspace: %v", err)
		}
		switch identifyWorkspaceHandleState(payload, entry.CMUXWorkspaceID) {
		case 1:
			windowID := identifyCallerWindowID(payload, entry.CMUXWorkspaceID)
			if windowID == "" {
				return cmuxmap.Entry{}, "", false, "cmux_identify_failed", fmt.Sprintf("identify cmux root workspace: response omitted verified caller.window_id for %s", entry.CMUXWorkspaceID)
			}
			return entry, windowID, true, "", ""
		case -1:
			continue
		default:
			return cmuxmap.Entry{}, "", false, "cmux_identify_failed", fmt.Sprintf("identify cmux root workspace: could not verify requested workspace: %s", entry.CMUXWorkspaceID)
		}
	}
	return cmuxmap.Entry{}, "", false, "", ""
}

func identifyCallerWindowID(payload map[string]any, workspaceID string) string {
	caller := nestedAnyMap(payload, "caller")
	if workspaceHandleState(caller, strings.TrimSpace(workspaceID)) != 1 {
		return ""
	}
	windowID, _ := caller["window_id"].(string)
	return strings.TrimSpace(windowID)
}

func setRootAnchorMapping(mapping *cmuxmap.File, anchorID string, title string, now string, previous cmuxmap.Entry) {
	createdAt := previous.CreatedAt
	if createdAt == "" {
		if existing := mapping.Workspaces[rootMappingWorkspaceID].Entries; len(existing) > 0 {
			createdAt = existing[0].CreatedAt
		}
	}
	if createdAt == "" {
		createdAt = now
	}
	mapping.Workspaces[rootMappingWorkspaceID] = cmuxmap.WorkspaceMapping{
		NextOrdinal: 2,
		Entries: []cmuxmap.Entry{{
			CMUXWorkspaceID: anchorID,
			Ordinal:         1,
			TitleSnapshot:   title,
			CreatedAt:       createdAt,
			LastUsedAt:      now,
		}},
	}
}

func (s *Service) cleanupCreatedGroup(
	ctx context.Context,
	client Client,
	groups workspaceGroupClient,
	groupID string,
	anchorID string,
	message string,
) string {
	cleanupCtx, cancel := cleanupContext(ctx)
	defer cancel()
	groupDissolved := true
	if groupID != "" {
		if err := groups.UngroupWorkspaceGroup(cleanupCtx, groupID); err != nil && !IsNotFoundError(err) {
			message = fmt.Sprintf("%s; cleanup failed for cmux group %s: %v; ungroup it manually", message, groupID, err)
			groupDissolved = false
		}
	}
	if anchorID != "" {
		if groupDissolved {
			message = s.compensateCreatedWorkspace(ctx, client, anchorID, message)
		} else {
			message = fmt.Sprintf("%s; provisional anchor %s was left open to avoid promoting another group member; after ungrouping %s, close the anchor manually", message, anchorID, groupID)
		}
	}
	return message
}

func cleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), cmuxCleanupTimeout)
}

func groupMappingForOpen(txn groupTransaction) *cmuxmap.GroupMapping {
	if !txn.Enabled {
		return nil
	}
	return cloneGroupMapping(txn.Group)
}

func firstGroupID(group *cmuxmap.GroupMapping) string {
	if group == nil {
		return ""
	}
	return strings.TrimSpace(group.CMUXGroupID)
}

func (s *Service) ensureWorkspaceGroupMembership(
	ctx context.Context,
	client Client,
	rootGroup *cmuxmap.GroupMapping,
	workspaceID string,
) (bool, string, string) {
	if rootGroup == nil {
		return false, "", ""
	}
	groups, ok := client.(workspaceGroupClient)
	if !ok {
		return false, "cmux_group_setup_failed", "cmux Workspace Group adapter is unavailable"
	}
	list, err := groups.ListWorkspaceGroups(ctx, rootGroup.CMUXWindowID)
	if err != nil {
		return false, "cmux_group_list_failed", fmt.Sprintf("list cmux workspace groups: %v", err)
	}
	targetGroup, found := findWorkspaceGroup(list.Groups, rootGroup.CMUXGroupID)
	if !found {
		return false, "cmux_group_not_found", fmt.Sprintf("managed cmux workspace group not found: %s", rootGroup.CMUXGroupID)
	}
	if workspaceGroupContains(targetGroup, workspaceID) {
		return false, "", ""
	}
	for _, group := range list.Groups {
		if chooseWorkspaceGroupHandle(group) == chooseWorkspaceGroupHandle(targetGroup) {
			continue
		}
		if workspaceGroupContains(group, workspaceID) {
			return false, "cmux_group_conflict", fmt.Sprintf("cmux workspace %s belongs to foreign group %s", workspaceID, chooseWorkspaceGroupHandle(group))
		}
	}
	if err := groups.AddWorkspaceToGroup(ctx, rootGroup.CMUXGroupID, workspaceID); err != nil {
		return false, "cmux_group_setup_failed", fmt.Sprintf("add cmux workspace %s to managed group: %v", workspaceID, err)
	}
	return true, "", ""
}

func (s *Service) compensateOpenTransaction(
	ctx context.Context,
	client Client,
	results []OpenResultItem,
	txn groupTransaction,
	primaryMessage string,
) string {
	message := primaryMessage
	groups, _ := client.(workspaceGroupClient)
	anchorIDs := map[string]struct{}{}
	for _, workspaceID := range []string{txn.CreatedRepairAnchorID, txn.CreatedAnchorID} {
		if workspaceID = strings.TrimSpace(workspaceID); workspaceID != "" {
			anchorIDs[workspaceID] = struct{}{}
		}
	}
	closedIDs := map[string]struct{}{}
	closeOnce := func(workspaceID string) {
		workspaceID = strings.TrimSpace(workspaceID)
		if workspaceID == "" {
			return
		}
		if _, closed := closedIDs[workspaceID]; closed {
			return
		}
		closedIDs[workspaceID] = struct{}{}
		message = s.compensateCreatedWorkspace(ctx, client, workspaceID, message)
	}
	// Close newly-created child workspaces first so a provisional anchor is
	// never auto-promoted during rollback.
	for _, item := range results {
		if item.ReusedExisting {
			continue
		}
		workspaceID := strings.TrimSpace(item.CMUXWorkspaceID)
		if _, isAnchor := anchorIDs[workspaceID]; isAnchor {
			continue
		}
		closeOnce(workspaceID)
	}
	createdGroupDissolved := strings.TrimSpace(txn.CreatedGroupID) == ""
	if txn.CreatedGroupID != "" && groups == nil {
		message = fmt.Sprintf("%s; cleanup unavailable for cmux group %s; ungroup it manually", message, txn.CreatedGroupID)
	}
	if groups != nil && txn.CreatedGroupID != "" {
		cleanupCtx, cancel := cleanupContext(ctx)
		defer cancel()
		// cmux's remove operation is workspace-scoped rather than
		// compare-and-remove-by-group. A KRA-created provisional group is always
		// dissolved as a whole before its anchor is closed. Ungrouping preserves
		// reused and concurrently-added foreign members and prevents cmux from
		// promoting one of them when a child or anchor close fails.
		if err := groups.UngroupWorkspaceGroup(cleanupCtx, txn.CreatedGroupID); err != nil && !IsNotFoundError(err) {
			message = fmt.Sprintf("%s; cleanup failed for cmux group %s: %v; ungroup it manually", message, txn.CreatedGroupID, err)
		} else {
			createdGroupDissolved = true
		}
	}
	closeOnce(txn.CreatedRepairAnchorID)
	if createdGroupDissolved {
		closeOnce(txn.CreatedAnchorID)
	} else if anchorID := strings.TrimSpace(txn.CreatedAnchorID); anchorID != "" {
		message = fmt.Sprintf("%s; provisional anchor %s was left open to avoid promoting another group member; after ungrouping %s, close the anchor manually", message, anchorID, txn.CreatedGroupID)
	}
	return message
}

func rootWorkspaceGroupName(root string) string {
	base := strings.TrimSpace(filepath.Base(filepath.Clean(root)))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "kra"
	}
	return base
}

func findWorkspaceGroup(groups []cmuxctl.WorkspaceGroup, id string) (cmuxctl.WorkspaceGroup, bool) {
	id = strings.TrimSpace(id)
	for _, group := range groups {
		if strings.TrimSpace(group.ID) == id || strings.TrimSpace(group.Ref) == id {
			return group, true
		}
	}
	return cmuxctl.WorkspaceGroup{}, false
}

func workspaceGroupContains(group cmuxctl.WorkspaceGroup, workspaceID string) bool {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return false
	}
	for _, id := range group.MemberWorkspaceIDs {
		if strings.TrimSpace(id) == workspaceID {
			return true
		}
	}
	for _, ref := range group.MemberWorkspaceRefs {
		if strings.TrimSpace(ref) == workspaceID {
			return true
		}
	}
	return false
}

func firstRootMappingID(mapping *cmuxmap.File) string {
	entries := mapping.Workspaces[rootMappingWorkspaceID].Entries
	if len(entries) == 0 {
		return ""
	}
	return strings.TrimSpace(entries[0].CMUXWorkspaceID)
}

func chooseWorkspaceGroupHandle(group cmuxctl.WorkspaceGroup) string {
	return firstNonEmpty(group.ID, group.Ref)
}

func chooseWorkspaceCreateHandle(result cmuxctl.WorkspaceCreateResult) string {
	return firstNonEmpty(result.WorkspaceID, result.WorkspaceRef)
}

func workspaceCreateResultMatchesGroup(result cmuxctl.WorkspaceCreateResult, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return false
	}
	for _, returned := range []string{result.GroupID, result.GroupRef} {
		if strings.TrimSpace(returned) == expected {
			return true
		}
	}
	return false
}

func cloneGroupMapping(group cmuxmap.GroupMapping) *cmuxmap.GroupMapping {
	copy := group
	return &copy
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func newOperationID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		seed := fmt.Sprintf("%032x", uint64(time.Now().UnixNano()))
		return seed[0:8] + "-" + seed[8:12] + "-4" + seed[13:16] + "-8" + seed[17:20] + "-" + seed[20:32]
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(raw[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}
