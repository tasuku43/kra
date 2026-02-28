package cmuxsession

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/tasuku43/kra/internal/cmuxmap"
	sessionstore "github.com/tasuku43/kra/internal/cmuxsession"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
)

const (
	defaultScreenLines = 120
)

type Warning struct {
	Code    string
	Message string
}

type SaveRequest struct {
	Root                string
	WorkspaceID         string
	Label               string
	IncludeBrowserState bool
	ScreenLines         int
}

type SaveResult struct {
	SessionID         string
	Label             string
	Path              string
	SavedAt           string
	PaneCount         int
	SurfaceCount      int
	BrowserStateSaved bool
	Warnings          []Warning
}

type ResumeRequest struct {
	Root        string
	WorkspaceID string
	SessionID   string
	Strict      bool
	SkipBrowser bool
}

type ResumeResult struct {
	SessionID       string
	SessionLabel    string
	ResumedAt       string
	WorkspaceSelect bool
	FocusRestored   bool
	BrowserRestored bool
	Warnings        []Warning
}

type Client interface {
	Identify(ctx context.Context, workspace string, surface string) (map[string]any, error)
	SelectWorkspace(ctx context.Context, workspace string) error
	ListPanes(ctx context.Context, workspace string) ([]cmuxctl.Pane, error)
	ListPaneSurfaces(ctx context.Context, workspace string, pane string) ([]cmuxctl.Surface, error)
	ReadScreen(ctx context.Context, workspace string, surface string, lines int, scrollback bool) (string, error)
	BrowserStateSave(ctx context.Context, workspace string, surface string, path string) error
	BrowserStateLoad(ctx context.Context, workspace string, surface string, path string) error
	FocusPane(ctx context.Context, pane string, workspace string) error
}

type MappingStore interface {
	Load() (cmuxmap.File, error)
	Save(cmuxmap.File) error
}

type SessionStore interface {
	Load() (sessionstore.File, error)
	Save(sessionstore.File) error
}

type NewClientFunc func() Client
type NewMappingStoreFunc func(root string) MappingStore
type NewSessionStoreFunc func(root string) SessionStore

type Service struct {
	NewClient       NewClientFunc
	NewMappingStore NewMappingStoreFunc
	NewSessionStore NewSessionStoreFunc
	Now             func() time.Time
}

func NewService(newClient NewClientFunc, newMappingStore NewMappingStoreFunc, newSessionStore NewSessionStoreFunc) *Service {
	return &Service{
		NewClient:       newClient,
		NewMappingStore: newMappingStore,
		NewSessionStore: newSessionStore,
		Now:             time.Now,
	}
}

func (s *Service) Save(ctx context.Context, req SaveRequest) (SaveResult, string, string) {
	if s.NewClient == nil || s.NewMappingStore == nil || s.NewSessionStore == nil {
		return SaveResult{}, "internal_error", "cmux session service is not initialized"
	}
	root := strings.TrimSpace(req.Root)
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	if root == "" || workspaceID == "" {
		return SaveResult{}, "invalid_argument", "root and workspace_id are required"
	}
	workspacePath, code, msg := resolveActiveWorkspacePath(root, workspaceID)
	if code != "" {
		return SaveResult{}, code, msg
	}
	mapStore := s.NewMappingStore(root)
	mapping, err := mapStore.Load()
	if err != nil {
		return SaveResult{}, "state_write_failed", fmt.Sprintf("load cmux mapping: %v", err)
	}
	cmuxWorkspaceID := resolveMappedCMUXWorkspaceID(mapping, workspaceID)
	if cmuxWorkspaceID == "" {
		return SaveResult{}, "cmux_not_mapped", fmt.Sprintf("no cmux mapping found for workspace: %s", workspaceID)
	}

	client := s.NewClient()
	if _, err := client.Identify(ctx, cmuxWorkspaceID, ""); err != nil {
		return SaveResult{}, "cmux_runtime_unavailable", fmt.Sprintf("identify cmux workspace: %v", err)
	}
	sessionStore := s.NewSessionStore(root)
	sessionFile, err := sessionStore.Load()
	if err != nil {
		return SaveResult{}, "state_write_failed", fmt.Sprintf("load cmux sessions: %v", err)
	}

	now := s.Now().UTC()
	sessionID := allocateSessionID(sessionFile.Workspaces[workspaceID].Sessions, now, req.Label)
	sessionDir := filepath.Join(workspacePath, "artifacts", "cmux", "sessions", sessionID)
	screenDir := filepath.Join(sessionDir, "screen")
	browserDir := filepath.Join(sessionDir, "browser")
	if err := os.MkdirAll(screenDir, 0o755); err != nil {
		return SaveResult{}, "state_write_failed", fmt.Sprintf("create session screen dir: %v", err)
	}
	includeBrowser := req.IncludeBrowserState
	if includeBrowser {
		if err := os.MkdirAll(browserDir, 0o755); err != nil {
			return SaveResult{}, "state_write_failed", fmt.Sprintf("create session browser dir: %v", err)
		}
	}
	lines := req.ScreenLines
	if lines < 1 {
		lines = defaultScreenLines
	}
	panes, err := client.ListPanes(ctx, cmuxWorkspaceID)
	if err != nil {
		return SaveResult{}, "cmux_runtime_unavailable", fmt.Sprintf("list cmux panes: %v", err)
	}
	warnings := make([]Warning, 0)
	doc := sessionDocument{
		Version:         1,
		SessionID:       sessionID,
		WorkspaceID:     workspaceID,
		CMUXWorkspaceID: cmuxWorkspaceID,
		Label:           strings.TrimSpace(req.Label),
		CreatedAt:       now.Format(time.RFC3339),
		Panes:           make([]sessionDocumentPane, 0, len(panes)),
	}
	totalSurfaceCount := 0
	browserSavedCount := 0

	for _, pane := range panes {
		paneHandle := firstNonEmpty(strings.TrimSpace(pane.ID), strings.TrimSpace(pane.Ref))
		if paneHandle == "" {
			continue
		}
		surfaces, err := client.ListPaneSurfaces(ctx, cmuxWorkspaceID, paneHandle)
		if err != nil {
			warnings = append(warnings, Warning{
				Code:    "surface_list_failed",
				Message: fmt.Sprintf("list pane surfaces (%s): %v", paneHandle, err),
			})
			continue
		}
		docPane := sessionDocumentPane{
			PaneID:   strings.TrimSpace(pane.ID),
			PaneRef:  strings.TrimSpace(pane.Ref),
			Focused:  pane.Focused,
			Surfaces: make([]sessionDocumentSurface, 0, len(surfaces)),
		}
		if pane.Focused && doc.FocusPaneID == "" {
			doc.FocusPaneID = firstNonEmpty(docPane.PaneID, docPane.PaneRef)
		}
		for _, surface := range surfaces {
			totalSurfaceCount++
			surfaceHandle := firstNonEmpty(strings.TrimSpace(surface.ID), strings.TrimSpace(surface.Ref))
			docSurface := sessionDocumentSurface{
				SurfaceID:  strings.TrimSpace(surface.ID),
				SurfaceRef: strings.TrimSpace(surface.Ref),
				Title:      strings.TrimSpace(surface.Title),
				Type:       strings.TrimSpace(surface.Type),
				Selected:   surface.Selected,
			}
			if surfaceHandle == "" {
				warnings = append(warnings, Warning{
					Code:    "surface_capture_failed",
					Message: "surface id/ref is empty",
				})
				docPane.Surfaces = append(docPane.Surfaces, docSurface)
				continue
			}
			screenPathAbs := filepath.Join(screenDir, sanitizePathComponent(surfaceHandle)+".txt")
			text, err := client.ReadScreen(ctx, cmuxWorkspaceID, surfaceHandle, lines, true)
			if err != nil {
				warnings = append(warnings, Warning{
					Code:    "screen_capture_failed",
					Message: fmt.Sprintf("read screen (%s): %v", surfaceHandle, err),
				})
			} else if writeErr := os.WriteFile(screenPathAbs, []byte(text), 0o644); writeErr != nil {
				warnings = append(warnings, Warning{
					Code:    "state_write_failed",
					Message: fmt.Sprintf("write screen snapshot (%s): %v", surfaceHandle, writeErr),
				})
			} else {
				docSurface.ScreenPath = normalizeRelativePath(root, screenPathAbs)
			}

			if includeBrowser && isBrowserSurface(surface) {
				browserPathAbs := filepath.Join(browserDir, sanitizePathComponent(surfaceHandle)+".state.json")
				if err := client.BrowserStateSave(ctx, cmuxWorkspaceID, surfaceHandle, browserPathAbs); err != nil {
					warnings = append(warnings, Warning{
						Code:    "browser_state_save_failed",
						Message: fmt.Sprintf("save browser state (%s): %v", surfaceHandle, err),
					})
				} else {
					docSurface.BrowserStatePath = normalizeRelativePath(root, browserPathAbs)
					browserSavedCount++
				}
			}
			docPane.Surfaces = append(docPane.Surfaces, docSurface)
		}
		doc.Panes = append(doc.Panes, docPane)
	}
	sort.SliceStable(doc.Panes, func(i, j int) bool {
		return firstNonEmpty(doc.Panes[i].PaneID, doc.Panes[i].PaneRef) < firstNonEmpty(doc.Panes[j].PaneID, doc.Panes[j].PaneRef)
	})
	sessionJSONPath := filepath.Join(sessionDir, "session.json")
	if err := writeSessionDocument(sessionJSONPath, doc); err != nil {
		return SaveResult{}, "state_write_failed", fmt.Sprintf("write session metadata: %v", err)
	}
	entry := sessionstore.Entry{
		SessionID:         sessionID,
		Label:             strings.TrimSpace(req.Label),
		CreatedAt:         now.Format(time.RFC3339),
		Path:              normalizeRelativePath(root, sessionDir),
		PaneCount:         len(doc.Panes),
		SurfaceCount:      totalSurfaceCount,
		BrowserStateSaved: browserSavedCount > 0,
	}
	wsSessions := sessionFile.Workspaces[workspaceID]
	wsSessions.Sessions = append(wsSessions.Sessions, entry)
	sessionFile.Workspaces[workspaceID] = wsSessions
	if err := sessionStore.Save(sessionFile); err != nil {
		return SaveResult{}, "state_write_failed", fmt.Sprintf("save cmux sessions: %v", err)
	}
	return SaveResult{
		SessionID:         entry.SessionID,
		Label:             entry.Label,
		Path:              entry.Path,
		SavedAt:           entry.CreatedAt,
		PaneCount:         entry.PaneCount,
		SurfaceCount:      entry.SurfaceCount,
		BrowserStateSaved: entry.BrowserStateSaved,
		Warnings:          warnings,
	}, "", ""
}

func (s *Service) ListSessions(root string, workspaceID string) ([]sessionstore.Entry, string, string) {
	if s.NewSessionStore == nil {
		return nil, "internal_error", "cmux session service is not initialized"
	}
	root = strings.TrimSpace(root)
	workspaceID = strings.TrimSpace(workspaceID)
	if root == "" || workspaceID == "" {
		return nil, "invalid_argument", "root and workspace_id are required"
	}
	sessionFile, err := s.NewSessionStore(root).Load()
	if err != nil {
		return nil, "state_write_failed", fmt.Sprintf("load cmux sessions: %v", err)
	}
	wsSessions := sessionFile.Workspaces[workspaceID].Sessions
	if len(wsSessions) == 0 {
		return []sessionstore.Entry{}, "", ""
	}
	out := make([]sessionstore.Entry, 0, len(wsSessions))
	out = append(out, wsSessions...)
	return out, "", ""
}

func (s *Service) Resume(ctx context.Context, req ResumeRequest) (ResumeResult, string, string) {
	if s.NewClient == nil || s.NewMappingStore == nil || s.NewSessionStore == nil {
		return ResumeResult{}, "internal_error", "cmux session service is not initialized"
	}
	root := strings.TrimSpace(req.Root)
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	sessionID := strings.TrimSpace(req.SessionID)
	if root == "" || workspaceID == "" || sessionID == "" {
		return ResumeResult{}, "invalid_argument", "root, workspace_id, and session_id are required"
	}
	_, code, msg := resolveActiveWorkspacePath(root, workspaceID)
	if code != "" {
		return ResumeResult{}, code, msg
	}
	mapping, err := s.NewMappingStore(root).Load()
	if err != nil {
		return ResumeResult{}, "state_write_failed", fmt.Sprintf("load cmux mapping: %v", err)
	}
	cmuxWorkspaceID := resolveMappedCMUXWorkspaceID(mapping, workspaceID)
	if cmuxWorkspaceID == "" {
		return ResumeResult{}, "cmux_not_mapped", fmt.Sprintf("no cmux mapping found for workspace: %s", workspaceID)
	}
	sessionFile, err := s.NewSessionStore(root).Load()
	if err != nil {
		return ResumeResult{}, "state_write_failed", fmt.Sprintf("load cmux sessions: %v", err)
	}
	entry, ok := findSessionEntry(sessionFile, workspaceID, sessionID)
	if !ok {
		return ResumeResult{}, "session_not_found", fmt.Sprintf("session not found: %s", sessionID)
	}
	doc, err := loadSessionDocument(root, entry.Path)
	if err != nil {
		return ResumeResult{}, "session_not_found", fmt.Sprintf("load session metadata: %v", err)
	}

	client := s.NewClient()
	if err := client.SelectWorkspace(ctx, cmuxWorkspaceID); err != nil {
		return ResumeResult{}, "cmux_runtime_unavailable", fmt.Sprintf("select cmux workspace: %v", err)
	}
	warnings := make([]Warning, 0)
	focusRestored := false
	if strings.TrimSpace(doc.FocusPaneID) != "" {
		if err := client.FocusPane(ctx, doc.FocusPaneID, cmuxWorkspaceID); err != nil {
			warnings = append(warnings, Warning{
				Code:    "focus_restore_failed",
				Message: fmt.Sprintf("focus pane (%s): %v", doc.FocusPaneID, err),
			})
		} else {
			focusRestored = true
		}
	}

	browserRestored := false
	if !req.SkipBrowser {
		for _, pane := range doc.Panes {
			for _, surface := range pane.Surfaces {
				if strings.TrimSpace(surface.BrowserStatePath) == "" {
					continue
				}
				surfaceHandle := firstNonEmpty(strings.TrimSpace(surface.SurfaceID), strings.TrimSpace(surface.SurfaceRef))
				if surfaceHandle == "" {
					warnings = append(warnings, Warning{
						Code:    "browser_state_restore_failed",
						Message: "surface id/ref is empty for browser restore",
					})
					continue
				}
				statePath := resolveStoredPath(root, surface.BrowserStatePath)
				if err := client.BrowserStateLoad(ctx, cmuxWorkspaceID, surfaceHandle, statePath); err != nil {
					warnings = append(warnings, Warning{
						Code:    "browser_state_restore_failed",
						Message: fmt.Sprintf("load browser state (%s): %v", surfaceHandle, err),
					})
					continue
				}
				browserRestored = true
			}
		}
	}

	if req.Strict && len(warnings) > 0 {
		return ResumeResult{
			SessionID:       entry.SessionID,
			SessionLabel:    entry.Label,
			ResumedAt:       s.Now().UTC().Format(time.RFC3339),
			WorkspaceSelect: true,
			FocusRestored:   focusRestored,
			BrowserRestored: browserRestored,
			Warnings:        warnings,
		}, "session_restore_partial", "resume completed with unresolved restore items in strict mode"
	}

	return ResumeResult{
		SessionID:       entry.SessionID,
		SessionLabel:    entry.Label,
		ResumedAt:       s.Now().UTC().Format(time.RFC3339),
		WorkspaceSelect: true,
		FocusRestored:   focusRestored,
		BrowserRestored: browserRestored,
		Warnings:        warnings,
	}, "", ""
}

type sessionDocument struct {
	Version         int                   `json:"version"`
	SessionID       string                `json:"session_id"`
	WorkspaceID     string                `json:"workspace_id"`
	CMUXWorkspaceID string                `json:"cmux_workspace_id"`
	Label           string                `json:"label,omitempty"`
	CreatedAt       string                `json:"created_at"`
	FocusPaneID     string                `json:"focus_pane_id,omitempty"`
	Panes           []sessionDocumentPane `json:"panes"`
}

type sessionDocumentPane struct {
	PaneID   string                   `json:"pane_id,omitempty"`
	PaneRef  string                   `json:"pane_ref,omitempty"`
	Focused  bool                     `json:"focused,omitempty"`
	Surfaces []sessionDocumentSurface `json:"surfaces"`
}

type sessionDocumentSurface struct {
	SurfaceID        string `json:"surface_id,omitempty"`
	SurfaceRef       string `json:"surface_ref,omitempty"`
	Title            string `json:"title,omitempty"`
	Type             string `json:"type,omitempty"`
	Selected         bool   `json:"selected,omitempty"`
	ScreenPath       string `json:"screen_path,omitempty"`
	BrowserStatePath string `json:"browser_state_path,omitempty"`
}

func writeSessionDocument(path string, doc sessionDocument) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session document: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write session document: %w", err)
	}
	return nil
}

func loadSessionDocument(root string, sessionPath string) (sessionDocument, error) {
	sessionDir := resolveStoredPath(root, sessionPath)
	path := filepath.Join(sessionDir, "session.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return sessionDocument{}, err
	}
	var doc sessionDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return sessionDocument{}, fmt.Errorf("parse session document: %w", err)
	}
	return doc, nil
}

func resolveActiveWorkspacePath(root string, workspaceID string) (string, string, string) {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	if fi, err := os.Stat(wsPath); err == nil && fi.IsDir() {
		return wsPath, "", ""
	}
	archivedPath := filepath.Join(root, "archive", workspaceID)
	if fi, err := os.Stat(archivedPath); err == nil && fi.IsDir() {
		return "", "workspace_not_active", fmt.Sprintf("workspace is archived: %s", workspaceID)
	}
	return "", "workspace_not_found", fmt.Sprintf("workspace not found: %s", workspaceID)
}

func resolveMappedCMUXWorkspaceID(mapping cmuxmap.File, workspaceID string) string {
	ws, ok := mapping.Workspaces[workspaceID]
	if !ok || len(ws.Entries) == 0 {
		return ""
	}
	return strings.TrimSpace(ws.Entries[0].CMUXWorkspaceID)
}

func findSessionEntry(file sessionstore.File, workspaceID string, sessionID string) (sessionstore.Entry, bool) {
	ws, ok := file.Workspaces[workspaceID]
	if !ok {
		return sessionstore.Entry{}, false
	}
	for _, entry := range ws.Sessions {
		if strings.TrimSpace(entry.SessionID) == sessionID {
			return entry, true
		}
	}
	return sessionstore.Entry{}, false
}

func allocateSessionID(existing []sessionstore.Entry, now time.Time, label string) string {
	base := now.UTC().Format("20060102T150405Z")
	if slug := slugifyLabel(label); slug != "" {
		base += "-" + slug
	}
	seen := map[string]struct{}{}
	for _, e := range existing {
		seen[strings.TrimSpace(e.SessionID)] = struct{}{}
	}
	id := base
	if _, ok := seen[id]; !ok {
		return id
	}
	for i := 2; ; i++ {
		id = fmt.Sprintf("%s-%d", base, i)
		if _, ok := seen[id]; !ok {
			return id
		}
	}
}

func slugifyLabel(label string) string {
	label = strings.TrimSpace(strings.ToLower(label))
	if label == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range label {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return ""
	}
	return out
}

func normalizeRelativePath(root string, abs string) string {
	if rel, err := filepath.Rel(root, abs); err == nil && rel != "" && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

func resolveStoredPath(root string, stored string) string {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return root
	}
	if filepath.IsAbs(stored) {
		return filepath.Clean(stored)
	}
	return filepath.Clean(filepath.Join(root, filepath.FromSlash(stored)))
}

func sanitizePathComponent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		switch r {
		case '.', '-', '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "unknown"
	}
	return out
}

func isBrowserSurface(s cmuxctl.Surface) bool {
	return strings.EqualFold(strings.TrimSpace(s.Type), "browser")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}
