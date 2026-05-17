package cli

import (
	"fmt"
	"slices"
	"strings"
)

var kraCompletionRootCommands = []string{
	"init",
	"agent",
	"context",
	"root",
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
	"agent",
	"context",
	"root",
	"repo",
	"template",
	"shell",
	"ws",
}

var kraCompletionSubcommands = map[string][]string{
	"agent":    {"prompt", "help"},
	"context":  {"current", "list", "create", "use", "rename", "rm", "help"},
	"root":     {"current", "open", "help"},
	"repo":     {"add", "discover", "preset", "remove", "gc", "help"},
	"template": {"create", "remove", "rm", "validate", "help"},
	"shell":    {"init", "completion", "help"},
	"ws": {
		"create",
		"import",
		"list",
		"ls",
		"dashboard",
		"doc",
		"lock",
		"unlock",
		"open",
		"add-repo",
		"remove-repo",
		"close",
		"reopen",
		"purge",
		"task",
		"status",
		"log",
		"help",
	},
}

var kraCompletionPathSubcommandOrder = []string{
	"repo preset",
	"ws import",
	"ws doc",
	"ws task",
}

var kraCompletionPathSubcommands = map[string][]string{
	"repo preset":      {"add", "rm", "remove", "list", "show", "help"},
	"ws import":        {"all", "github", "jira", "help"},
	"ws import github": {"issue", "review", "help"},
	"ws doc":           {"open", "help"},
	"ws task":          {"list", "ls", "tui", "add", "status", "sync", "help"},
}

var kraCompletionCommandFlagOrder = []string{
	"init",
	"agent",
	"doctor",
	"help",
	"version",
	"ws",
}

var kraCompletionCommandFlags = map[string][]string{
	"init":    {"--root", "--context", "--format", "--help", "-h"},
	"agent":   {"--help", "-h"},
	"doctor":  {"--format", "--fix", "--plan", "--apply", "--help", "-h"},
	"help":    {"--mode", "--format", "--help", "-h"},
	"version": {"--help", "-h"},
	"ws":      {"--id", "--current", "--select", "--multi-select", "--all", "--help", "-h"},
}

var kraCompletionPathFlagOrder = []string{
	"context current",
	"agent prompt",
	"context list",
	"context create",
	"context use",
	"context rename",
	"context rm",
	"root current",
	"root open",
	"root migrate",
	"repo add",
	"repo discover",
	"repo preset",
	"repo preset add",
	"repo preset rm",
	"repo preset remove",
	"repo preset list",
	"repo preset show",
	"repo remove",
	"repo gc",
	"template create",
	"template remove",
	"template rm",
	"template validate",
	"shell init",
	"shell completion",
	"ws create",
	"ws import",
	"ws import github",
	"ws import github issue",
	"ws import github review",
	"ws import jira",
	"ws import all",
	"ws doc",
	"ws doc open",
	"ws task",
	"ws task dock",
	"ws task dock install",
	"ws task list",
	"ws task ls",
	"ws task tui",
	"ws status",
	"ws task add",
	"ws task status",
	"ws task sync",
	"ws list",
	"ws ls",
	"ws dashboard",
	"ws open",
	"ws add-repo",
	"ws remove-repo",
	"ws close",
	"ws reopen",
	"ws purge",
	"ws log",
	"ws lock",
	"ws unlock",
}

var kraCompletionPathFlags = map[string][]string{
	"agent prompt":            {"--brief", "--format", "--help", "-h"},
	"context current":         {"--format", "--help", "-h"},
	"context list":            {"--format", "--help", "-h"},
	"context create":          {"--path", "--use", "--format", "--help", "-h"},
	"context use":             {"--format", "--help", "-h"},
	"context rename":          {"--format", "--help", "-h"},
	"context rm":              {"--format", "--help", "-h"},
	"root current":            {"--format", "--help", "-h"},
	"root open":               {"--format", "--help", "-h"},
	"root migrate":            {"--apply", "--format", "--help", "-h"},
	"repo add":                {"--format", "--help", "-h"},
	"repo discover":           {"--org", "--provider", "--help", "-h"},
	"repo preset":             {"--help", "-h"},
	"repo preset add":         {"--yes", "--help", "-h"},
	"repo preset rm":          {"--help", "-h"},
	"repo preset remove":      {"--help", "-h"},
	"repo preset list":        {"--help", "-h"},
	"repo preset show":        {"--help", "-h"},
	"repo remove":             {"--format", "--help", "-h"},
	"repo gc":                 {"--format", "--yes", "--help", "-h"},
	"template create":         {"--name", "--from", "--help", "-h"},
	"template remove":         {"--name", "--help", "-h"},
	"template rm":             {"--name", "--help", "-h"},
	"template validate":       {"--name", "--help", "-h"},
	"shell init":              {"--with-completion", "--help", "-h"},
	"shell completion":        {"--help", "-h"},
	"ws create":               {"--no-prompt", "--template", "--format", "--id", "--title", "--jira", "--help", "-h"},
	"ws import":               {"--help", "-h"},
	"ws import all":           {"--target", "--limit", "--apply", "--no-prompt", "--format", "--help", "-h"},
	"ws import github":        {"--help", "-h"},
	"ws import github issue":  {"--org", "--repo", "--state", "--limit", "--apply", "--no-prompt", "--format", "--help", "-h"},
	"ws import github review": {"--org", "--repo", "--limit", "--apply", "--no-prompt", "--format", "--help", "-h"},
	"ws import jira":          {"--sprint", "--space", "--project", "--jql", "--limit", "--apply", "--no-prompt", "--format", "--help", "-h"},
	"ws doc":                  {"--help", "-h"},
	"ws doc open":             {"--id", "--current", "--select", "--surface", "--no-focus", "--format", "--help", "-h"},
	"ws task":                 {"--id", "--current", "--cmux-current", "--select", "--help", "-h"},
	"ws task dock":            {"--help", "-h"},
	"ws task dock install":    {"--global", "--format", "--help", "-h"},
	"ws task list":            {"--id", "--current", "--select", "--format", "--help", "-h"},
	"ws task ls":              {"--id", "--current", "--select", "--format", "--help", "-h"},
	"ws task view":            {"--id", "--current", "--cmux-current", "--select", "--all", "--watch", "--todo-only", "--include-done", "--no-color", "--help", "-h"},
	"ws task tui":             {"--id", "--current", "--cmux-current", "--select", "--all", "--todo-only", "--include-done", "--no-color", "--help", "-h"},
	"ws status":               {"--id", "--current", "--cmux-current", "--select", "--all", "--todo-only", "--include-done", "--no-color", "--help", "-h"},
	"ws task add":             {"--id", "--current", "--select", "--title", "--description", "--format", "--help", "-h"},
	"ws task status":          {"--id", "--current", "--select", "--format", "--help", "-h"},
	"ws task sync":            {"--id", "--current", "--select", "--all", "--format", "--help", "-h"},
	"ws list":                 {"--archived", "--tree", "--format", "--help", "-h"},
	"ws ls":                   {"--archived", "--tree", "--format", "--help", "-h"},
	"ws dashboard":            {"--archived", "--workspace", "--format", "--help", "-h"},
	"ws open":                 {"--id", "--current", "--select", "--multi-select", "--all", "--concurrency", "--format", "--help", "-h"},
	"ws add-repo":             {"--id", "--current", "--select", "--format", "--preset", "--repo", "--branch", "--base-ref", "--yes", "--refresh", "--no-fetch", "--help", "-h"},
	"ws remove-repo":          {"--id", "--current", "--select", "--format", "--repo", "--yes", "--force", "--help", "-h"},
	"ws close":                {"--id", "--current", "--select", "--multi-select", "--force", "--format", "--no-commit", "--dry-run", "--help", "-h"},
	"ws reopen":               {"--id", "--current", "--select", "--format", "--no-commit", "--dry-run", "--help", "-h"},
	"ws purge":                {"--id", "--current", "--select", "--no-prompt", "--force", "--format", "--no-commit", "--dry-run", "--help", "-h"},
	"ws log":                  {"--id", "--current", "--help", "-h"},
	"ws lock":                 {"--id", "--current", "--select", "--multi-select", "--format", "--help", "-h"},
	"ws unlock":               {"--id", "--current", "--select", "--multi-select", "--format", "--help", "-h"},
}

var kraCompletionPathFlagValues = map[string]map[string][]string{
	"ws import github issue": {
		"--state": {"open", "closed", "all"},
	},
	"ws import all": {
		"--target": {"jira", "github-review", "both"},
	},
}

var kraCompletionTargetRequiredPaths = []string{
	"ws open",
	"ws task tui",
	"ws status",
	"ws task sync",
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
	"--multi-select",
	"--all",
	"--help",
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

  if [[ -n "${subcmd2}" ]]; then
    path="${cmd} ${subcmd} ${subcmd2}"
  elif [[ -n "${subcmd}" ]]; then
    path="${cmd} ${subcmd}"
  else
    path="${cmd}"
  fi

  case "${path}:${prev}" in
%s
  esac

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
`, strings.Join(kraCompletionTopWords(), " "), renderBashTargetSelectorGateCases(), renderBashCommandFlagCases(), renderBashPathFlagCases(), renderBashFlagValueCases(), renderBashSubcommandCases(), renderBashPathSubcommandCases())
}

func renderBashCommandFlagCases() string {
	lines := make([]string, 0, len(kraCompletionCommandFlagOrder)*3)
	for _, cmd := range kraCompletionCommandFlagOrder {
		flags := completionRenderableFlags(kraCompletionCommandFlags[cmd])
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
		flags := completionRenderableFlags(kraCompletionPathFlags[path])
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

func renderBashFlagValueCases() string {
	lines := make([]string, 0, len(kraCompletionPathFlagValues)*3)
	for _, path := range kraCompletionPathFlagOrder {
		byFlag, ok := kraCompletionPathFlagValues[path]
		if !ok {
			continue
		}
		flags := sortedCompletionFlagValueKeys(byFlag)
		for _, flag := range flags {
			values := byFlag[flag]
			if len(values) == 0 {
				continue
			}
			lines = append(lines,
				fmt.Sprintf("    %q)", path+":"+flag),
				fmt.Sprintf("      COMPREPLY=( $(compgen -W %q -- \"${cur}\") )", strings.Join(values, " ")),
				"      return 0",
				"      ;;",
			)
		}
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
			"            --id|--id=*|--current|--select|--multi-select|--all) has_target=1; break ;;",
			"          esac",
			"        done",
			"        if [[ ${has_target} -eq 0 ]]; then",
			fmt.Sprintf("          COMPREPLY=( $(compgen -W %q -- \"${cur}\") )", strings.Join(kraCompletionTargetSelectorFlags, " ")),
			"        else",
			fmt.Sprintf("          COMPREPLY=( $(compgen -W %q -- \"${cur}\") )", strings.Join(completionRenderableFlags(completionFlagsWithoutTargetSelectors(path)), " ")),
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
    compadd -V kra_top -- "${top[@]}"
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
      compadd -V kra_flags -- "${flags[@]}"
    fi
    return 0
  fi

  if [[ -n "${subcmd2}" ]]; then
    path="${cmd} ${subcmd} ${subcmd2}"
  elif [[ -n "${subcmd}" ]]; then
    path="${cmd} ${subcmd}"
  else
    path="${cmd}"
  fi
  case "${path}:${words[CURRENT-1]}" in
%s
  esac

  sub=()
  if [[ -z "${subcmd}" ]]; then
    case "$cmd" in
%s
    esac
    if [[ ${#sub[@]} -gt 0 ]] && [[ "${words[CURRENT-1]}" == "$cmd" ]]; then
      compadd -V kra_sub -- "${sub[@]}"
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
      compadd -V kra_sub2 -- "${sub2[@]}"
    fi
  fi
}
compdef _kra_completion kra
`, zshQuotedWords(kraCompletionTopWords()), renderZshTargetSelectorGateCases(), renderZshCommandFlagCases(), renderZshPathFlagCases(), renderZshFlagValueCases(), renderZshSubcommandCases(), renderZshPathSubcommandCases())
}

func renderZshCommandFlagCases() string {
	lines := make([]string, 0, len(kraCompletionCommandFlagOrder))
	for _, cmd := range kraCompletionCommandFlagOrder {
		flags := completionRenderableFlags(kraCompletionCommandFlags[cmd])
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
		flags := completionRenderableFlags(kraCompletionPathFlags[path])
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

func renderZshFlagValueCases() string {
	lines := make([]string, 0, len(kraCompletionPathFlagValues))
	for _, path := range kraCompletionPathFlagOrder {
		byFlag, ok := kraCompletionPathFlagValues[path]
		if !ok {
			continue
		}
		flags := sortedCompletionFlagValueKeys(byFlag)
		for _, flag := range flags {
			values := byFlag[flag]
			if len(values) == 0 {
				continue
			}
			lines = append(lines, fmt.Sprintf("    %q) compadd -V kra_values -- %s; return 0 ;;", path+":"+flag, zshQuotedWords(values)))
		}
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
			"          --id|--id=*|--current|--select|--multi-select|--all) has_target=1; break ;;",
			"        esac",
			"      done",
			"      if [[ ${has_target} -eq 0 ]]; then",
			fmt.Sprintf("        flags=(%s)", zshQuotedWords(kraCompletionTargetSelectorFlags)),
			"      else",
			fmt.Sprintf("        flags=(%s)", zshQuotedWords(completionRenderableFlags(completionFlagsWithoutTargetSelectors(path)))),
			"      fi",
			"      compadd -V kra_flags -- \"${flags[@]}\"",
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
		case "--id", "--current", "--select", "--multi-select", "--all":
			continue
		default:
			out = append(out, flag)
		}
	}
	if len(out) == 0 {
		return []string{"--help"}
	}
	return out
}

func completionRenderableFlags(flags []string) []string {
	out := make([]string, 0, len(flags))
	seen := make(map[string]struct{}, len(flags))
	hasLongHelp := false
	for _, flag := range flags {
		v := strings.TrimSpace(flag)
		if v == "" || v == "-h" {
			continue
		}
		if v == "--help" {
			hasLongHelp = true
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if hasLongHelp {
		out = append(out, "--help")
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
	b.WriteString("complete -c kra -l help -d \"Show help\"\n")
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
		for _, flag := range completionRenderableFlags(kraCompletionCommandFlags[cmd]) {
			b.WriteString(renderFishFlagCompletionLine(cond, flag))
		}
	}
	for _, path := range kraCompletionPathFlagOrder {
		cond := fishConditionForPath(path)
		for _, flag := range completionRenderableFlags(kraCompletionPathFlags[path]) {
			b.WriteString(renderFishFlagCompletionLine(cond, flag))
		}
	}
	for _, path := range kraCompletionPathFlagOrder {
		byFlag, ok := kraCompletionPathFlagValues[path]
		if !ok {
			continue
		}
		flags := sortedCompletionFlagValueKeys(byFlag)
		for _, flag := range flags {
			values := byFlag[flag]
			if len(values) == 0 {
				continue
			}
			b.WriteString(renderFishFlagValueCompletionLines(path, flag, values))
		}
	}
	return b.String()
}

func renderFishFlagCompletionLine(cond string, flag string) string {
	if strings.HasPrefix(flag, "--") {
		return fmt.Sprintf("complete -c kra -n %q -l %s\n", cond, strings.TrimPrefix(flag, "--"))
	}
	return ""
}

func renderFishFlagValueCompletionLines(path string, flag string, values []string) string {
	if len(values) == 0 || !strings.HasPrefix(flag, "--") {
		return ""
	}
	return fmt.Sprintf("complete -c kra -n %q -l %s -a %q\n", fishConditionForPath(path), strings.TrimPrefix(flag, "--"), strings.Join(values, " "))
}

func sortedCompletionFlagValueKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
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
	out = append(out, completionRenderableFlags(kraCompletionGlobalFlags)...)
	return out
}
