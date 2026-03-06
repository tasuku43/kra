package cli

import "github.com/tasuku43/kra/internal/app/workspacemeta"

const workspaceMetaFilename = workspacemeta.FileName

type workspaceMetaFile = workspacemeta.File
type workspaceMetaWorkspace = workspacemeta.Workspace
type workspaceMetaRepoRestore = workspacemeta.RepoRestore
type workspaceMetaProtection = workspacemeta.Protection
type workspaceMetaPurgeGuard = workspacemeta.PurgeGuard
type workspaceBaseline = workspacemeta.Baseline
type workspaceBaselineRepo = workspacemeta.BaselineRepo

func newWorkspaceMetaFileForCreate(id string, title string, sourceURL string, now int64) workspaceMetaFile {
	return workspacemeta.NewForCreate(id, title, sourceURL, now)
}

func writeWorkspaceMetaFile(wsPath string, meta workspaceMetaFile) error {
	return workspacemeta.Write(wsPath, meta)
}

func loadWorkspaceMetaFile(wsPath string) (workspaceMetaFile, error) {
	return workspacemeta.Load(wsPath)
}

func setWorkspaceMetaWorkState(wsPath string, next workspaceWorkState, now int64) (bool, error) {
	return workspacemeta.SetWorkState(wsPath, string(next), now)
}

func upsertWorkspaceMetaReposRestore(wsPath string, repos []workspaceMetaRepoRestore, now int64) error {
	return workspacemeta.UpsertReposRestore(wsPath, repos, now)
}

func workspaceMetaPurgeGuardEnabled(meta workspaceMetaFile) bool {
	return workspacemeta.PurgeGuardEnabled(meta)
}

func setWorkspaceMetaPurgeGuard(meta *workspaceMetaFile, enabled bool, now int64) {
	workspacemeta.SetPurgeGuard(meta, enabled, now)
}

func removeWorkspaceMetaReposRestoreByAlias(wsPath string, aliases []string, now int64) error {
	return workspacemeta.RemoveReposRestoreByAlias(wsPath, aliases, now)
}
