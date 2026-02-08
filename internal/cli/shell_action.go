package cli

import (
	"os"
	"strings"

	appshell "github.com/tasuku43/gionx/internal/app/shellaction"
)

const shellActionFileEnv = "GIONX_SHELL_ACTION_FILE"

type shellActionAdapter struct{}

func (a shellActionAdapter) WriteActionLine(line string) error {
	actionPath := strings.TrimSpace(os.Getenv(shellActionFileEnv))
	if actionPath == "" {
		return nil
	}
	return os.WriteFile(actionPath, []byte(line), 0o600)
}

func emitShellActionCD(path string) error {
	svc := appshell.NewService(shellActionAdapter{})
	return svc.EmitCD(path)
}
