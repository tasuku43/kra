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
	case "completion":
		return c.runShellCompletion(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"shell"}, args[0]), " "))
		c.printShellUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runShellInit(args []string) int {
	shellName := ""
	withCompletion := false
	rest := append([]string{}, args...)
	for len(rest) > 0 {
		cur := strings.TrimSpace(rest[0])
		switch cur {
		case "-h", "--help", "help":
			c.printShellUsage(c.Out)
			return exitOK
		case "--with-completion":
			withCompletion = true
			rest = rest[1:]
		default:
			if strings.HasPrefix(cur, "--with-completion=") {
				value := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(cur, "--with-completion=")))
				switch value {
				case "1", "true", "yes", "on":
					withCompletion = true
				case "0", "false", "no", "off":
					withCompletion = false
				default:
					fmt.Fprintf(c.Err, "invalid --with-completion value: %q (supported: true/false)\n", value)
					c.printShellUsage(c.Err)
					return exitUsage
				}
				rest = rest[1:]
				continue
			}
			if strings.HasPrefix(cur, "-") {
				fmt.Fprintf(c.Err, "unknown flag for shell init: %q\n", cur)
				c.printShellUsage(c.Err)
				return exitUsage
			}
			if shellName != "" {
				fmt.Fprintf(c.Err, "unexpected args for shell init: %q\n", strings.Join(rest, " "))
				c.printShellUsage(c.Err)
				return exitUsage
			}
			shellName = cur
			rest = rest[1:]
		}
	}

	if shellName == "" {
		shellName = detectShellName()
	}
	if shellName == "" {
		shellName = "zsh"
	}

	script, err := renderShellInitScript(shellName, withCompletion)
	if err != nil {
		fmt.Fprintf(c.Err, "render shell init script: %v\n", err)
		return exitUsage
	}
	fmt.Fprint(c.Out, script)
	return exitOK
}

func (c *CLI) runShellCompletion(args []string) int {
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for shell completion: %q\n", strings.Join(args[1:], " "))
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

	script, err := renderShellCompletionScript(shellName)
	if err != nil {
		fmt.Fprintf(c.Err, "render shell completion script: %v\n", err)
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

func renderShellInitScript(shellName string, withCompletion bool) (string, error) {
	var initScript string
	switch strings.ToLower(strings.TrimSpace(shellName)) {
	case "zsh", "bash", "sh":
		initScript = renderPOSIXShellInitScript(shellName)
	case "fish":
		initScript = renderFishShellInitScript()
	default:
		return "", fmt.Errorf("unsupported shell %q (supported: zsh, bash, sh, fish)", shellName)
	}

	if !withCompletion {
		return initScript, nil
	}
	completionScript, err := renderShellCompletionScript(shellName)
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(initScript, "\n") {
		return initScript + "\n" + completionScript, nil
	}
	return initScript + "\n\n" + completionScript, nil
}

func renderPOSIXShellInitScript(shellName string) string {
	return fmt.Sprintf(`# kra shell integration (%s)
# Add this line to your shell rc file (~/.%src), then restart shell:
#   eval "$(kra shell init %s)"
kra() {
  local __kra_action_file __kra_status
  __kra_action_file="$(mktemp "${TMPDIR:-/tmp}/kra-action.XXXXXX")" || return 1
  KRA_SHELL_ACTION_FILE="$__kra_action_file" command kra "$@"
  __kra_status=$?
  if [ $__kra_status -ne 0 ]; then
    rm -f "$__kra_action_file"
    return $__kra_status
  fi
  if [ -s "$__kra_action_file" ]; then
    eval "$(cat "$__kra_action_file")"
  fi
  rm -f "$__kra_action_file"
}
`, shellName, shellName, shellName)
}

func renderFishShellInitScript() string {
	return `# kra shell integration (fish)
# Add this line to your config.fish, then restart shell:
#   eval (kra shell init fish)
function kra
  set -l __kra_action_file (mktemp "/tmp/kra-action.XXXXXX"); or return 1
  env KRA_SHELL_ACTION_FILE="$__kra_action_file" command kra $argv
  set -l __kra_status $status
  if test $__kra_status -ne 0
    rm -f "$__kra_action_file"
    return $__kra_status
  end
  if test -s "$__kra_action_file"
    eval (cat "$__kra_action_file")
  end
  rm -f "$__kra_action_file"
end
`
}
