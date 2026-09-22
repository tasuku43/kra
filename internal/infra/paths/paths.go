package paths

import base "github.com/tasuku43/kra/internal/paths"

func ResolveExistingRoot(cwd string) (string, error) { return base.ResolveExistingRoot(cwd) }
func RepoPoolPath(root string) string                { return base.RepoPoolPath(root) }
func LegacyRepoPoolPath() (string, error)            { return base.LegacyRepoPoolPath() }
func ConfigPath() (string, error)                    { return base.ConfigPath() }
func RootConfigPath(root string) string              { return base.RootConfigPath(root) }
func WriteCurrentContext(root string) error          { return base.WriteCurrentContext(root) }
func ReadCurrentContext() (string, bool, error)      { return base.ReadCurrentContext() }
