package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
)

const (
	wsCloseJournalVersion   = 1
	wsCloseJournalOperation = "ws-close"

	wsClosePhaseRiskChecked       = "risk_checked"
	wsClosePhaseClosePreCommitted = "close_pre_committed"
	wsClosePhaseWorktreesRemoved  = "worktrees_removed"
	wsClosePhaseMetadataArchived  = "metadata_archived"
	wsClosePhaseWorkspaceRenamed  = "workspace_renamed"
	wsClosePhaseArchiveCommitted  = "archive_committed"
	wsClosePhaseCompleted         = "completed"
)

var wsClosePhaseOrder = map[string]int{
	wsClosePhaseRiskChecked:       1,
	wsClosePhaseClosePreCommitted: 2,
	wsClosePhaseWorktreesRemoved:  3,
	wsClosePhaseMetadataArchived:  4,
	wsClosePhaseWorkspaceRenamed:  5,
	wsClosePhaseArchiveCommitted:  6,
	wsClosePhaseCompleted:         7,
}

type wsCloseLifecycleJournal struct {
	Version              int    `json:"version"`
	Operation            string `json:"operation"`
	WorkspaceID          string `json:"workspace_id"`
	StartedAt            int64  `json:"started_at"`
	UpdatedAt            int64  `json:"updated_at"`
	Phase                string `json:"phase"`
	CommitEnabled        bool   `json:"commit_enabled"`
	ClosePreCommitSHA    string `json:"close_pre_commit_sha,omitempty"`
	ArchiveCommitSHA     string `json:"archive_commit_sha,omitempty"`
	WorkspacePathPresent bool   `json:"workspace_path_present"`
	ArchivePathPresent   bool   `json:"archive_path_present"`
}

var commitArchiveChangeFn = commitArchiveChange

func wsCloseJournalDir(root string) string {
	return filepath.Join(root, ".kra", "state", "operations", "ws-close")
}

func wsCloseJournalPath(root string, workspaceID string) string {
	return filepath.Join(wsCloseJournalDir(root), workspaceID+".json")
}

func newWSCloseLifecycleJournal(workspaceID string, commitEnabled bool, now int64) wsCloseLifecycleJournal {
	if now <= 0 {
		now = time.Now().Unix()
	}
	return wsCloseLifecycleJournal{
		Version:              wsCloseJournalVersion,
		Operation:            wsCloseJournalOperation,
		WorkspaceID:          workspaceID,
		StartedAt:            now,
		UpdatedAt:            now,
		Phase:                wsClosePhaseRiskChecked,
		CommitEnabled:        commitEnabled,
		WorkspacePathPresent: true,
		ArchivePathPresent:   false,
	}
}

func loadWSCloseLifecycleJournal(root string, workspaceID string) (wsCloseLifecycleJournal, error) {
	return loadWSCloseLifecycleJournalFile(wsCloseJournalPath(root, workspaceID))
}

func loadWSCloseLifecycleJournalFile(path string) (wsCloseLifecycleJournal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return wsCloseLifecycleJournal{}, err
	}
	var journal wsCloseLifecycleJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return wsCloseLifecycleJournal{}, fmt.Errorf("parse ws close journal %s: %w", path, err)
	}
	if err := normalizeWSCloseLifecycleJournal(&journal, path); err != nil {
		return wsCloseLifecycleJournal{}, err
	}
	return journal, nil
}

func normalizeWSCloseLifecycleJournal(journal *wsCloseLifecycleJournal, source string) error {
	if journal == nil {
		return fmt.Errorf("ws close journal is required")
	}
	if journal.Version != wsCloseJournalVersion {
		return fmt.Errorf("unsupported ws close journal version in %s: %d", source, journal.Version)
	}
	if strings.TrimSpace(journal.Operation) != wsCloseJournalOperation {
		return fmt.Errorf("unsupported ws close journal operation in %s: %q", source, journal.Operation)
	}
	if err := validateWorkspaceID(strings.TrimSpace(journal.WorkspaceID)); err != nil {
		return fmt.Errorf("invalid ws close journal workspace id in %s: %w", source, err)
	}
	if _, ok := wsClosePhaseOrder[strings.TrimSpace(journal.Phase)]; !ok {
		return fmt.Errorf("unsupported ws close journal phase in %s: %q", source, journal.Phase)
	}
	return nil
}

func saveWSCloseLifecycleJournal(root string, journal wsCloseLifecycleJournal) error {
	if err := normalizeWSCloseLifecycleJournal(&journal, wsCloseJournalPath(root, journal.WorkspaceID)); err != nil {
		return err
	}
	if err := os.MkdirAll(wsCloseJournalDir(root), 0o755); err != nil {
		return fmt.Errorf("create ws close journal dir: %w", err)
	}
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ws close journal: %w", err)
	}
	data = append(data, '\n')
	if err := writeAtomic(wsCloseJournalPath(root, journal.WorkspaceID), data, 0o644); err != nil {
		return fmt.Errorf("write ws close journal: %w", err)
	}
	return nil
}

func removeWSCloseLifecycleJournal(root string, workspaceID string) error {
	if err := os.Remove(wsCloseJournalPath(root, workspaceID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove ws close journal: %w", err)
	}
	return nil
}

func (journal *wsCloseLifecycleJournal) advance(nextPhase string, now int64) error {
	if journal == nil {
		return fmt.Errorf("ws close journal is required")
	}
	currentOrder, ok := wsClosePhaseOrder[strings.TrimSpace(journal.Phase)]
	if !ok {
		return fmt.Errorf("unsupported current ws close phase: %q", journal.Phase)
	}
	nextOrder, ok := wsClosePhaseOrder[strings.TrimSpace(nextPhase)]
	if !ok {
		return fmt.Errorf("unsupported next ws close phase: %q", nextPhase)
	}
	if nextOrder < currentOrder {
		return fmt.Errorf("ws close phase regression: %s -> %s", journal.Phase, nextPhase)
	}
	journal.Phase = nextPhase
	if now <= 0 {
		now = time.Now().Unix()
	}
	journal.UpdatedAt = now
	return nil
}

func findUnfinishedWSCloseJournal(root string, workspaceID string) (wsCloseLifecycleJournal, bool, error) {
	journal, err := loadWSCloseLifecycleJournal(root, workspaceID)
	if err != nil {
		if os.IsNotExist(err) {
			return wsCloseLifecycleJournal{}, false, nil
		}
		return wsCloseLifecycleJournal{}, false, err
	}
	if journal.Phase == wsClosePhaseCompleted {
		return journal, false, nil
	}
	return journal, true, nil
}

func ensureNoUnfinishedWSCloseJournal(root string, workspaceID string) error {
	journal, exists, err := findUnfinishedWSCloseJournal(root, workspaceID)
	if err != nil {
		return fmt.Errorf("inspect ws close journal: %w", err)
	}
	if !exists {
		return nil
	}
	return fmt.Errorf("unfinished ws close recovery exists for %s (phase=%s); run kra doctor --fix --plan", workspaceID, journal.Phase)
}

func canResumeWSCloseLifecycleJournal(root string, journal wsCloseLifecycleJournal) (bool, string) {
	if journal.Phase != wsClosePhaseWorkspaceRenamed && journal.Phase != wsClosePhaseArchiveCommitted {
		return false, fmt.Sprintf("phase %s is not resumable", journal.Phase)
	}
	if journal.WorkspacePathPresent || !journal.ArchivePathPresent {
		return false, "journal path state does not describe a post-rename archive state"
	}

	wsPath := filepath.Join(root, "workspaces", journal.WorkspaceID)
	if fi, err := os.Stat(wsPath); err == nil && fi.IsDir() {
		return false, "workspace path still exists"
	} else if err != nil && !os.IsNotExist(err) {
		return false, fmt.Sprintf("stat workspace path: %v", err)
	}

	archivePath := filepath.Join(root, "archive", journal.WorkspaceID)
	fi, err := os.Stat(archivePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "archive path is missing"
		}
		return false, fmt.Sprintf("stat archive path: %v", err)
	}
	if !fi.IsDir() {
		return false, "archive path is not a directory"
	}
	meta, err := loadWorkspaceMetaFile(archivePath)
	if err != nil {
		return false, fmt.Sprintf("load archived workspace meta: %v", err)
	}
	if strings.TrimSpace(meta.Workspace.Status) != "archived" {
		return false, fmt.Sprintf("archived workspace meta status=%q", strings.TrimSpace(meta.Workspace.Status))
	}
	return true, ""
}

func canResetWSCloseLifecycleJournal(root string, journal wsCloseLifecycleJournal) (bool, string) {
	if journal.Phase != wsClosePhaseRiskChecked {
		return false, fmt.Sprintf("phase %s is not resettable", journal.Phase)
	}
	if !journal.WorkspacePathPresent || journal.ArchivePathPresent {
		return false, "journal path state does not describe a pre-rename workspace state"
	}

	wsPath := filepath.Join(root, "workspaces", journal.WorkspaceID)
	fi, err := os.Stat(wsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "workspace path is missing"
		}
		return false, fmt.Sprintf("stat workspace path: %v", err)
	}
	if !fi.IsDir() {
		return false, "workspace path is not a directory"
	}

	archivePath := filepath.Join(root, "archive", journal.WorkspaceID)
	if fi, err := os.Stat(archivePath); err == nil {
		if fi.IsDir() {
			return false, "archive path already exists"
		}
		return false, "archive path is not a directory"
	} else if !os.IsNotExist(err) {
		return false, fmt.Sprintf("stat archive path: %v", err)
	}

	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		return false, fmt.Sprintf("load active workspace meta: %v", err)
	}
	if strings.TrimSpace(meta.Workspace.Status) != "active" {
		return false, fmt.Sprintf("active workspace meta status=%q", strings.TrimSpace(meta.Workspace.Status))
	}
	return true, ""
}

func resumeWSCloseLifecycleJournal(ctx context.Context, root string, journal wsCloseLifecycleJournal, debugf func(string, ...any)) error {
	if ok, reason := canResumeWSCloseLifecycleJournal(root, journal); !ok {
		return fmt.Errorf("ws close journal requires manual recovery: %s", reason)
	}

	if journal.Phase == wsClosePhaseWorkspaceRenamed {
		if journal.CommitEnabled {
			archivePath := filepath.Join(root, "archive", journal.WorkspaceID)
			expectedFiles, err := listWorkspaceNonRepoFiles(archivePath)
			if err != nil {
				return fmt.Errorf("list archived workspace files for resume: %w", err)
			}
			postSHA, err := commitArchiveChangeFn(ctx, root, journal.WorkspaceID, expectedFiles)
			if err != nil {
				return fmt.Errorf("commit archive change during resume: %w", err)
			}
			journal.ArchiveCommitSHA = postSHA
		}
		if err := journal.advance(wsClosePhaseArchiveCommitted, time.Now().Unix()); err != nil {
			return err
		}
		if err := saveWSCloseLifecycleJournal(root, journal); err != nil {
			return err
		}
	}

	closeMappedCMUXWorkspacesBestEffort(ctx, root, journal.WorkspaceID, debugf)
	if err := journal.advance(wsClosePhaseCompleted, time.Now().Unix()); err != nil {
		return err
	}
	if err := saveWSCloseLifecycleJournal(root, journal); err != nil {
		return err
	}
	if err := removeWSCloseLifecycleJournal(root, journal.WorkspaceID); err != nil {
		return err
	}
	return nil
}

func closeMappedCMUXWorkspacesBestEffort(ctx context.Context, root string, workspaceID string, debugf func(string, ...any)) {
	store := cmuxmap.NewStore(root)
	mapping, err := store.Load()
	if err != nil {
		if debugf != nil {
			debugf("ws close cmux mapping load skipped workspace=%s err=%v", workspaceID, err)
		}
		return
	}
	ws, ok := mapping.Workspaces[workspaceID]
	if !ok || len(ws.Entries) == 0 {
		return
	}
	client := newCMUXCloseClient()
	if client == nil {
		if debugf != nil {
			debugf("ws close cmux close skipped workspace=%s err=nil client", workspaceID)
		}
		return
	}

	nonRecoverableErr := false
	for _, entry := range ws.Entries {
		cmuxWorkspaceID := strings.TrimSpace(entry.CMUXWorkspaceID)
		if cmuxWorkspaceID == "" {
			continue
		}
		if err := client.CloseWorkspace(ctx, cmuxWorkspaceID); err != nil {
			if isCMUXWorkspaceNotFoundError(err) {
				if debugf != nil {
					debugf("ws close cmux workspace already absent workspace=%s cmux=%s", workspaceID, cmuxWorkspaceID)
				}
				continue
			}
			nonRecoverableErr = true
			if debugf != nil {
				debugf("ws close cmux workspace close failed workspace=%s cmux=%s err=%v", workspaceID, cmuxWorkspaceID, err)
			}
			continue
		}
		if debugf != nil {
			debugf("ws close cmux workspace closed workspace=%s cmux=%s", workspaceID, cmuxWorkspaceID)
		}
	}
	if nonRecoverableErr {
		if debugf != nil {
			debugf("ws close cmux mapping kept due close errors workspace=%s", workspaceID)
		}
		return
	}

	delete(mapping.Workspaces, workspaceID)
	if err := store.Save(mapping); err != nil && debugf != nil {
		debugf("ws close cmux mapping save skipped workspace=%s err=%v", workspaceID, err)
	}
}

func detectLegacyHalfClosedWSClose(root string, workspaceID string) (bool, string) {
	if _, err := os.Stat(wsCloseJournalPath(root, workspaceID)); err == nil {
		return false, ""
	}
	archivePath := filepath.Join(root, "archive", workspaceID)
	meta, err := loadWorkspaceMetaFile(archivePath)
	if err != nil {
		return false, ""
	}
	if strings.TrimSpace(meta.Workspace.Status) != "archived" {
		return false, ""
	}
	if fi, err := os.Stat(filepath.Join(root, "workspaces", workspaceID)); err == nil && fi.IsDir() {
		return false, ""
	}
	ctx := context.Background()
	if err := ensureRootGitWorktree(ctx, root); err != nil {
		return false, ""
	}
	subj, err := latestGitSubject(ctx, root)
	if err != nil {
		return false, ""
	}
	if subj != fmt.Sprintf("close-pre: %s", workspaceID) {
		return false, ""
	}
	status, err := gitStatusShortPaths(ctx, root, filepath.ToSlash(filepath.Join("archive", workspaceID)), filepath.ToSlash(filepath.Join("workspaces", workspaceID)))
	if err != nil || strings.TrimSpace(status) == "" {
		return false, ""
	}
	return true, "archive rename appears to have completed before lifecycle journal support; finish archive commit manually"
}

func workspaceIDFromWSCloseJournalTarget(root string, target string) (string, error) {
	expectedDir := filepath.Clean(wsCloseJournalDir(root))
	if filepath.Clean(filepath.Dir(target)) != expectedDir {
		return "", fmt.Errorf("ws close journal target is outside expected directory: %s", target)
	}
	base := strings.TrimSpace(filepath.Base(target))
	if !strings.HasSuffix(base, ".json") {
		return "", fmt.Errorf("ws close journal target must be a json file: %s", target)
	}
	workspaceID := strings.TrimSuffix(base, ".json")
	if err := validateWorkspaceID(workspaceID); err != nil {
		return "", fmt.Errorf("invalid workspace id from ws close journal target: %w", err)
	}
	return workspaceID, nil
}
