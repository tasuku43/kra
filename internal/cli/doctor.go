package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tasuku43/kra/internal/infra/paths"
	"github.com/tasuku43/kra/internal/infra/stateregistry"
)

type doctorSeverity string

const (
	doctorSeverityWarn  doctorSeverity = "warn"
	doctorSeverityError doctorSeverity = "error"
)

type doctorFinding struct {
	Severity doctorSeverity `json:"severity"`
	Code     string         `json:"code"`
	Target   string         `json:"target"`
	Message  string         `json:"message"`
}

type doctorReport struct {
	Root     string          `json:"root"`
	OK       int             `json:"ok"`
	Warn     int             `json:"warn"`
	Error    int             `json:"error"`
	Findings []doctorFinding `json:"findings,omitempty"`
}

func (c *CLI) runDoctor(args []string) int {
	outputFormat := "human"
	withFix := false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printDoctorUsage(c.Out)
			return exitOK
		case "--fix":
			withFix = true
			args = args[1:]
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printDoctorUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for doctor: %q\n", args[0])
			c.printDoctorUsage(c.Err)
			return exitUsage
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(c.Err, "unexpected args for doctor: %q\n", strings.Join(args, " "))
		c.printDoctorUsage(c.Err)
		return exitUsage
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printDoctorUsage(c.Err)
		return exitUsage
	}
	if withFix {
		msg := "--fix is reserved and not supported yet"
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "doctor",
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: msg,
				},
			})
			return exitUsage
		}
		fmt.Fprintln(c.Err, msg)
		c.printDoctorUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "doctor",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("get working dir: %v", err),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "doctor",
				Error: &cliJSONError{
					Code:    "not_found",
					Message: fmt.Sprintf("resolve KRA_ROOT: %v", err),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "resolve KRA_ROOT: %v\n", err)
		return exitError
	}
	if err := c.ensureDebugLog(root, "doctor"); err != nil {
		fmt.Fprintf(c.Err, "enable debug logging: %v\n", err)
	}
	c.debugf("run doctor format=%s", outputFormat)

	report := runDoctorChecks(root)
	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     report.Error == 0,
			Action: "doctor",
			Result: report,
		})
		if report.Error > 0 {
			return exitError
		}
		return exitOK
	}

	useColorOut := writerSupportsColor(c.Out)
	lines := []string{
		fmt.Sprintf("%s: %s", styleAccent("root", useColorOut), report.Root),
		fmt.Sprintf("%s: %d", styleSuccess("ok", useColorOut), report.OK),
		fmt.Sprintf("%s: %d", styleWarn("warn", useColorOut), report.Warn),
		fmt.Sprintf("%s: %d", styleError("error", useColorOut), report.Error),
	}
	if len(report.Findings) > 0 {
		lines = append(lines, "findings:")
		for _, sev := range []doctorSeverity{doctorSeverityError, doctorSeverityWarn} {
			var group []doctorFinding
			for _, f := range report.Findings {
				if f.Severity == sev {
					group = append(group, f)
				}
			}
			if len(group) == 0 {
				continue
			}
			lines = append(lines, fmt.Sprintf("  %s:", strings.ToUpper(string(sev))))
			for _, f := range group {
				line := fmt.Sprintf("    - [%s] %s: %s", f.Code, f.Target, f.Message)
				if sev == doctorSeverityError {
					line = styleError(line, useColorOut)
				} else {
					line = styleWarn(line, useColorOut)
				}
				lines = append(lines, line)
			}
		}
	}
	body := make([]string, 0, len(lines))
	for _, line := range lines {
		body = append(body, fmt.Sprintf("%s%s", uiIndent, line))
	}
	printSection(c.Out, renderResultTitle(useColorOut), body, sectionRenderOptions{
		blankAfterHeading: false,
		trailingBlank:     true,
	})
	if report.Error > 0 {
		return exitError
	}
	return exitOK
}

func runDoctorChecks(root string) doctorReport {
	report := doctorReport{
		Root:     root,
		Findings: make([]doctorFinding, 0),
	}
	addOK := func() {
		report.OK++
	}
	addWarn := func(code string, target string, message string) {
		report.Warn++
		report.Findings = append(report.Findings, doctorFinding{
			Severity: doctorSeverityWarn,
			Code:     code,
			Target:   target,
			Message:  message,
		})
	}
	addError := func(code string, target string, message string) {
		report.Error++
		report.Findings = append(report.Findings, doctorFinding{
			Severity: doctorSeverityError,
			Code:     code,
			Target:   target,
			Message:  message,
		})
	}

	checkDir := func(name string) {
		p := filepath.Join(root, name)
		fi, err := os.Stat(p)
		if err != nil {
			addError("missing_root_dir", p, "required root directory is missing")
			return
		}
		if !fi.IsDir() {
			addError("invalid_root_entry", p, "required root directory path is not a directory")
			return
		}
		addOK()
	}
	checkDir("workspaces")
	checkDir("archive")

	scanDoctorWorkspaceScope(root, "workspaces", "active", true, addOK, addWarn, addError)
	scanDoctorWorkspaceScope(root, "archive", "archived", false, addOK, addWarn, addError)
	scanDoctorLocks(root, addOK, addWarn)
	scanDoctorRegistry(root, addOK, addWarn)

	slices.SortFunc(report.Findings, func(a, b doctorFinding) int {
		if a.Severity != b.Severity {
			if a.Severity == doctorSeverityError {
				return -1
			}
			return 1
		}
		if a.Code != b.Code {
			return strings.Compare(a.Code, b.Code)
		}
		return strings.Compare(a.Target, b.Target)
	})

	return report
}

func scanDoctorWorkspaceScope(
	root string,
	scopeDir string,
	expectStatus string,
	checkBindingWorktree bool,
	addOK func(),
	addWarn func(code string, target string, message string),
	addError func(code string, target string, message string),
) {
	baseDir := filepath.Join(root, scopeDir)
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		addError("read_scope_dir_failed", baseDir, err.Error())
		return
	}
	addOK()

	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		workspaceID := strings.TrimSpace(ent.Name())
		wsPath := filepath.Join(baseDir, workspaceID)
		if err := validateWorkspaceID(workspaceID); err != nil {
			addWarn("invalid_workspace_id", wsPath, err.Error())
			continue
		}

		meta, err := loadWorkspaceMetaFile(wsPath)
		if err != nil {
			addError("workspace_meta_invalid", filepath.Join(wsPath, workspaceMetaFilename), err.Error())
			continue
		}
		addOK()

		if strings.TrimSpace(meta.Workspace.ID) != workspaceID {
			addWarn("workspace_meta_id_mismatch", filepath.Join(wsPath, workspaceMetaFilename), fmt.Sprintf("workspace.id=%q directory=%q", strings.TrimSpace(meta.Workspace.ID), workspaceID))
		}
		if strings.TrimSpace(meta.Workspace.Status) != expectStatus {
			addWarn("workspace_status_mismatch", filepath.Join(wsPath, workspaceMetaFilename), fmt.Sprintf("status=%q expected=%q", strings.TrimSpace(meta.Workspace.Status), expectStatus))
		}

		aliasSeen := make(map[string]bool, len(meta.ReposRestore))
		for _, r := range meta.ReposRestore {
			alias := strings.TrimSpace(r.Alias)
			if alias == "" {
				addError("repos_restore_alias_empty", filepath.Join(wsPath, workspaceMetaFilename), "repos_restore alias is required")
				continue
			}
			if aliasSeen[alias] {
				addError("repos_restore_alias_duplicate", filepath.Join(wsPath, workspaceMetaFilename), fmt.Sprintf("duplicate alias=%q", alias))
				continue
			}
			aliasSeen[alias] = true

			if !checkBindingWorktree {
				continue
			}
			worktreePath := filepath.Join(wsPath, "repos", alias)
			fi, statErr := os.Stat(worktreePath)
			if statErr != nil {
				if os.IsNotExist(statErr) {
					addWarn("binding_missing_worktree", worktreePath, "repos_restore binding exists but worktree directory is missing")
					continue
				}
				addWarn("worktree_stat_failed", worktreePath, statErr.Error())
				continue
			}
			if !fi.IsDir() {
				addWarn("worktree_not_directory", worktreePath, "worktree path is not a directory")
				continue
			}
			addOK()
		}

		reposDir := filepath.Join(wsPath, "repos")
		repoEntries, readErr := os.ReadDir(reposDir)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			addWarn("repos_dir_read_failed", reposDir, readErr.Error())
			continue
		}
		addOK()
		if !checkBindingWorktree {
			for _, repoEnt := range repoEntries {
				if repoEnt.IsDir() {
					addWarn("archived_worktree_exists", filepath.Join(reposDir, repoEnt.Name()), "archived workspace should not keep live worktree directories")
				}
			}
			continue
		}
		for _, repoEnt := range repoEntries {
			if !repoEnt.IsDir() {
				continue
			}
			alias := strings.TrimSpace(repoEnt.Name())
			if alias == "" {
				continue
			}
			if !aliasSeen[alias] {
				addWarn("worktree_missing_binding", filepath.Join(reposDir, alias), "worktree exists but repos_restore metadata is missing")
			}
		}
	}
}

func scanDoctorLocks(
	root string,
	addOK func(),
	addWarn func(code string, target string, message string),
) {
	lockDir := filepath.Join(root, ".kra", "locks")
	entries, err := os.ReadDir(lockDir)
	if err != nil {
		if os.IsNotExist(err) {
			addOK()
			return
		}
		addWarn("lock_dir_read_failed", lockDir, err.Error())
		return
	}
	addOK()

	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".lock") {
			continue
		}
		lockPath := filepath.Join(lockDir, ent.Name())
		raw, readErr := os.ReadFile(lockPath)
		if readErr != nil {
			addWarn("lock_read_failed", lockPath, readErr.Error())
			continue
		}
		pid, ok := parseWorkspaceAddRepoLockPID(string(raw))
		if !ok || pid <= 0 {
			addWarn("lock_pid_missing_or_invalid", lockPath, "lock file does not contain valid pid metadata")
			continue
		}
		if !isProcessAlive(pid) {
			addWarn("stale_lock", lockPath, fmt.Sprintf("owner pid=%d is not alive", pid))
			continue
		}
		addOK()
	}
}

func scanDoctorRegistry(
	root string,
	addOK func(),
	addWarn func(code string, target string, message string),
) {
	registryPath, err := stateregistry.Path()
	if err != nil {
		addWarn("registry_path_resolve_failed", "KRA_HOME", err.Error())
		return
	}
	entries, err := stateregistry.Load(registryPath)
	if err != nil {
		addWarn("registry_load_failed", registryPath, err.Error())
		return
	}
	addOK()

	cleanRoot, absErr := filepath.Abs(root)
	if absErr != nil {
		addWarn("root_abs_resolve_failed", root, absErr.Error())
		return
	}
	found := false
	for _, e := range entries {
		if filepath.Clean(strings.TrimSpace(e.RootPath)) == filepath.Clean(cleanRoot) {
			found = true
			break
		}
	}
	if !found {
		addWarn("root_not_registered", registryPath, "current root is missing in root-registry")
		return
	}
	addOK()
}
