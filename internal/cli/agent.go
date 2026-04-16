package cli

import (
	"fmt"
	"io"
	"strings"
)

type agentPromptSpec struct {
	WhatKraIs                   []string `json:"what_kra_is"`
	RecommendedWorkflow         []string `json:"recommended_workflow"`
	AutomationRules             []string `json:"automation_rules"`
	SafeCommands                []string `json:"safe_commands"`
	DestructiveCommands         []string `json:"destructive_commands"`
	JSONFirstCommands           []string `json:"json_first_commands"`
	CommandsToAvoidWithoutFlags []string `json:"commands_to_avoid_without_explicit_flags"`
	RecoveryBasics              []string `json:"recovery_basics"`
	DrillDownCommands           []string `json:"drill_down_commands"`
}

var defaultAgentPrompt = agentPromptSpec{
	WhatKraIs: []string{
		"kra is a filesystem-based workspace lifecycle CLI.",
		"Main lifecycle: init -> ws create -> ws add-repo -> ws open -> ws close/reopen/purge.",
		"KRA_ROOT stores active and archived workspaces; shared state lives under KRA_HOME.",
	},
	RecommendedWorkflow: []string{
		"kra init --root <path> --context <name> --format json",
		"kra ws create --id <id> --no-prompt --format json",
		"kra ws add-repo --format json --id <id> --repo <repo-key> ... --yes",
		"kra ws open --id <id> --format json",
		"kra ws close --dry-run --format json --id <id>",
	},
	AutomationRules: []string{
		"Prefer explicit flags over interactive selectors.",
		"Prefer --id over --current, and avoid --select/--multi-select in automation.",
		"Prefer --format json when supported.",
		"Run dry-run preflight before destructive or high-risk actions when available.",
	},
	SafeCommands: []string{
		"kra agent prompt",
		"kra help <command-path> --mode agent",
		"kra doctor --format json",
		"kra ws list --format json",
		"kra root current --format json",
	},
	DestructiveCommands: []string{
		"kra ws purge",
	},
	JSONFirstCommands: []string{
		"kra init",
		"kra doctor",
		"kra ws create",
		"kra ws open",
		"kra ws add-repo",
		"kra ws close",
		"kra ws reopen",
		"kra ws purge",
	},
	CommandsToAvoidWithoutFlags: []string{
		"avoid selector-driven forms such as kra ws --select ... in automation",
		"avoid prompt-driven template or repo selection without explicit flags",
		"avoid ws purge apply without reviewing dry-run output first",
	},
	RecoveryBasics: []string{
		"if a command fails, inspect stderr or JSON error.code before retrying",
		"when available, fetch command-specific contract via kra help <command-path> --mode agent",
		"if KRA_ROOT cannot be resolved, run from inside a root or initialize one with kra init",
	},
	DrillDownCommands: []string{
		"kra help init --mode agent",
		"kra help doctor --mode agent",
		"kra help ws create --mode agent",
		"kra help ws add-repo --mode agent",
		"kra help ws open --mode agent",
		"kra help ws close --mode agent",
		"kra help ws reopen --mode agent",
		"kra help ws purge --mode agent",
	},
}

var briefAgentPrompt = agentPromptSpec{
	WhatKraIs: []string{
		"kra = workspace lifecycle CLI (init -> ws create -> ws add-repo -> open -> close/reopen/purge).",
	},
	AutomationRules: []string{
		"Automation: prefer --id over --current/--select, --format json where supported.",
	},
	SafeCommands: []string{
		"Safe JSON-first: kra doctor | ws list | root current | ws open/close/create/add-repo.",
	},
	DestructiveCommands: []string{
		"Destructive: kra ws purge (run --dry-run first).",
	},
	DrillDownCommands: []string{
		"Drill down: kra help <command-path> --mode agent",
	},
}

func (c *CLI) runAgent(args []string) int {
	if len(args) == 0 {
		c.printAgentUsage(c.Err)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		c.printAgentUsage(c.Out)
		return exitOK
	case "prompt":
		return c.runAgentPrompt(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"agent"}, args[0]), " "))
		c.printAgentUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runAgentPrompt(args []string) int {
	outputFormat := "text"
	brief := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help", "help":
			c.printAgentPromptUsage(c.Out)
			return exitOK
		case "--brief":
			brief = true
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printAgentPromptUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[i+1])
			i++
		default:
			if strings.HasPrefix(arg, "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
				continue
			}
			fmt.Fprintf(c.Err, "unexpected args for agent prompt: %q\n", strings.Join(args[i:], " "))
			c.printAgentPromptUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "text", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: text, json)\n", outputFormat)
		c.printAgentPromptUsage(c.Err)
		return exitUsage
	}
	if outputFormat == "json" {
		spec := defaultAgentPrompt
		if brief {
			spec = briefAgentPrompt
		}
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "agent.prompt",
			Result: spec,
		})
		return exitOK
	}
	if brief {
		c.printAgentPromptBrief(briefAgentPrompt)
		return exitOK
	}
	c.printAgentPrompt(defaultAgentPrompt)
	return exitOK
}

func (c *CLI) printAgentPrompt(spec agentPromptSpec) {
	fmt.Fprintln(c.Out, "kra agent prompt")
	writeAgentPromptSection(c.Out, "What kra is", spec.WhatKraIs)
	writeAgentPromptSection(c.Out, "Recommended workflow", spec.RecommendedWorkflow)
	writeAgentPromptSection(c.Out, "Automation rules", spec.AutomationRules)
	writeAgentPromptSection(c.Out, "Safe commands", spec.SafeCommands)
	writeAgentPromptSection(c.Out, "Destructive commands", spec.DestructiveCommands)
	writeAgentPromptSection(c.Out, "JSON-first commands", spec.JSONFirstCommands)
	writeAgentPromptSection(c.Out, "Commands to avoid without explicit flags", spec.CommandsToAvoidWithoutFlags)
	writeAgentPromptSection(c.Out, "Recovery basics", spec.RecoveryBasics)
	writeAgentPromptSection(c.Out, "Drill down", spec.DrillDownCommands)
}

func (c *CLI) printAgentPromptBrief(spec agentPromptSpec) {
	for _, line := range spec.WhatKraIs {
		fmt.Fprintln(c.Out, line)
	}
	for _, line := range spec.AutomationRules {
		fmt.Fprintln(c.Out, line)
	}
	for _, line := range spec.SafeCommands {
		fmt.Fprintln(c.Out, line)
	}
	for _, line := range spec.DestructiveCommands {
		fmt.Fprintln(c.Out, line)
	}
	for _, line := range spec.DrillDownCommands {
		fmt.Fprintln(c.Out, line)
	}
}

func writeAgentPromptSection(w io.Writer, title string, items []string) {
	fmt.Fprintf(w, "\n%s:\n", title)
	for _, item := range items {
		fmt.Fprintf(w, "- %s\n", item)
	}
}
