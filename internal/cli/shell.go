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
  if [ "$1" = "ws" ] && [ "$2" = "go" ]; then
    local __gionx_cd
    __gionx_cd="$(command gionx ws go "${@:3}")" || return $?
    eval "$__gionx_cd"
  else
    command gionx "$@"
  fi
}
`, shellName, shellName, shellName)
}

func renderFishShellInitScript() string {
	return `# gionx shell integration (fish)
# Add this line to your config.fish, then restart shell:
#   eval (gionx shell init fish)
function gionx
  if test (count $argv) -ge 2; and test "$argv[1]" = "ws"; and test "$argv[2]" = "go"
    set -l __gionx_cd (command gionx ws go $argv[3..-1]); or return $status
    eval $__gionx_cd
  else
    command gionx $argv
  end
end
`
}
