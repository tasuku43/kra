package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	workspaceTemplatesDirName    = "templates"
	defaultWorkspaceTemplateName = "default"
)

type workspaceTemplate struct {
	Name string
	Path string
}

type workspaceTemplateViolation struct {
	Template string
	Path     string
	Reason   string
}

func workspaceTemplatesPath(root string) string {
	return filepath.Join(root, workspaceTemplatesDirName)
}

func workspaceTemplatePath(root string, name string) string {
	return filepath.Join(workspaceTemplatesPath(root), name)
}

func validateWorkspaceTemplateName(name string) error {
	if err := validateWorkspaceID(name); err != nil {
		return fmt.Errorf("invalid template name: %w", err)
	}
	return nil
}

func listWorkspaceTemplateNames(root string) ([]string, error) {
	templatesDir := workspaceTemplatesPath(root)
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read templates dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		names = append(names, ent.Name())
	}
	slices.Sort(names)
	return names, nil
}

func resolveWorkspaceTemplate(root string, name string) (workspaceTemplate, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultWorkspaceTemplateName
	}
	if err := validateWorkspaceTemplateName(name); err != nil {
		return workspaceTemplate{}, err
	}

	templatesDir := workspaceTemplatesPath(root)
	templatesStat, err := os.Stat(templatesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return workspaceTemplate{}, fmt.Errorf("templates directory does not exist: %s", templatesDir)
		}
		return workspaceTemplate{}, fmt.Errorf("stat templates directory: %w", err)
	}
	if !templatesStat.IsDir() {
		return workspaceTemplate{}, fmt.Errorf("templates path is not a directory: %s", templatesDir)
	}

	templateDir := workspaceTemplatePath(root, name)
	ent, err := os.Stat(templateDir)
	if err != nil {
		if os.IsNotExist(err) {
			available, listErr := listWorkspaceTemplateNames(root)
			if listErr != nil {
				return workspaceTemplate{}, fmt.Errorf("template %q not found under %s", name, templatesDir)
			}
			if len(available) == 0 {
				return workspaceTemplate{}, fmt.Errorf("template %q not found under %s (no templates available)", name, templatesDir)
			}
			return workspaceTemplate{}, fmt.Errorf("template %q not found under %s (available: %s)", name, templatesDir, strings.Join(available, ", "))
		}
		return workspaceTemplate{}, fmt.Errorf("stat template: %w", err)
	}
	if !ent.IsDir() {
		return workspaceTemplate{}, fmt.Errorf("template %q is not a directory: %s", name, templateDir)
	}
	return workspaceTemplate{Name: name, Path: templateDir}, nil
}

func validateWorkspaceTemplate(tmpl workspaceTemplate) ([]workspaceTemplateViolation, error) {
	violations := make([]workspaceTemplateViolation, 0)
	err := filepath.WalkDir(tmpl.Path, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == tmpl.Path {
			return nil
		}

		rel, err := filepath.Rel(tmpl.Path, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		// Do not allow symlinks in templates to avoid path traversal and env-dependent output.
		if d.Type()&os.ModeSymlink != 0 {
			violations = append(violations, workspaceTemplateViolation{
				Template: tmpl.Name,
				Path:     rel,
				Reason:   "symlink is not allowed in templates",
			})
		}

		switch rel {
		case "repos":
			violations = append(violations, workspaceTemplateViolation{
				Template: tmpl.Name,
				Path:     rel,
				Reason:   `reserved path "repos/" is not allowed`,
			})
			if d.IsDir() {
				return filepath.SkipDir
			}
		case ".git":
			violations = append(violations, workspaceTemplateViolation{
				Template: tmpl.Name,
				Path:     rel,
				Reason:   `reserved path ".git/" is not allowed`,
			})
			if d.IsDir() {
				return filepath.SkipDir
			}
		case workspaceMetaFilename:
			violations = append(violations, workspaceTemplateViolation{
				Template: tmpl.Name,
				Path:     rel,
				Reason:   fmt.Sprintf(`reserved path "%s" is not allowed`, workspaceMetaFilename),
			})
		}

		if d.IsDir() {
			return nil
		}
		if d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			return nil
		}
		violations = append(violations, workspaceTemplateViolation{
			Template: tmpl.Name,
			Path:     rel,
			Reason:   fmt.Sprintf("unsupported file type: %s", info.Mode().Type().String()),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk template %q: %w", tmpl.Name, err)
	}
	slices.SortFunc(violations, func(a, b workspaceTemplateViolation) int {
		if c := strings.Compare(a.Template, b.Template); c != 0 {
			return c
		}
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return strings.Compare(a.Reason, b.Reason)
	})
	return violations, nil
}

func copyWorkspaceTemplate(tmpl workspaceTemplate, wsPath string) error {
	return filepath.WalkDir(tmpl.Path, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == tmpl.Path {
			return nil
		}

		rel, err := filepath.Rel(tmpl.Path, path)
		if err != nil {
			return err
		}
		rel = filepath.Clean(rel)
		target := filepath.Join(wsPath, rel)

		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("template contains symlink: %s", filepath.ToSlash(rel))
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()

		if d.IsDir() {
			return os.MkdirAll(target, mode.Perm())
		}
		if !mode.IsRegular() {
			return fmt.Errorf("unsupported file type in template: %s", filepath.ToSlash(rel))
		}
		return copyTemplateFile(path, target, mode.Perm())
	})
}

func copyTemplateFile(src string, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func renderWorkspaceTemplateViolations(violations []workspaceTemplateViolation) string {
	if len(violations) == 0 {
		return ""
	}
	lines := make([]string, 0, len(violations))
	for _, v := range violations {
		lines = append(lines, fmt.Sprintf("template=%s path=%s: %s", v.Template, v.Path, v.Reason))
	}
	return strings.Join(lines, "\n")
}
