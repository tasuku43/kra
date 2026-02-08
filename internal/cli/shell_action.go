package cli

import (
	"fmt"
	"os"
	"strings"
)

const shellActionFileEnv = "GIONX_SHELL_ACTION_FILE"

func writeShellActionCD(path string) error {
	actionPath := strings.TrimSpace(os.Getenv(shellActionFileEnv))
	if actionPath == "" {
		return nil
	}
	line := fmt.Sprintf("cd %s\n", shellSingleQuote(path))
	return os.WriteFile(actionPath, []byte(line), 0o600)
}
