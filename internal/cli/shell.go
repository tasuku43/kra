package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (c *CLI) runShell(args []string) int {
	if len(args) == 0 {
		c.printShellUsage(c.Err)
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printShellUsage(c.Out)
		return exitOK
	case "init":
		return c.runShellInit(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"shell"}, args[0]), " "))
		c.printShellUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runShellInit(args []string) int {
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for shell init: %q\n", strings.Join(args[1:], " "))
		c.printShellUsage(c.Err)
		return exitUsage
	}

	shellName := ""
	if len(args) == 1 {
		shellName = strings.TrimSpace(args[0])
	} else {
		shellName = detectShellName()
	}
	if shellName == "" {
		shellName = "zsh"
	}

	script, err := renderShellInitScript(shellName)
	if err != nil {
		fmt.Fprintf(c.Err, "render shell init script: %v\n", err)
		return exitUsage
	}
	fmt.Fprint(c.Out, script)
	return exitOK
}

func detectShellName() string {
	raw := strings.TrimSpace(os.Getenv("SHELL"))
	if raw == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(filepath.Base(raw)))
}

func renderShellInitScript(shellName string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(shellName)) {
	case "zsh", "bash", "sh":
		return renderPOSIXShellInitScript(shellName), nil
	case "fish":
		return renderFishShellInitScript(), nil
	default:
		return "", fmt.Errorf("unsupported shell %q (supported: zsh, bash, sh, fish)", shellName)
	}
}

func renderPOSIXShellInitScript(shellName string) string {
	return fmt.Sprintf(`# gionx shell integration (%s)
# Add this line to your shell rc file (~/.%src), then restart shell:
#   eval "$(gionx shell init %s)"
gionx() {
  local __gionx_action_file __gionx_status
  __gionx_action_file="$(mktemp "${TMPDIR:-/tmp}/gionx-action.XXXXXX")" || return 1
  if [ "$1" = "ws" ] && [ "$2" = "go" ]; then
    local __gionx_cd
    __gionx_cd="$(GIONX_SHELL_ACTION_FILE="$__gionx_action_file" command gionx ws go "${@:3}")" || {
      __gionx_status=$?
      rm -f "$__gionx_action_file"
      return $__gionx_status
    }
    eval "$__gionx_cd"
  else
    GIONX_SHELL_ACTION_FILE="$__gionx_action_file" command gionx "$@"
    __gionx_status=$?
    if [ $__gionx_status -ne 0 ]; then
      rm -f "$__gionx_action_file"
      return $__gionx_status
    fi
    if [ -s "$__gionx_action_file" ]; then
      eval "$(cat "$__gionx_action_file")"
    fi
  fi
  rm -f "$__gionx_action_file"
}
`, shellName, shellName, shellName)
}

func renderFishShellInitScript() string {
	return `# gionx shell integration (fish)
# Add this line to your config.fish, then restart shell:
#   eval (gionx shell init fish)
function gionx
  set -l __gionx_action_file (mktemp "/tmp/gionx-action.XXXXXX"); or return 1
  if test (count $argv) -ge 2; and test "$argv[1]" = "ws"; and test "$argv[2]" = "go"
    set -l __gionx_cd (env GIONX_SHELL_ACTION_FILE="$__gionx_action_file" command gionx ws go $argv[3..-1]); or begin
      set -l __gionx_status $status
      rm -f "$__gionx_action_file"
      return $__gionx_status
    end
    eval $__gionx_cd
  else
    env GIONX_SHELL_ACTION_FILE="$__gionx_action_file" command gionx $argv
    set -l __gionx_status $status
    if test $__gionx_status -ne 0
      rm -f "$__gionx_action_file"
      return $__gionx_status
    end
    if test -s "$__gionx_action_file"
      eval (cat "$__gionx_action_file")
    end
  end
  rm -f "$__gionx_action_file"
end
`
}
