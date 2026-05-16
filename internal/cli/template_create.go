package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/kra/internal/app/templatecmd"
	"github.com/tasuku43/kra/internal/infra/paths"
)

func (c *CLI) runTemplateCreate(args []string) int {
	templateName := ""
	fromTemplate := ""
	promptedName := false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printTemplateCreateUsage(c.Out)
			return exitOK
		case "--name":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--name requires a value")
				c.printTemplateCreateUsage(c.Err)
				return exitUsage
			}
			templateName = strings.TrimSpace(args[1])
			args = args[2:]
		case "--from":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--from requires a value")
				c.printTemplateCreateUsage(c.Err)
				return exitUsage
			}
			fromTemplate = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			fmt.Fprintf(c.Err, "unknown flag for template create: %q\n", args[0])
			c.printTemplateCreateUsage(c.Err)
			return exitUsage
		}
	}
	if len(args) > 1 {
		fmt.Fprintf(c.Err, "unexpected args for template create: %q\n", strings.Join(args, " "))
		c.printTemplateCreateUsage(c.Err)
		return exitUsage
	}
	if len(args) == 1 {
		if templateName != "" {
			fmt.Fprintln(c.Err, "--name cannot be combined with positional <template>")
			c.printTemplateCreateUsage(c.Err)
			return exitUsage
		}
		templateName = strings.TrimSpace(args[0])
	}
	if templateName == "" {
		useColorErr := writerSupportsColor(c.Err)
		line, err := c.promptLine(renderTemplateCreateNamePrompt(useColorErr))
		if err != nil {
			fmt.Fprintf(c.Err, "read template name: %v\n", err)
			return exitError
		}
		templateName = strings.TrimSpace(line)
		promptedName = true
	}
	if err := validateWorkspaceTemplateName(templateName); err != nil {
		fmt.Fprintln(c.Err, err.Error())
		c.printTemplateCreateUsage(c.Err)
		return exitUsage
	}
	if fromTemplate != "" {
		if err := validateWorkspaceTemplateName(fromTemplate); err != nil {
			fmt.Fprintln(c.Err, fmt.Sprintf("invalid source template name: %v", err))
			c.printTemplateCreateUsage(c.Err)
			return exitUsage
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		fmt.Fprintf(c.Err, "resolve KRA_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "template-create"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run template create name=%q from=%q", templateName, fromTemplate)

	templatePath := ""
	if fromTemplate == "" {
		templatePath, err = createWorkspaceTemplateScaffold(root, templateName)
	} else {
		templatePath, err = createWorkspaceTemplateFromSource(root, templateName, fromTemplate)
	}
	if err != nil {
		printTemplateCreateError(c.Err, writerSupportsColor(c.Err), templateName, fromTemplate, err)
		return exitError
	}
	commitSHA, err := templatecmd.CommitCreate(context.Background(), root, templateName)
	if err != nil {
		printTemplateCreateError(c.Err, writerSupportsColor(c.Err), templateName, fromTemplate, fmt.Errorf("commit create change: %w", err))
		return exitError
	}

	useColorOut := writerSupportsColor(c.Out)
	if promptedName {
		printTemplateCreateInputs(c.Err, writerSupportsColor(c.Err), templateName, fromTemplate)
	}
	lines := []string{
		styleSuccess("Created 1 / 1", useColorOut),
		fmt.Sprintf("%s %s", styleSuccess("✔", useColorOut), templateName),
		styleMuted(fmt.Sprintf("path: %s", templatePath), useColorOut),
	}
	if fromTemplate != "" {
		lines = append(lines, styleMuted(fmt.Sprintf("from: %s", fromTemplate), useColorOut))
	}
	if strings.TrimSpace(commitSHA) != "" {
		lines = append(lines, styleMuted(fmt.Sprintf("commit: %s", shortCommitSHA(commitSHA)), useColorOut))
	}
	if promptedName {
		body := make([]string, 0, len(lines))
		for _, line := range lines {
			body = append(body, fmt.Sprintf("%s%s", uiIndent, line))
		}
		printSection(c.Out, renderResultTitle(useColorOut), body, sectionRenderOptions{
			blankAfterHeading: false,
			trailingBlank:     true,
		})
	} else {
		printResultSection(c.Out, useColorOut, lines...)
	}
	return exitOK
}

func renderTemplateCreateNamePrompt(useColor bool) string {
	return renderTemplateNamePrompt(useColor)
}

func renderTemplateNamePrompt(useColor bool) string {
	lines := renderSectionAtoms(newSectionAtom(styleBold("Inputs:", useColor), []string{
		fmt.Sprintf("%s%s: ", uiIndent, styleAccent("name", useColor)),
	}, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     false,
	}))
	return strings.Join(lines, "\n")
}

func printTemplateCreateInputs(out io.Writer, useColor bool, name string, from string) {
	body := []string{
		fmt.Sprintf("%s%s: %s", uiIndent, styleAccent("name", useColor), name),
	}
	if strings.TrimSpace(from) != "" {
		body = append(body, fmt.Sprintf("%s%s: %s", uiIndent, styleAccent("from", useColor), from))
	}
	printSection(out, styleBold("Inputs:", useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     false,
	})
}

func printTemplateCreateError(out io.Writer, useColor bool, name string, from string, err error) {
	body := []string{
		fmt.Sprintf("%sname: %s", uiIndent, name),
	}
	if strings.TrimSpace(from) != "" {
		body = append(body, fmt.Sprintf("%sfrom: %s", uiIndent, from))
	}
	body = append(body, fmt.Sprintf("%sreason: %s", uiIndent, err.Error()))
	fmt.Fprintln(out)
	printSection(out, styleBold(styleError("Error:", useColor), useColor), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
}

func createWorkspaceTemplateScaffold(root string, name string) (string, error) {
	templatesDir := workspaceTemplatesPath(root)
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		return "", fmt.Errorf("create templates/: %w", err)
	}

	templatePath := workspaceTemplatePath(root, name)
	if st, err := os.Stat(templatePath); err == nil {
		if st.IsDir() {
			return "", fmt.Errorf("template %q already exists: %s", name, templatePath)
		}
		return "", fmt.Errorf("template path already exists and is not a directory: %s", templatePath)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat template path: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(templatePath, "notes"), 0o755); err != nil {
		return "", fmt.Errorf("create template notes/: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(templatePath, "artifacts"), 0o755); err != nil {
		return "", fmt.Errorf("create template artifacts/: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(templatePath, ".cmux"), 0o755); err != nil {
		return "", fmt.Errorf("create template .cmux/: %w", err)
	}
	if err := os.WriteFile(filepath.Join(templatePath, rootAgentsFilename), []byte(defaultWorkspaceTemplateAgentsContent()), 0o644); err != nil {
		return "", fmt.Errorf("write template AGENTS.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(templatePath, ".cmux", "dock.json"), []byte(defaultWorkspaceCMUXDockContent()), 0o644); err != nil {
		return "", fmt.Errorf("write template .cmux/dock.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(templatePath, "tasks.md"), []byte(defaultWorkspaceTasksContent), 0o644); err != nil {
		return "", fmt.Errorf("write template tasks.md: %w", err)
	}
	return templatePath, nil
}

func createWorkspaceTemplateFromSource(root string, name string, from string) (string, error) {
	tmpl, err := resolveWorkspaceTemplate(root, from)
	if err != nil {
		return "", err
	}
	violations, err := validateWorkspaceTemplate(tmpl)
	if err != nil {
		return "", err
	}
	if len(violations) > 0 {
		return "", fmt.Errorf("source template validation failed:\n%s", renderWorkspaceTemplateViolations(violations))
	}

	templatesDir := workspaceTemplatesPath(root)
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		return "", fmt.Errorf("create templates/: %w", err)
	}
	templatePath := workspaceTemplatePath(root, name)
	if st, err := os.Stat(templatePath); err == nil {
		if st.IsDir() {
			return "", fmt.Errorf("template %q already exists: %s", name, templatePath)
		}
		return "", fmt.Errorf("template path already exists and is not a directory: %s", templatePath)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat template path: %w", err)
	}
	if err := os.MkdirAll(templatePath, 0o755); err != nil {
		return "", fmt.Errorf("create template root: %w", err)
	}
	if err := copyWorkspaceTemplate(tmpl, templatePath); err != nil {
		_ = os.RemoveAll(templatePath)
		return "", fmt.Errorf("copy from template %q: %w", from, err)
	}
	return templatePath, nil
}
