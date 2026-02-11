package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/gionx/internal/infra/paths"
)

func ensureGlobalConfigScaffold() error {
	configPath, err := paths.ConfigPath()
	if err != nil {
		return fmt.Errorf("resolve global config path: %w", err)
	}
	if info, err := os.Stat(configPath); err == nil {
		if info.IsDir() {
			return fmt.Errorf("global config path is a directory: %s", configPath)
		}
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat global config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create global config dir: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(defaultGlobalConfigContent()), 0o644); err != nil {
		return fmt.Errorf("write global config: %w", err)
	}
	return nil
}

func defaultGlobalConfigContent() string {
	return `# gionx global config
# Precedence (high -> low):
#   1) CLI flags
#   2) root config: <GIONX_ROOT>/.gionx/config.yaml
#   3) this file: ~/.gionx/config.yaml
#   4) built-in defaults
#
# Empty string values are treated as unset.

workspace:
  # defaults:
  #   template: default

integration:
  jira:
    # defaults:
    #   space: SRE
    #   type: sprint # sprint | jql
`
}

func shouldBootstrapGlobalConfig(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "init":
		return !containsHelpArg(args[1:])
	case "context":
		if len(args) < 2 {
			return false
		}
		if containsHelpArg(args[1:]) {
			return false
		}
		switch args[1] {
		case "create", "use", "rename", "rm", "remove":
			return true
		default:
			return false
		}
	case "repo":
		if len(args) < 2 {
			return false
		}
		if containsHelpArg(args[1:]) {
			return false
		}
		switch args[1] {
		case "add", "discover", "remove", "gc":
			return true
		default:
			return false
		}
	case "ws":
		if len(args) < 2 {
			return false
		}
		if containsHelpArg(args[1:]) {
			return false
		}
		if args[1] == "create" {
			return true
		}
		action := parseWSAct(args[1:])
		switch action {
		case "add-repo", "close", "reopen", "purge":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func parseWSAct(args []string) string {
	rest := append([]string{}, args...)
	for len(rest) > 0 {
		cur := strings.TrimSpace(rest[0])
		switch {
		case cur == "--act":
			if len(rest) < 2 {
				return ""
			}
			return strings.TrimSpace(rest[1])
		case strings.HasPrefix(cur, "--act="):
			return strings.TrimSpace(strings.TrimPrefix(cur, "--act="))
		case strings.HasPrefix(cur, "-"):
			if cur == "--id" || cur == "--format" {
				if len(rest) < 2 {
					return ""
				}
				rest = rest[2:]
				continue
			}
			rest = rest[1:]
		default:
			return ""
		}
	}
	return ""
}

func containsHelpArg(args []string) bool {
	for _, arg := range args {
		switch strings.TrimSpace(arg) {
		case "-h", "--help", "help":
			return true
		}
	}
	return false
}
