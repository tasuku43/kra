package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/infra/paths"
)

type wsInsightAddOptions struct {
	format    string
	workspace string
	ticket    string
	sessionID string
	what      string
	context   string
	why       string
	next      string
	tags      []string
	approved  bool
}

func (c *CLI) runWSInsight(args []string) int {
	if len(args) == 0 {
		c.printWSInsightUsage(c.Err)
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		c.printWSInsightUsage(c.Out)
		return exitOK
	case "add":
		return c.runWSInsightAdd(args[1:])
	default:
		fmt.Fprintf(c.Err, "unknown command: %q\n", strings.Join(append([]string{"ws", "insight"}, args[0]), " "))
		c.printWSInsightUsage(c.Err)
		return exitUsage
	}
}

func (c *CLI) runWSInsightAdd(args []string) int {
	opts, err := parseWSInsightAddOptions(args)
	if err != nil {
		if errors.Is(err, errHelpRequested) {
			c.printWSInsightAddUsage(c.Out)
			return exitOK
		}
		if opts.format == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:          false,
				Action:      "ws.insight.add",
				WorkspaceID: opts.workspace,
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: err.Error(),
				},
			})
			return exitUsage
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		c.printWSInsightAddUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeWSInsightAddError(opts, "internal_error", fmt.Sprintf("get working dir: %v", err))
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeWSInsightAddError(opts, "not_found", fmt.Sprintf("resolve KRA_ROOT: %v", err))
	}
	if err := c.ensureDebugLog(root, "ws-insight-add"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}

	workspacePath, ok, err := resolveWorkspacePathByID(root, opts.workspace)
	if err != nil {
		return c.writeWSInsightAddError(opts, "internal_error", fmt.Sprintf("resolve workspace path: %v", err))
	}
	if !ok {
		return c.writeWSInsightAddError(opts, "not_found", fmt.Sprintf("workspace not found: %s", opts.workspace))
	}

	createdPath, err := writeWorkspaceInsightDoc(workspacePath, opts)
	if err != nil {
		code := "internal_error"
		if errors.Is(err, os.ErrExist) {
			code = "conflict"
		}
		return c.writeWSInsightAddError(opts, code, fmt.Sprintf("write insight doc: %v", err))
	}

	if opts.format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      "ws.insight.add",
			WorkspaceID: opts.workspace,
			Result: map[string]any{
				"path": createdPath,
				"kind": "insight",
			},
		})
		return exitOK
	}

	useColorOut := writerSupportsColor(c.Out)
	printResultSection(
		c.Out,
		useColorOut,
		styleSuccess(fmt.Sprintf("Insight saved: %s", createdPath), useColorOut),
		styleMuted(fmt.Sprintf("workspace: %s", opts.workspace), useColorOut),
		styleMuted(fmt.Sprintf("ticket: %s", opts.ticket), useColorOut),
	)
	return exitOK
}

func parseWSInsightAddOptions(args []string) (wsInsightAddOptions, error) {
	opts := wsInsightAddOptions{
		format: "human",
		tags:   []string{},
	}
	rest := append([]string{}, args...)
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		arg := rest[0]
		switch {
		case arg == "-h" || arg == "--help" || arg == "help":
			return wsInsightAddOptions{}, errHelpRequested
		case arg == "--approved":
			opts.approved = true
			rest = rest[1:]
		case strings.HasPrefix(arg, "--id="):
			opts.workspace = strings.TrimSpace(strings.TrimPrefix(arg, "--id="))
			rest = rest[1:]
		case arg == "--id":
			if len(rest) < 2 {
				return opts, fmt.Errorf("--id requires a value")
			}
			opts.workspace = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--ticket="):
			opts.ticket = strings.TrimSpace(strings.TrimPrefix(arg, "--ticket="))
			rest = rest[1:]
		case arg == "--ticket":
			if len(rest) < 2 {
				return opts, fmt.Errorf("--ticket requires a value")
			}
			opts.ticket = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--session-id="):
			opts.sessionID = strings.TrimSpace(strings.TrimPrefix(arg, "--session-id="))
			rest = rest[1:]
		case arg == "--session-id":
			if len(rest) < 2 {
				return opts, fmt.Errorf("--session-id requires a value")
			}
			opts.sessionID = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--what="):
			opts.what = strings.TrimSpace(strings.TrimPrefix(arg, "--what="))
			rest = rest[1:]
		case arg == "--what":
			if len(rest) < 2 {
				return opts, fmt.Errorf("--what requires a value")
			}
			opts.what = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--context="):
			opts.context = strings.TrimSpace(strings.TrimPrefix(arg, "--context="))
			rest = rest[1:]
		case arg == "--context":
			if len(rest) < 2 {
				return opts, fmt.Errorf("--context requires a value")
			}
			opts.context = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--why="):
			opts.why = strings.TrimSpace(strings.TrimPrefix(arg, "--why="))
			rest = rest[1:]
		case arg == "--why":
			if len(rest) < 2 {
				return opts, fmt.Errorf("--why requires a value")
			}
			opts.why = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--next="):
			opts.next = strings.TrimSpace(strings.TrimPrefix(arg, "--next="))
			rest = rest[1:]
		case arg == "--next":
			if len(rest) < 2 {
				return opts, fmt.Errorf("--next requires a value")
			}
			opts.next = strings.TrimSpace(rest[1])
			rest = rest[2:]
		case strings.HasPrefix(arg, "--tag="):
			tag := strings.TrimSpace(strings.TrimPrefix(arg, "--tag="))
			if tag != "" {
				opts.tags = append(opts.tags, tag)
			}
			rest = rest[1:]
		case arg == "--tag":
			if len(rest) < 2 {
				return opts, fmt.Errorf("--tag requires a value")
			}
			tag := strings.TrimSpace(rest[1])
			if tag != "" {
				opts.tags = append(opts.tags, tag)
			}
			rest = rest[2:]
		case strings.HasPrefix(arg, "--format="):
			opts.format = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
			rest = rest[1:]
		case arg == "--format":
			if len(rest) < 2 {
				return opts, fmt.Errorf("--format requires a value")
			}
			opts.format = strings.TrimSpace(rest[1])
			rest = rest[2:]
		default:
			return opts, fmt.Errorf("unknown flag for ws insight add: %q", arg)
		}
	}
	if len(rest) > 0 {
		return opts, fmt.Errorf("unexpected args for ws insight add: %q", strings.Join(rest, " "))
	}
	switch opts.format {
	case "human", "json":
	default:
		return opts, fmt.Errorf("unsupported --format: %q (supported: human, json)", opts.format)
	}
	if err := validateWorkspaceID(opts.workspace); err != nil {
		return opts, fmt.Errorf("invalid --id: %w", err)
	}
	if strings.TrimSpace(opts.ticket) == "" {
		return opts, fmt.Errorf("--ticket is required")
	}
	if strings.TrimSpace(opts.sessionID) == "" {
		return opts, fmt.Errorf("--session-id is required")
	}
	if strings.TrimSpace(opts.what) == "" {
		return opts, fmt.Errorf("--what is required")
	}
	if !opts.approved {
		return opts, fmt.Errorf("--approved is required to persist insight")
	}
	opts.context = fallbackInsightText(opts.context)
	opts.why = fallbackInsightText(opts.why)
	opts.next = fallbackInsightText(opts.next)
	opts.tags = normalizeInsightTags(opts.tags)
	return opts, nil
}

func fallbackInsightText(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "-"
	}
	return trimmed
}

func normalizeInsightTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func writeWorkspaceInsightDoc(workspacePath string, opts wsInsightAddOptions) (string, error) {
	dir := filepath.Join(workspacePath, "worklog", "insights")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create insights dir: %w", err)
	}
	timestamp := time.Now().UTC().Format("20060102-150405")
	baseSlug := slugifyInsight(opts.what)
	if baseSlug == "" {
		baseSlug = "insight"
	}
	for attempt := 0; attempt < 100; attempt++ {
		slug := baseSlug
		if attempt > 0 {
			slug = fmt.Sprintf("%s-%d", baseSlug, attempt+1)
		}
		path := filepath.Join(dir, fmt.Sprintf("%s-%s.md", timestamp, slug))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", err
		}
		content := renderInsightDocMarkdown(opts)
		if _, writeErr := f.WriteString(content); writeErr != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return "", writeErr
		}
		if closeErr := f.Close(); closeErr != nil {
			_ = os.Remove(path)
			return "", closeErr
		}
		return path, nil
	}
	return "", fmt.Errorf("unique insight filename conflict")
}

func slugifyInsight(raw string) string {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return "insight"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range text {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "insight"
	}
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-")
	}
	if slug == "" {
		return "insight"
	}
	return slug
}

func renderInsightDocMarkdown(opts wsInsightAddOptions) string {
	createdAt := time.Now().UTC().Format(time.RFC3339)
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("kind: insight\n")
	b.WriteString(fmt.Sprintf("ticket: %q\n", opts.ticket))
	b.WriteString(fmt.Sprintf("workspace: %q\n", opts.workspace))
	b.WriteString(fmt.Sprintf("created_at: %q\n", createdAt))
	b.WriteString(fmt.Sprintf("session_id: %q\n", opts.sessionID))
	if len(opts.tags) == 0 {
		b.WriteString("tags: []\n")
	} else {
		b.WriteString("tags:\n")
		for _, tag := range opts.tags {
			b.WriteString(fmt.Sprintf("  - %q\n", tag))
		}
	}
	b.WriteString("---\n\n")
	b.WriteString("## Context\n\n")
	b.WriteString(opts.context)
	b.WriteString("\n\n## What happened\n\n")
	b.WriteString(strings.TrimSpace(opts.what))
	b.WriteString("\n\n## Why it matters\n\n")
	b.WriteString(opts.why)
	b.WriteString("\n\n## Next reuse\n\n")
	b.WriteString(opts.next)
	b.WriteString("\n")
	return b.String()
}

func (c *CLI) writeWSInsightAddError(opts wsInsightAddOptions, code string, message string) int {
	if opts.format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "ws.insight.add",
			WorkspaceID: opts.workspace,
			Error: &cliJSONError{
				Code:    code,
				Message: message,
			},
		})
		return exitError
	}
	fmt.Fprintf(c.Err, "%s\n", message)
	return exitError
}
