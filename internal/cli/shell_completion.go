package cli

import (
	"fmt"
	"strings"
)

var kraCompletionRootCommands = []string{
	"init",
	"context",
	"repo",
	"template",
	"shell",
	"ws",
	"doctor",
	"version",
	"help",
}

var kraCompletionGlobalFlags = []string{
	"--debug",
	"--version",
	"--help",
	"-h",
}

var kraCompletionSubcommandOrder = []string{
	"context",
	"repo",
	"template",
	"shell",
	"ws",
}

var kraCompletionSubcommands = map[string][]string{
	"context":  {"current", "list", "create", "use", "rename", "rm", "help"},
	"repo":     {"add", "discover", "remove", "gc", "help"},
	"template": {"validate", "help"},
	"shell":    {"init", "completion", "help"},
	"ws": {
		"create",
		"import",
		"list",
		"ls",
		"dashboard",
		"insight",
		"select",
		"lock",
		"unlock",
		"open",
		"switch",
		"add-repo",
		"remove-repo",
		"close",
		"reopen",
		"purge",
		"help",
	},
}

var kraCompletionPathSubcommandOrder = []string{
	"ws import",
	"ws insight",
}

var kraCompletionPathSubcommands = map[string][]string{
	"ws import":  {"jira", "help"},
	"ws insight": {"add", "help"},
}

var kraCompletionCommandFlagOrder = []string{
	"init",
	"doctor",
	"version",
	"ws",
}

var kraCompletionCommandFlags = map[string][]string{
	"init":    {"--root", "--context", "--format", "--help", "-h"},
	"doctor":  {"--format", "--fix", "--plan", "--apply", "--help", "-h"},
	"version": {"--help", "-h"},
	"ws":      {"--id", "--current", "--select", "--help", "-h"},
}

var kraCompletionPathFlagOrder = []string{
	"context current",
	"context list",
	"context create",
	"context use",
	"context rename",
	"context rm",
	"repo add",
	"repo discover",
	"repo remove",
	"repo gc",
	"template validate",
	"shell init",
	"shell completion",
	"ws create",
	"ws import",
	"ws import jira",
	"ws list",
	"ws ls",
	"ws dashboard",
	"ws open",
	"ws switch",
	"ws add-repo",
	"ws remove-repo",
	"ws close",
	"ws reopen",
	"ws purge",
	"ws lock",
	"ws unlock",
	"ws select",
	"ws insight",
	"ws insight add",
}

var kraCompletionPathFlags = map[string][]string{
	"context current":   {"--format", "--help", "-h"},
	"context list":      {"--format", "--help", "-h"},
	"context create":    {"--path", "--use", "--format", "--help", "-h"},
	"context use":       {"--format", "--help", "-h"},
	"context rename":    {"--format", "--help", "-h"},
	"context rm":        {"--format", "--help", "-h"},
	"repo add":          {"--format", "--help", "-h"},
	"repo discover":     {"--org", "--provider", "--help", "-h"},
	"repo remove":       {"--format", "--help", "-h"},
	"repo gc":           {"--format", "--yes", "--help", "-h"},
	"template validate": {"--name", "--help", "-h"},
	"shell init":        {"--help", "-h"},
	"shell completion":  {"--help", "-h"},
	"ws create":         {"--no-prompt", "--template", "--format", "--id", "--title", "--jira", "--help", "-h"},
	"ws import":         {"--help", "-h"},
	"ws import jira":    {"--sprint", "--space", "--project", "--jql", "--limit", "--apply", "--no-prompt", "--format", "--help", "-h"},
	"ws list":           {"--archived", "--tree", "--format", "--help", "-h"},
	"ws ls":             {"--archived", "--tree", "--format", "--help", "-h"},
	"ws dashboard":      {"--archived", "--workspace", "--format", "--help", "-h"},
	"ws open":           {"--id", "--current", "--select", "--multi", "--concurrency", "--format", "--help", "-h"},
	"ws switch":         {"--id", "--current", "--select", "--multi", "--concurrency", "--format", "--help", "-h"},
	"ws add-repo":       {"--id", "--current", "--select", "--format", "--repo", "--branch", "--base-ref", "--yes", "--refresh", "--no-fetch", "--help", "-h"},
	"ws remove-repo":    {"--id", "--current", "--select", "--format", "--repo", "--yes", "--force", "--help", "-h"},
	"ws close":          {"--id", "--current", "--select", "--force", "--format", "--no-commit", "--dry-run", "--help", "-h"},
	"ws reopen":         {"--id", "--current", "--select", "--format", "--no-commit", "--dry-run", "--help", "-h"},
	"ws purge":          {"--id", "--current", "--select", "--no-prompt", "--force", "--format", "--no-commit", "--dry-run", "--help", "-h"},
	"ws lock":           {"--format", "--help", "-h"},
	"ws unlock":         {"--format", "--help", "-h"},
	"ws select":         {"--select", "--multi", "--archived", "--no-commit", "--help", "-h"},
	"ws insight":        {"--help", "-h"},
	"ws insight add":    {"--id", "--ticket", "--session-id", "--what", "--approved", "--context", "--why", "--next", "--tag", "--format", "--help", "-h"},
}

var kraCompletionTargetRequiredPaths = []string{
	"ws open",
	"ws switch",
	"ws add-repo",
	"ws remove-repo",
	"ws close",
	"ws reopen",
	"ws purge",
}

var kraCompletionTargetSelectorFlags = []string{
	"--id",
	"--current",
	"--select",
	"--help",
	"-h",
}

func renderShellCompletionScript(shellName string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(shellName)) {
	case "zsh":
		return renderZshCompletionScript(), nil
	case "bash", "sh":
		return renderBashCompletionScript(), nil
	case "fish":
		return renderFishCompletionScript(), nil
	default:
		return "", fmt.Errorf("unsupported shell %q (supported: zsh, bash, sh, fish)", shellName)
	}
}

func renderBashCompletionScript() string {
	return fmt.Sprintf(`# kra completion (bash)
_kra_completion() {
  local cur prev cmd subcmd subcmd2 path i j has_target
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev=""
  if [[ ${COMP_CWORD} -gt 0 ]]; then
    prev="${COMP_WORDS[COMP_CWORD-1]}"
  fi

  cmd=""
  subcmd=""
  subcmd2=""
  for ((i=1; i<COMP_CWORD; i++)); do
    if [[ "${COMP_WORDS[i]}" != -* ]]; then
      if [[ -z "${cmd}" ]]; then
        cmd="${COMP_WORDS[i]}"
      elif [[ -z "${subcmd}" ]]; then
        subcmd="${COMP_WORDS[i]}"
      elif [[ -z "${subcmd2}" ]]; then
        subcmd2="${COMP_WORDS[i]}"
      fi
    fi
  done

  if [[ -z "${cmd}" ]]; then
    COMPREPLY=( $(compgen -W %q -- "${cur}") )
    return 0
  fi

  if [[ "${cur}" == -* ]]; then
    if [[ -n "${subcmd2}" ]]; then
      path="${cmd} ${subcmd} ${subcmd2}"
    elif [[ -n "${subcmd}" ]]; then
      path="${cmd} ${subcmd}"
    else
      path="${cmd}"
    fi

    case "${path}" in
%s
    esac

    case "${path}" in
%s
%s
    esac
    return 0
  fi

  if [[ -z "${subcmd}" ]]; then
    case "${cmd}" in
%s
    esac
    return 0
  fi

  if [[ -z "${subcmd2}" ]]; then
    path="${cmd} ${subcmd}"
    case "${path}" in
%s
    esac
  fi

  return 0
}
complete -o default -F _kra_completion kra
`, strings.Join(kraCompletionTopWords(), " "), renderBashTargetSelectorGateCases(), renderBashCommandFlagCases(), renderBashPathFlagCases(), renderBashSubcommandCases(), renderBashPathSubcommandCases())
}

func renderBashCommandFlagCases() string {
	lines := make([]string, 0, len(kraCompletionCommandFlagOrder)*3)
	for _, cmd := range kraCompletionCommandFlagOrder {
		flags := kraCompletionCommandFlags[cmd]
		if len(flags) == 0 {
			continue
		}
		lines = append(lines,
			fmt.Sprintf("      %q)", cmd),
			fmt.Sprintf("        COMPREPLY=( $(compgen -W %q -- \"${cur}\") )", strings.Join(flags, " ")),
			"        ;;",
		)
	}
	return strings.Join(lines, "\n")
}

func renderBashPathFlagCases() string {
	lines := make([]string, 0, len(kraCompletionPathFlagOrder)*3)
	for _, path := range kraCompletionPathFlagOrder {
		flags := kraCompletionPathFlags[path]
		if len(flags) == 0 {
			continue
		}
		lines = append(lines,
			fmt.Sprintf("      %q)", path),
			fmt.Sprintf("        COMPREPLY=( $(compgen -W %q -- \"${cur}\") )", strings.Join(flags, " ")),
			"        ;;",
		)
	}
	return strings.Join(lines, "\n")
}

func renderBashSubcommandCases() string {
	lines := make([]string, 0, len(kraCompletionSubcommandOrder)*4)
	for _, cmd := range kraCompletionSubcommandOrder {
		subs := strings.Join(kraCompletionSubcommands[cmd], " ")
		lines = append(lines,
			fmt.Sprintf("    %s)", cmd),
			fmt.Sprintf("      if [[ \"${prev}\" == %q ]]; then", cmd),
			fmt.Sprintf("        COMPREPLY=( $(compgen -W %q -- \"${cur}\") )", subs),
			"      fi",
			"      ;;",
		)
	}
	return strings.Join(lines, "\n")
}

func renderBashPathSubcommandCases() string {
	lines := make([]string, 0, len(kraCompletionPathSubcommandOrder)*4)
	for _, path := range kraCompletionPathSubcommandOrder {
		subs := strings.Join(kraCompletionPathSubcommands[path], " ")
		parts := strings.Fields(path)
		if len(parts) < 2 {
			continue
		}
		prev := parts[len(parts)-1]
		lines = append(lines,
			fmt.Sprintf("      %q)", path),
			fmt.Sprintf("        if [[ \"${prev}\" == %q ]]; then", prev),
			fmt.Sprintf("          COMPREPLY=( $(compgen -W %q -- \"${cur}\") )", subs),
			"        fi",
			"        ;;",
		)
	}
	return strings.Join(lines, "\n")
}

func renderBashTargetSelectorGateCases() string {
	lines := make([]string, 0, len(kraCompletionTargetRequiredPaths)*12)
	for _, path := range kraCompletionTargetRequiredPaths {
		lines = append(lines,
			fmt.Sprintf("      %q)", path),
			"        has_target=0",
			"        for ((j=1; j<COMP_CWORD; j++)); do",
			"          case \"${COMP_WORDS[j]}\" in",
			"            --id|--id=*|--current|--select) has_target=1; break ;;",
			"          esac",
			"        done",
			"        if [[ ${has_target} -eq 0 ]]; then",
			fmt.Sprintf("          COMPREPLY=( $(compgen -W %q -- \"${cur}\") )", strings.Join(kraCompletionTargetSelectorFlags, " ")),
			"        else",
			fmt.Sprintf("          COMPREPLY=( $(compgen -W %q -- \"${cur}\") )", strings.Join(completionFlagsWithoutTargetSelectors(path), " ")),
			"        fi",
			"        return 0",
			"        ;;",
		)
	}
	return strings.Join(lines, "\n")
}

func renderZshCompletionScript() string {
	return fmt.Sprintf(`# kra completion (zsh)
_kra_completion() {
  local -a top sub sub2 flags
  local cmd="" subcmd="" subcmd2="" path="" i j has_target
  local current_word="${words[CURRENT]}"

  top=(%s)

  for (( i=2; i<CURRENT; i++ )); do
    if [[ "${words[i]}" != -* ]]; then
      if [[ -z "${cmd}" ]]; then
        cmd="${words[i]}"
      elif [[ -z "${subcmd}" ]]; then
        subcmd="${words[i]}"
      elif [[ -z "${subcmd2}" ]]; then
        subcmd2="${words[i]}"
      fi
    fi
  done

  if [[ -z "${cmd}" ]]; then
    compadd -- "${top[@]}"
    return 0
  fi

  if [[ "${current_word}" == -* ]]; then
    if [[ -n "${subcmd2}" ]]; then
      path="${cmd} ${subcmd} ${subcmd2}"
    elif [[ -n "${subcmd}" ]]; then
      path="${cmd} ${subcmd}"
    else
      path="${cmd}"
    fi
    case "$path" in
%s
    esac
    flags=()
    case "$path" in
%s
%s
    esac
    if [[ ${#flags[@]} -gt 0 ]]; then
      compadd -- "${flags[@]}"
    fi
    return 0
  fi

  sub=()
  if [[ -z "${subcmd}" ]]; then
    case "$cmd" in
%s
    esac
    if [[ ${#sub[@]} -gt 0 ]] && [[ "${words[CURRENT-1]}" == "$cmd" ]]; then
      compadd -- "${sub[@]}"
    fi
    return 0
  fi

  sub2=()
  if [[ -z "${subcmd2}" ]]; then
    path="${cmd} ${subcmd}"
    case "$path" in
%s
    esac
    if [[ ${#sub2[@]} -gt 0 ]] && [[ "${words[CURRENT-1]}" == "$subcmd" ]]; then
      compadd -- "${sub2[@]}"
    fi
  fi
}
compdef _kra_completion kra
`, zshQuotedWords(kraCompletionTopWords()), renderZshTargetSelectorGateCases(), renderZshCommandFlagCases(), renderZshPathFlagCases(), renderZshSubcommandCases(), renderZshPathSubcommandCases())
}

func renderZshCommandFlagCases() string {
	lines := make([]string, 0, len(kraCompletionCommandFlagOrder))
	for _, cmd := range kraCompletionCommandFlagOrder {
		flags := kraCompletionCommandFlags[cmd]
		if len(flags) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("    %q) flags=(%s) ;;", cmd, zshQuotedWords(flags)))
	}
	return strings.Join(lines, "\n")
}

func renderZshPathFlagCases() string {
	lines := make([]string, 0, len(kraCompletionPathFlagOrder))
	for _, path := range kraCompletionPathFlagOrder {
		flags := kraCompletionPathFlags[path]
		if len(flags) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("    %q) flags=(%s) ;;", path, zshQuotedWords(flags)))
	}
	return strings.Join(lines, "\n")
}

func renderZshSubcommandCases() string {
	lines := make([]string, 0, len(kraCompletionSubcommandOrder))
	for _, cmd := range kraCompletionSubcommandOrder {
		lines = append(lines, fmt.Sprintf("    %q) sub=(%s) ;;", cmd, zshQuotedWords(kraCompletionSubcommands[cmd])))
	}
	return strings.Join(lines, "\n")
}

func renderZshPathSubcommandCases() string {
	lines := make([]string, 0, len(kraCompletionPathSubcommandOrder))
	for _, path := range kraCompletionPathSubcommandOrder {
		lines = append(lines, fmt.Sprintf("    %q) sub2=(%s) ;;", path, zshQuotedWords(kraCompletionPathSubcommands[path])))
	}
	return strings.Join(lines, "\n")
}

func renderZshTargetSelectorGateCases() string {
	lines := make([]string, 0, len(kraCompletionTargetRequiredPaths)*14)
	for _, path := range kraCompletionTargetRequiredPaths {
		lines = append(lines,
			fmt.Sprintf("    %q)", path),
			"      has_target=0",
			"      for (( j=2; j<CURRENT; j++ )); do",
			"        case \"${words[j]}\" in",
			"          --id|--id=*|--current|--select) has_target=1; break ;;",
			"        esac",
			"      done",
			"      if [[ ${has_target} -eq 0 ]]; then",
			fmt.Sprintf("        flags=(%s)", zshQuotedWords(kraCompletionTargetSelectorFlags)),
			"      else",
			fmt.Sprintf("        flags=(%s)", zshQuotedWords(completionFlagsWithoutTargetSelectors(path))),
			"      fi",
			"      compadd -- \"${flags[@]}\"",
			"      return 0",
			"      ;;",
		)
	}
	return strings.Join(lines, "\n")
}

func completionFlagsWithoutTargetSelectors(path string) []string {
	flags := kraCompletionPathFlags[path]
	if len(flags) == 0 {
		return append([]string{}, kraCompletionTargetSelectorFlags...)
	}
	out := make([]string, 0, len(flags))
	for _, flag := range flags {
		switch flag {
		case "--id", "--current", "--select":
			continue
		default:
			out = append(out, flag)
		}
	}
	if len(out) == 0 {
		return []string{"--help", "-h"}
	}
	return out
}

func zshQuotedWords(words []string) string {
	quoted := make([]string, 0, len(words))
	for _, word := range words {
		quoted = append(quoted, fmt.Sprintf("%q", word))
	}
	return strings.Join(quoted, " ")
}

func renderFishCompletionScript() string {
	var b strings.Builder
	b.WriteString("# kra completion (fish)\n")
	b.WriteString("complete -c kra -f\n")
	b.WriteString("complete -c kra -l debug -d \"Enable debug logging\"\n")
	b.WriteString("complete -c kra -l version -d \"Print version and exit\"\n")
	b.WriteString("complete -c kra -l help -s h -d \"Show help\"\n")
	b.WriteString(
		fmt.Sprintf(
			"complete -c kra -n \"__fish_use_subcommand\" -a %q\n",
			strings.Join(kraCompletionRootCommands, " "),
		),
	)
	for _, cmd := range kraCompletionSubcommandOrder {
		b.WriteString(
			fmt.Sprintf(
				"complete -c kra -n %q -a %q\n",
				fishConditionForPath(cmd),
				strings.Join(kraCompletionSubcommands[cmd], " "),
			),
		)
	}
	for _, path := range kraCompletionPathSubcommandOrder {
		b.WriteString(
			fmt.Sprintf(
				"complete -c kra -n %q -a %q\n",
				fishConditionForPath(path),
				strings.Join(kraCompletionPathSubcommands[path], " "),
			),
		)
	}
	for _, cmd := range kraCompletionCommandFlagOrder {
		cond := fishConditionForPath(cmd)
		for _, flag := range kraCompletionCommandFlags[cmd] {
			b.WriteString(renderFishFlagCompletionLine(cond, flag))
		}
	}
	for _, path := range kraCompletionPathFlagOrder {
		cond := fishConditionForPath(path)
		for _, flag := range kraCompletionPathFlags[path] {
			b.WriteString(renderFishFlagCompletionLine(cond, flag))
		}
	}
	return b.String()
}

func renderFishFlagCompletionLine(cond string, flag string) string {
	if flag == "-h" {
		return fmt.Sprintf("complete -c kra -n %q -s h -d \"Show help\"\n", cond)
	}
	if strings.HasPrefix(flag, "--") {
		return fmt.Sprintf("complete -c kra -n %q -l %s\n", cond, strings.TrimPrefix(flag, "--"))
	}
	return ""
}

func fishConditionForPath(path string) string {
	parts := strings.Fields(strings.TrimSpace(path))
	if len(parts) == 0 {
		return "__fish_use_subcommand"
	}
	conds := make([]string, 0, len(parts))
	for _, p := range parts {
		conds = append(conds, fmt.Sprintf("__fish_seen_subcommand_from %s", p))
	}
	return strings.Join(conds, "; and ")
}

func kraCompletionTopWords() []string {
	out := make([]string, 0, len(kraCompletionRootCommands)+len(kraCompletionGlobalFlags))
	out = append(out, kraCompletionRootCommands...)
	out = append(out, kraCompletionGlobalFlags...)
	return out
}
