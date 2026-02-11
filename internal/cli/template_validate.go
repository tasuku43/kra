package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/gionx/internal/infra/paths"
)

type templateValidationReport struct {
	TemplateName string
	Violations   []workspaceTemplateViolation
}

func (c *CLI) runTemplateValidate(args []string) int {
	targetName := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printTemplateValidateUsage(c.Out)
			return exitOK
		case "--name":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--name requires a value")
				c.printTemplateValidateUsage(c.Err)
				return exitUsage
			}
			targetName = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			fmt.Fprintf(c.Err, "unknown flag for template validate: %q\n", args[0])
			c.printTemplateValidateUsage(c.Err)
			return exitUsage
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(c.Err, "unexpected args for template validate: %q\n", strings.Join(args, " "))
		c.printTemplateValidateUsage(c.Err)
		return exitUsage
	}
	if targetName != "" {
		if err := validateWorkspaceTemplateName(targetName); err != nil {
			fmt.Fprintln(c.Err, err.Error())
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
		fmt.Fprintf(c.Err, "resolve GIONX_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "template-validate"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run template validate name=%q", targetName)

	reports, err := validateWorkspaceTemplates(root, targetName)
	if err != nil {
		fmt.Fprintf(c.Err, "validate templates: %v\n", err)
		return exitError
	}

	total := len(reports)
	validated := 0
	flatViolations := make([]workspaceTemplateViolation, 0)
	for _, r := range reports {
		if len(r.Violations) == 0 {
			validated++
			continue
		}
		flatViolations = append(flatViolations, r.Violations...)
	}

	useColorOut := writerSupportsColor(c.Out)
	if len(flatViolations) == 0 {
		lines := []string{
			styleSuccess(fmt.Sprintf("Validated %d / %d", validated, total), useColorOut),
		}
		for _, r := range reports {
			lines = append(lines, fmt.Sprintf("✔ %s", r.TemplateName))
		}
		printResultSection(c.Out, useColorOut, lines...)
		return exitOK
	}

	lines := []string{
		styleError(fmt.Sprintf("Validated %d / %d", validated, total), useColorOut),
		styleError(fmt.Sprintf("Violations: %d", len(flatViolations)), useColorOut),
	}
	printResultSection(c.Out, useColorOut, lines...)

	fmt.Fprintln(c.Err, "template validation failed:")
	for _, v := range flatViolations {
		fmt.Fprintf(c.Err, "  - template=%s path=%s: %s\n", v.Template, v.Path, v.Reason)
	}
	return exitError
}

func validateWorkspaceTemplates(root string, targetName string) ([]templateValidationReport, error) {
	if strings.TrimSpace(targetName) != "" {
		tmpl, err := resolveWorkspaceTemplate(root, targetName)
		if err != nil {
			return nil, err
		}
		violations, err := validateWorkspaceTemplate(tmpl)
		if err != nil {
			return nil, err
		}
		return []templateValidationReport{{
			TemplateName: tmpl.Name,
			Violations:   violations,
		}}, nil
	}

	templatesDir := workspaceTemplatesPath(root)
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("templates directory does not exist: %s", templatesDir)
		}
		return nil, fmt.Errorf("read templates directory: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no templates found under %s", templatesDir)
	}

	reports := make([]templateValidationReport, 0, len(entries))
	for _, ent := range entries {
		name := ent.Name()
		violations := make([]workspaceTemplateViolation, 0)
		if !ent.IsDir() {
			violations = append(violations, workspaceTemplateViolation{
				Template: name,
				Path:     ".",
				Reason:   "template entry must be a directory",
			})
			reports = append(reports, templateValidationReport{
				TemplateName: name,
				Violations:   violations,
			})
			continue
		}
		if err := validateWorkspaceTemplateName(name); err != nil {
			violations = append(violations, workspaceTemplateViolation{
				Template: name,
				Path:     ".",
				Reason:   err.Error(),
			})
		}

		tmpl := workspaceTemplate{
			Name: name,
			Path: filepath.Join(templatesDir, name),
		}
		contentViolations, err := validateWorkspaceTemplate(tmpl)
		if err != nil {
			return nil, err
		}
		violations = append(violations, contentViolations...)
		reports = append(reports, templateValidationReport{
			TemplateName: name,
			Violations:   violations,
		})
	}
	return reports, nil
}
