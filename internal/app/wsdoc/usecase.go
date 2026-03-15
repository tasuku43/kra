package wsdoc

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/cmuxdocs"
	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

const viewerPrefix = "docs:"

type EnsureStageWorkspaceFunc func(ctx context.Context, root string) (workspaceRef string, code string, msg string)

type Client interface {
	Capabilities(ctx context.Context) (cmuxctl.Capabilities, error)
	Identify(ctx context.Context, workspace string, surface string) (map[string]any, error)
	ListPanes(ctx context.Context, workspace string) ([]cmuxctl.Pane, error)
	ListPaneSurfaces(ctx context.Context, workspace string, pane string) ([]cmuxctl.Surface, error)
	CreatePane(ctx context.Context, workspace string, direction string) (cmuxctl.PaneCreateResult, error)
	CreateSplit(ctx context.Context, direction string, workspace string, surface string) (cmuxctl.PaneCreateResult, error)
	MarkdownOpen(ctx context.Context, path string, workspace string, surface string) (cmuxctl.MarkdownOpenResult, error)
	MoveSurface(ctx context.Context, surface string, pane string, workspace string, after string, focus bool) (cmuxctl.MoveSurfaceResult, error)
	RenameTab(ctx context.Context, workspace string, surface string, title string) error
	CloseSurface(ctx context.Context, workspace string, surface string) error
}

type Service struct {
	NewClient            func() Client
	NewMappingStore      func(root string) cmuxmap.Store
	NewDocsStore         func(root string) cmuxdocs.Store
	EnsureStageWorkspace EnsureStageWorkspaceFunc
	Now                  func() time.Time
}

type OpenRequest struct {
	Root        string
	WorkspaceID string
	Path        string
	ScopePath   string
	SurfaceHint string
	Focus       bool
}

type OpenResult struct {
	Path             string
	ScopePath        string
	CMUXWorkspaceID  string
	DocsPaneRef      string
	ViewerSurfaceRef string
	Focused          bool
}

type docsSlot struct {
	PaneRef          string
	LastViewerRef    string
	BootstrapSurface string
}

type stageSlot struct {
	WorkspaceRef string
	PaneRef      string
	SurfaceRef   string
}

func NewService(
	newClient func() Client,
	newMappingStore func(root string) cmuxmap.Store,
	newDocsStore func(root string) cmuxdocs.Store,
	ensureStageWorkspace EnsureStageWorkspaceFunc,
) *Service {
	return &Service{
		NewClient:            newClient,
		NewMappingStore:      newMappingStore,
		NewDocsStore:         newDocsStore,
		EnsureStageWorkspace: ensureStageWorkspace,
		Now:                  time.Now,
	}
}

func (s *Service) Open(ctx context.Context, req OpenRequest) (OpenResult, string, string) {
	if s.NewClient == nil || s.NewMappingStore == nil || s.NewDocsStore == nil || s.EnsureStageWorkspace == nil {
		return OpenResult{}, "internal_error", "ws doc service is not initialized"
	}

	client := s.NewClient()
	caps, err := client.Capabilities(ctx)
	if err != nil {
		return OpenResult{}, "cmux_capability_missing", fmt.Sprintf("read cmux capabilities: %v", err)
	}
	for _, method := range []string{
		"markdown.open",
		"pane.create",
		"pane.list",
		"pane.surfaces",
		"surface.move",
		"surface.close",
	} {
		if _, ok := caps.Methods[method]; !ok {
			return OpenResult{}, "cmux_capability_missing", fmt.Sprintf("cmux capability missing: %s", method)
		}
	}

	mapping, err := s.NewMappingStore(req.Root).Load()
	if err != nil {
		return OpenResult{}, "state_write_failed", fmt.Sprintf("load cmux mapping: %v", err)
	}
	cmuxWorkspaceID, ok := mappedWorkspaceID(mapping, req.WorkspaceID)
	if !ok {
		return OpenResult{}, "cmux_workspace_not_found", fmt.Sprintf("no mapped cmux workspace for %s", req.WorkspaceID)
	}
	if _, err := client.Identify(ctx, cmuxWorkspaceID, ""); err != nil {
		if isNotFoundError(err) {
			return OpenResult{}, "cmux_workspace_not_found", fmt.Sprintf("mapped cmux workspace is not reachable: %s", cmuxWorkspaceID)
		}
		return OpenResult{}, "cmux_identify_failed", fmt.Sprintf("identify cmux workspace: %v", err)
	}

	docsStore := s.NewDocsStore(req.Root)
	docsFile, err := docsStore.Load()
	if err != nil {
		return OpenResult{}, "state_write_failed", fmt.Sprintf("load cmux docs state: %v", err)
	}

	stage, code, msg := s.ensureStageSlot(ctx, client, req.Root, docsFile.Stage)
	if code != "" {
		return OpenResult{}, code, msg
	}
	docs, err := s.ensureDocsSlot(ctx, client, cmuxWorkspaceID, req.SurfaceHint, docsFile.Workspaces[req.WorkspaceID])
	if err != nil {
		return OpenResult{}, slotErrorCode(err), err.Error()
	}
	afterRef, err := s.moveAfterSurface(ctx, client, cmuxWorkspaceID, docs)
	if err != nil {
		return OpenResult{}, "cmux_list_surfaces_failed", fmt.Sprintf("list docs pane surfaces: %v", err)
	}

	md, err := client.MarkdownOpen(ctx, req.Path, stage.WorkspaceRef, stage.SurfaceRef)
	if err != nil {
		return OpenResult{}, "cmux_markdown_open_failed", fmt.Sprintf("open markdown viewer: %v", err)
	}

	viewerSurfaceRef := strings.TrimSpace(md.SurfaceRef)
	if viewerSurfaceRef == "" {
		return OpenResult{}, "cmux_markdown_open_failed", "markdown viewer returned empty surface ref"
	}

	moved, err := client.MoveSurface(ctx, viewerSurfaceRef, docs.PaneRef, cmuxWorkspaceID, afterRef, req.Focus)
	if err != nil {
		return OpenResult{}, "cmux_move_surface_failed", fmt.Sprintf("move markdown viewer into docs pane: %v", err)
	}
	if strings.TrimSpace(moved.SurfaceRef) != "" {
		viewerSurfaceRef = strings.TrimSpace(moved.SurfaceRef)
	}

	viewerTitle := viewerPrefix + filepath.Base(req.Path)
	if err := client.RenameTab(ctx, cmuxWorkspaceID, viewerSurfaceRef, viewerTitle); err != nil {
		return OpenResult{}, "cmux_rename_tab_failed", fmt.Sprintf("rename markdown tab: %v", err)
	}
	if strings.TrimSpace(docs.BootstrapSurface) != "" {
		if err := client.CloseSurface(ctx, cmuxWorkspaceID, docs.BootstrapSurface); err != nil && !isNotFoundError(err) {
			return OpenResult{}, "cmux_close_surface_failed", fmt.Sprintf("close temporary docs tab: %v", err)
		}
	}

	now := s.Now().UTC().Format(time.RFC3339)
	docsFile.Stage = cmuxdocs.Stage{
		WorkspaceRef: stage.WorkspaceRef,
		PaneRef:      stage.PaneRef,
		SurfaceRef:   stage.SurfaceRef,
		LastUsedAt:   now,
	}
	docsFile.Workspaces[req.WorkspaceID] = cmuxdocs.Slot{
		CMUXWorkspaceID: cmuxWorkspaceID,
		PaneRef:         docs.PaneRef,
		LastViewerRef:   viewerSurfaceRef,
		LastUsedAt:      now,
	}
	if err := docsStore.Save(docsFile); err != nil {
		return OpenResult{}, "state_write_failed", fmt.Sprintf("save cmux docs state: %v", err)
	}

	return OpenResult{
		Path:             req.Path,
		ScopePath:        req.ScopePath,
		CMUXWorkspaceID:  cmuxWorkspaceID,
		DocsPaneRef:      docs.PaneRef,
		ViewerSurfaceRef: viewerSurfaceRef,
		Focused:          req.Focus,
	}, "", ""
}

func (s *Service) ensureStageSlot(ctx context.Context, client Client, root string, current cmuxdocs.Stage) (stageSlot, string, string) {
	workspaceRef, code, msg := s.EnsureStageWorkspace(ctx, root)
	if code != "" {
		return stageSlot{}, code, msg
	}
	if slot, ok := s.reuseStoredStage(ctx, client, workspaceRef, current); ok {
		return slot, "", ""
	}
	slot, err := s.inspectStageWorkspace(ctx, client, workspaceRef)
	if err != nil {
		return stageSlot{}, "cmux_stage_workspace_failed", fmt.Sprintf("inspect root workspace for staging: %v", err)
	}
	return slot, "", ""
}

func (s *Service) reuseStoredStage(ctx context.Context, client Client, workspaceRef string, current cmuxdocs.Stage) (stageSlot, bool) {
	workspaceRef = strings.TrimSpace(workspaceRef)
	if workspaceRef == "" {
		return stageSlot{}, false
	}
	if strings.TrimSpace(current.WorkspaceRef) != "" && strings.TrimSpace(current.WorkspaceRef) != workspaceRef {
		return stageSlot{}, false
	}
	if _, err := client.Identify(ctx, workspaceRef, ""); err != nil {
		return stageSlot{}, false
	}
	if strings.TrimSpace(current.PaneRef) != "" && strings.TrimSpace(current.SurfaceRef) != "" {
		surfaces, err := client.ListPaneSurfaces(ctx, workspaceRef, current.PaneRef)
		if err == nil && containsSurface(surfaces, current.SurfaceRef) {
			return stageSlot{
				WorkspaceRef: workspaceRef,
				PaneRef:      strings.TrimSpace(current.PaneRef),
				SurfaceRef:   strings.TrimSpace(current.SurfaceRef),
			}, true
		}
	}
	slot, err := s.inspectStageWorkspace(ctx, client, workspaceRef)
	if err != nil {
		return stageSlot{}, false
	}
	return slot, true
}

func (s *Service) inspectStageWorkspace(ctx context.Context, client Client, workspaceRef string) (stageSlot, error) {
	panes, err := client.ListPanes(ctx, workspaceRef)
	if err != nil {
		return stageSlot{}, err
	}
	if len(panes) == 0 {
		return stageSlot{}, fmt.Errorf("stage workspace has no panes")
	}
	pane := pickStagePane(panes)
	surfaces, err := client.ListPaneSurfaces(ctx, workspaceRef, pane.Ref)
	if err != nil {
		return stageSlot{}, err
	}
	surfaceRef := pickSurfaceRef(surfaces)
	if surfaceRef == "" {
		return stageSlot{}, fmt.Errorf("stage workspace has no surfaces")
	}
	return stageSlot{
		WorkspaceRef: strings.TrimSpace(workspaceRef),
		PaneRef:      strings.TrimSpace(pane.Ref),
		SurfaceRef:   surfaceRef,
	}, nil
}

func (s *Service) ensureDocsSlot(ctx context.Context, client Client, cmuxWorkspaceID string, surfaceHint string, current cmuxdocs.Slot) (docsSlot, error) {
	if slot, ok := s.reuseStoredSlot(ctx, client, cmuxWorkspaceID, current); ok {
		return slot, nil
	}
	if slot, ok, err := s.rediscoverSlot(ctx, client, cmuxWorkspaceID); err != nil {
		return docsSlot{}, err
	} else if ok {
		return slot, nil
	}

	var created cmuxctl.PaneCreateResult
	var err error
	if strings.TrimSpace(surfaceHint) != "" {
		created, err = client.CreateSplit(ctx, "right", cmuxWorkspaceID, surfaceHint)
	} else {
		created, err = client.CreatePane(ctx, cmuxWorkspaceID, "right")
	}
	if err != nil {
		return docsSlot{}, fmt.Errorf("create docs pane: %w", err)
	}
	if strings.TrimSpace(created.PaneRef) == "" || strings.TrimSpace(created.SurfaceRef) == "" {
		return docsSlot{}, fmt.Errorf("create docs pane: empty pane/surface ref")
	}
	return docsSlot{
		PaneRef:          strings.TrimSpace(created.PaneRef),
		BootstrapSurface: strings.TrimSpace(created.SurfaceRef),
	}, nil
}

func (s *Service) reuseStoredSlot(ctx context.Context, client Client, cmuxWorkspaceID string, current cmuxdocs.Slot) (docsSlot, bool) {
	if strings.TrimSpace(current.CMUXWorkspaceID) != "" && strings.TrimSpace(current.CMUXWorkspaceID) != strings.TrimSpace(cmuxWorkspaceID) {
		return docsSlot{}, false
	}
	if strings.TrimSpace(current.PaneRef) == "" {
		return docsSlot{}, false
	}
	surfaces, err := client.ListPaneSurfaces(ctx, cmuxWorkspaceID, current.PaneRef)
	if err != nil || len(surfaces) == 0 {
		return docsSlot{}, false
	}
	return docsSlot{
		PaneRef:       strings.TrimSpace(current.PaneRef),
		LastViewerRef: pickDocsSurface(surfaces, current.LastViewerRef),
	}, true
}

func (s *Service) rediscoverSlot(ctx context.Context, client Client, cmuxWorkspaceID string) (docsSlot, bool, error) {
	panes, err := client.ListPanes(ctx, cmuxWorkspaceID)
	if err != nil {
		return docsSlot{}, false, fmt.Errorf("list panes: %w", err)
	}
	best := docsSlot{}
	bestCount := 0
	for _, pane := range panes {
		surfaces, err := client.ListPaneSurfaces(ctx, cmuxWorkspaceID, pane.Ref)
		if err != nil {
			return docsSlot{}, false, fmt.Errorf("list pane surfaces: %w", err)
		}
		count := countDocsSurfaces(surfaces)
		if count == 0 {
			continue
		}
		if count > bestCount {
			bestCount = count
			best = docsSlot{
				PaneRef:       strings.TrimSpace(pane.Ref),
				LastViewerRef: pickDocsSurface(surfaces, ""),
			}
		}
	}
	if bestCount == 0 {
		return docsSlot{}, false, nil
	}
	return best, true, nil
}

func (s *Service) moveAfterSurface(ctx context.Context, client Client, cmuxWorkspaceID string, slot docsSlot) (string, error) {
	surfaces, err := client.ListPaneSurfaces(ctx, cmuxWorkspaceID, slot.PaneRef)
	if err != nil {
		return "", err
	}
	if preferred := pickDocsSurface(surfaces, slot.LastViewerRef); preferred != "" {
		return preferred, nil
	}
	if last := lastSurfaceRef(surfaces); last != "" {
		return last, nil
	}
	return "", nil
}

func mappedWorkspaceID(mapping cmuxmap.File, workspaceID string) (string, bool) {
	ws, ok := mapping.Workspaces[workspaceID]
	if !ok || len(ws.Entries) == 0 {
		return "", false
	}
	id := strings.TrimSpace(ws.Entries[0].CMUXWorkspaceID)
	if id == "" {
		return "", false
	}
	return id, true
}

func isNotFoundError(err error) bool {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "not found") || strings.Contains(msg, "not_found") || strings.Contains(msg, "unknown workspace")
}

func slotErrorCode(err error) string {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "list panes"), strings.Contains(msg, "list pane surfaces"):
		return "cmux_list_surfaces_failed"
	case strings.Contains(msg, "create docs pane"):
		return "cmux_docs_pane_failed"
	default:
		return "internal_error"
	}
}

func countDocsSurfaces(surfaces []cmuxctl.Surface) int {
	count := 0
	for _, surface := range surfaces {
		if strings.HasPrefix(strings.TrimSpace(surface.Title), viewerPrefix) {
			count++
		}
	}
	return count
}

func pickDocsSurface(surfaces []cmuxctl.Surface, preferred string) string {
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		for _, surface := range surfaces {
			if strings.TrimSpace(surface.Ref) == preferred && strings.HasPrefix(strings.TrimSpace(surface.Title), viewerPrefix) {
				return preferred
			}
		}
	}
	bestRef := ""
	bestIndex := -1
	for _, surface := range surfaces {
		if !strings.HasPrefix(strings.TrimSpace(surface.Title), viewerPrefix) {
			continue
		}
		if surface.Index >= bestIndex {
			bestIndex = surface.Index
			bestRef = strings.TrimSpace(surface.Ref)
		}
	}
	return bestRef
}

func lastSurfaceRef(surfaces []cmuxctl.Surface) string {
	bestRef := ""
	bestIndex := -1
	for _, surface := range surfaces {
		if surface.Index >= bestIndex {
			bestIndex = surface.Index
			bestRef = strings.TrimSpace(surface.Ref)
		}
	}
	return bestRef
}

func containsSurface(surfaces []cmuxctl.Surface, ref string) bool {
	ref = strings.TrimSpace(ref)
	for _, surface := range surfaces {
		if strings.TrimSpace(surface.Ref) == ref {
			return true
		}
	}
	return false
}

func pickStagePane(panes []cmuxctl.Pane) cmuxctl.Pane {
	best := panes[0]
	for _, pane := range panes[1:] {
		if pane.Focused && !best.Focused {
			best = pane
			continue
		}
		if pane.Focused == best.Focused && pane.Index < best.Index {
			best = pane
		}
	}
	return best
}

func pickSurfaceRef(surfaces []cmuxctl.Surface) string {
	if len(surfaces) == 0 {
		return ""
	}
	best := surfaces[0]
	for _, surface := range surfaces[1:] {
		if surface.Selected && !best.Selected {
			best = surface
			continue
		}
		if surface.Selected == best.Selected && surface.Index < best.Index {
			best = surface
		}
	}
	return strings.TrimSpace(best.Ref)
}
