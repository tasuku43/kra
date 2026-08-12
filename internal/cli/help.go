package cli

import (
	"fmt"
	"io"
	"strings"
)

type helpSpec struct {
	Command    string             `json:"command"`
	Summary    string             `json:"summary"`
	Automation helpAutomationSpec `json:"automation"`
	Examples   []helpExample      `json:"examples,omitempty"`
	Errors     []helpError        `json:"errors,omitempty"`
}

type helpAutomationSpec struct {
	NonInteractive             bool     `json:"non_interactive"`
	JSONSupported              bool     `json:"json_supported"`
	Unsafe                     bool     `json:"unsafe"`
	Idempotent                 bool     `json:"idempotent"`
	RequiredFlagsForAutomation []string `json:"required_flags_for_automation,omitempty"`
	AvoidFlagsInAutomation     []string `json:"avoid_flags_in_automation,omitempty"`
	PromptsWhen                []string `json:"prompts_when,omitempty"`
	RecommendedInvocation      string   `json:"recommended_invocation"`
}

type helpExample struct {
	Title   string `json:"title"`
	Command string `json:"command"`
}

type helpError struct {
	Code     string `json:"code"`
	When     string `json:"when"`
	Recovery string `json:"recovery"`
}

type helpDoc struct {
	Path       []string
	PrintHuman func(*CLI, io.Writer)
	Agent      helpSpec
}

var helpDocs = []helpDoc{
	{
		Path: []string{"init"},
		PrintHuman: func(c *CLI, w io.Writer) {
			c.printInitUsage(w)
		},
		Agent: helpSpec{
			Command: "kra init",
			Summary: "Initialize KRA_ROOT and select the current context.",
			Automation: helpAutomationSpec{
				NonInteractive:             true,
				JSONSupported:              true,
				Unsafe:                     false,
				Idempotent:                 false,
				RequiredFlagsForAutomation: []string{"--root <path>", "--context <name>"},
				PromptsWhen: []string{
					"--root is omitted in a TTY session",
					"--context is omitted in a TTY session",
				},
				RecommendedInvocation: "kra init --root <path> --context <name> --format json",
			},
			Examples: []helpExample{
				{Title: "Automation-safe initialization", Command: "kra init --root ~/kra --context default --format json"},
			},
			Errors: []helpError{
				{Code: "invalid_argument", When: "--root is omitted in --format json mode", Recovery: "pass --root <path>"},
				{Code: "invalid_argument", When: "--context is omitted in --format json mode", Recovery: "pass --context <name>"},
			},
		},
	},
	{
		Path: []string{"doctor"},
		PrintHuman: func(c *CLI, w io.Writer) {
			c.printDoctorUsage(w)
		},
		Agent: helpSpec{
			Command: "kra doctor",
			Summary: "Diagnose current KRA_ROOT health and optionally run remediation.",
			Automation: helpAutomationSpec{
				NonInteractive:             true,
				JSONSupported:              true,
				Unsafe:                     false,
				Idempotent:                 true,
				RequiredFlagsForAutomation: []string{"--format json"},
				PromptsWhen:                nil,
				RecommendedInvocation:      "kra doctor --format json",
			},
			Examples: []helpExample{
				{Title: "Read-only health check", Command: "kra doctor --format json"},
				{Title: "Plan remediation", Command: "kra doctor --fix --plan --format json"},
			},
			Errors: []helpError{
				{Code: "invalid_argument", When: "--fix is used without exactly one of --plan/--apply", Recovery: "pass one of --plan or --apply together with --fix"},
				{Code: "not_found", When: "current working directory is not inside a KRA_ROOT", Recovery: "run from a root or initialize one with kra init"},
			},
		},
	},
	{
		Path: []string{"ws", "create"},
		PrintHuman: func(c *CLI, w io.Writer) {
			c.printWSCreateUsage(w)
		},
		Agent: helpSpec{
			Command: "kra ws create",
			Summary: "Create one workspace from a template and write workspace metadata.",
			Automation: helpAutomationSpec{
				NonInteractive:             true,
				JSONSupported:              true,
				Unsafe:                     false,
				Idempotent:                 false,
				RequiredFlagsForAutomation: []string{"--id <id> or positional <id>", "--no-prompt", "--format json"},
				AvoidFlagsInAutomation:     []string{"interactive title prompt"},
				PromptsWhen: []string{
					"--title is omitted for non-Jira create and --no-prompt is not set",
				},
				RecommendedInvocation: "kra ws create --id <id> --no-prompt --format json",
			},
			Examples: []helpExample{
				{Title: "Create a workspace without prompts", Command: "kra ws create --id TASK-123 --no-prompt --format json"},
			},
			Errors: []helpError{
				{Code: "invalid_argument", When: "--format json is used without an explicit id", Recovery: "pass --id <id> or positional <id>"},
			},
		},
	},
	{
		Path: []string{"ws", "open"},
		PrintHuman: func(c *CLI, w io.Writer) {
			c.printWSOpenUsage(w)
		},
		Agent: helpSpec{
			Command: "kra ws open",
			Summary: "Open one or more workspace runtimes or sync cwd fallback when cmux is unavailable.",
			Automation: helpAutomationSpec{
				NonInteractive:             true,
				JSONSupported:              true,
				Unsafe:                     false,
				Idempotent:                 false,
				RequiredFlagsForAutomation: []string{"one explicit target selector", "--format json"},
				AvoidFlagsInAutomation:     []string{"--select", "--multi-select"},
				PromptsWhen: []string{
					"--select or --multi-select is used",
				},
				RecommendedInvocation: "kra ws open --id <id> --format json",
			},
			Examples: []helpExample{
				{Title: "Open one known workspace", Command: "kra ws open --id TASK-123 --format json"},
			},
			Errors: []helpError{
				{Code: "invalid_argument", When: "no target selector is provided in automation", Recovery: "pass --id <id>, --current, or --all"},
			},
		},
	},
	{
		Path: []string{"ws", "add-repo"},
		PrintHuman: func(c *CLI, w io.Writer) {
			c.printWSAddRepoUsage(w)
		},
		Agent: helpSpec{
			Command: "kra ws add-repo",
			Summary: "Attach repo pool entries to a workspace as worktrees.",
			Automation: helpAutomationSpec{
				NonInteractive:             true,
				JSONSupported:              true,
				Unsafe:                     false,
				Idempotent:                 false,
				RequiredFlagsForAutomation: []string{"--format json", "--id <workspace-id>", "--repo <repo-key> or --preset <name>", "--yes"},
				AvoidFlagsInAutomation:     []string{"--select"},
				PromptsWhen: []string{
					"--format human is used and repo/branch/base-ref must be selected or confirmed",
				},
				RecommendedInvocation: "kra ws add-repo --format json --id <workspace-id> --repo <repo-key> --branch <name> --base-ref <origin/branch> --yes",
			},
			Examples: []helpExample{
				{Title: "Attach one repo with explicit branch plan", Command: "kra ws add-repo --format json --id TASK-123 --repo api --branch task-123 --base-ref origin/main --yes"},
			},
			Errors: []helpError{
				{Code: "invalid_argument", When: "required repo selection data is omitted in JSON mode", Recovery: "pass --repo <repo-key> or --preset <name> with explicit workspace id"},
			},
		},
	},
	{
		Path: []string{"ws", "close"},
		PrintHuman: func(c *CLI, w io.Writer) {
			c.printWSCloseUsage(w)
		},
		Agent: helpSpec{
			Command: "kra ws close",
			Summary: "Archive an active workspace after worktree/risk checks.",
			Automation: helpAutomationSpec{
				NonInteractive:             true,
				JSONSupported:              true,
				Unsafe:                     false,
				Idempotent:                 false,
				RequiredFlagsForAutomation: []string{"--id <id>", "--format json"},
				AvoidFlagsInAutomation:     []string{"--select", "--multi-select"},
				PromptsWhen: []string{
					"--format human is used and risk confirmation is required without --force",
				},
				RecommendedInvocation: "kra ws close --id <id> --force --format json",
			},
			Examples: []helpExample{
				{Title: "Dry-run close preflight", Command: "kra ws close --dry-run --format json --id TASK-123"},
				{Title: "Close with explicit confirmation bypass", Command: "kra ws close --id TASK-123 --force --format json"},
			},
			Errors: []helpError{
				{Code: "invalid_argument", When: "--format json is used without --id", Recovery: "pass --id <id>"},
				{Code: "confirmation_required", When: "risk confirmation is needed in human mode", Recovery: "inspect dry-run output, then rerun with --force if acceptable"},
			},
		},
	},
	{
		Path: []string{"ws", "reopen"},
		PrintHuman: func(c *CLI, w io.Writer) {
			c.printWSReopenUsage(w)
		},
		Agent: helpSpec{
			Command: "kra ws reopen",
			Summary: "Move one archived workspace back to active workspaces and recreate worktrees.",
			Automation: helpAutomationSpec{
				NonInteractive:             true,
				JSONSupported:              true,
				Unsafe:                     false,
				Idempotent:                 false,
				RequiredFlagsForAutomation: []string{"--id <id>", "--dry-run", "--format json"},
				AvoidFlagsInAutomation:     []string{"--select"},
				PromptsWhen: []string{
					"interactive archived selector is used",
				},
				RecommendedInvocation: "kra ws reopen --dry-run --format json --id <id>",
			},
			Examples: []helpExample{
				{Title: "Preflight reopen in automation", Command: "kra ws reopen --dry-run --format json --id TASK-123"},
			},
			Errors: []helpError{
				{Code: "invalid_argument", When: "--id is omitted", Recovery: "pass --id <id>"},
				{Code: "invalid_argument", When: "--format json is used without --dry-run", Recovery: "use --dry-run for machine-readable preflight or switch to human mode for apply"},
			},
		},
	},
	{
		Path: []string{"ws", "purge"},
		PrintHuman: func(c *CLI, w io.Writer) {
			c.printWSPurgeUsage(w)
		},
		Agent: helpSpec{
			Command: "kra ws purge",
			Summary: "Permanently delete an archived or active workspace and its worktrees.",
			Automation: helpAutomationSpec{
				NonInteractive:             true,
				JSONSupported:              true,
				Unsafe:                     true,
				Idempotent:                 false,
				RequiredFlagsForAutomation: []string{"--id <id>", "--dry-run", "--format json"},
				AvoidFlagsInAutomation:     []string{"--select"},
				PromptsWhen: []string{
					"--no-prompt is omitted in human mode",
					"extra confirmation is required when purging active risky workspaces",
				},
				RecommendedInvocation: "kra ws purge --dry-run --format json --id <id>",
			},
			Examples: []helpExample{
				{Title: "Safety preflight before destructive purge", Command: "kra ws purge --dry-run --format json --id TASK-123"},
			},
			Errors: []helpError{
				{Code: "invalid_argument", When: "--dry-run is omitted in JSON mode", Recovery: "use --dry-run for machine-readable preflight"},
				{Code: "confirmation_required", When: "purge is run without required confirmations", Recovery: "rerun with explicit confirmation flags only after checking dry-run output"},
			},
		},
	},
}

func (c *CLI) runHelp(args []string) int {
	mode := "human"
	outputFormat := "text"
	target := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help":
			c.printHelpUsage(c.Out)
			return exitOK
		case "--mode":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--mode requires a value")
				c.printHelpUsage(c.Err)
				return exitUsage
			}
			mode = strings.TrimSpace(args[i+1])
			i++
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printHelpUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[i+1])
			i++
		default:
			if strings.HasPrefix(arg, "--mode=") {
				mode = strings.TrimSpace(strings.TrimPrefix(arg, "--mode="))
				continue
			}
			if strings.HasPrefix(arg, "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
				continue
			}
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(c.Err, "unknown flag for help: %q\n", arg)
				c.printHelpUsage(c.Err)
				return exitUsage
			}
			target = append(target, arg)
		}
	}
	switch mode {
	case "human", "agent":
	default:
		fmt.Fprintf(c.Err, "unsupported --mode: %q (supported: human, agent)\n", mode)
		c.printHelpUsage(c.Err)
		return exitUsage
	}
	switch outputFormat {
	case "text", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: text, json)\n", outputFormat)
		c.printHelpUsage(c.Err)
		return exitUsage
	}
	if len(target) == 0 {
		if mode == "human" && outputFormat == "text" {
			c.printRootUsage(c.Out)
			return exitOK
		}
		fmt.Fprintln(c.Err, "help requires a command path such as `kra help ws close --mode agent`")
		c.printHelpUsage(c.Err)
		return exitUsage
	}
	doc, ok := lookupHelpDoc(target)
	if !ok {
		fmt.Fprintf(c.Err, "unknown help target: %q\n", strings.Join(target, " "))
		c.printHelpUsage(c.Err)
		return exitUsage
	}
	if mode == "human" {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     true,
				Action: "help",
				Result: map[string]any{
					"mode":    "human",
					"command": doc.Agent.Command,
					"summary": doc.Agent.Summary,
				},
			})
			return exitOK
		}
		doc.PrintHuman(c, c.Out)
		return exitOK
	}
	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     true,
			Action: "help",
			Result: map[string]any{
				"mode": "agent",
				"spec": doc.Agent,
			},
		})
		return exitOK
	}
	c.printAgentHelp(c.Out, doc.Agent)
	return exitOK
}

func lookupHelpDoc(target []string) (helpDoc, bool) {
	joined := strings.Join(target, " ")
	for _, doc := range helpDocs {
		if strings.Join(doc.Path, " ") == joined {
			return doc, true
		}
	}
	return helpDoc{}, false
}

func (c *CLI) printAgentHelp(w io.Writer, spec helpSpec) {
	fmt.Fprintf(w, "Command: %s\n", spec.Command)
	fmt.Fprintf(w, "Summary: %s\n", spec.Summary)
	fmt.Fprintf(w, "non_interactive: %t\n", spec.Automation.NonInteractive)
	fmt.Fprintf(w, "json_supported: %t\n", spec.Automation.JSONSupported)
	fmt.Fprintf(w, "unsafe: %t\n", spec.Automation.Unsafe)
	fmt.Fprintf(w, "idempotent: %t\n", spec.Automation.Idempotent)
	writeHelpList(w, "required_flags_for_automation", spec.Automation.RequiredFlagsForAutomation)
	writeHelpList(w, "avoid_flags_in_automation", spec.Automation.AvoidFlagsInAutomation)
	writeHelpList(w, "prompts_when", spec.Automation.PromptsWhen)
	fmt.Fprintf(w, "recommended_invocation: %s\n", spec.Automation.RecommendedInvocation)
	writeHelpExamples(w, spec.Examples)
	writeHelpErrors(w, spec.Errors)
}

func writeHelpList(w io.Writer, label string, values []string) {
	fmt.Fprintf(w, "%s:\n", label)
	if len(values) == 0 {
		fmt.Fprintln(w, "- none")
		return
	}
	for _, value := range values {
		fmt.Fprintf(w, "- %s\n", value)
	}
}

func writeHelpExamples(w io.Writer, values []helpExample) {
	fmt.Fprintln(w, "examples:")
	if len(values) == 0 {
		fmt.Fprintln(w, "- none")
		return
	}
	for _, value := range values {
		fmt.Fprintf(w, "- %s: %s\n", value.Title, value.Command)
	}
}

func writeHelpErrors(w io.Writer, values []helpError) {
	fmt.Fprintln(w, "common_failures:")
	if len(values) == 0 {
		fmt.Fprintln(w, "- none")
		return
	}
	for _, value := range values {
		fmt.Fprintf(w, "- %s: when=%s; recovery=%s\n", value.Code, value.When, value.Recovery)
	}
}
