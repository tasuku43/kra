package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/infra/gitutil"
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

type doctorFixAction struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type doctorFixSummary struct {
	Planned int `json:"planned"`
	Applied int `json:"applied"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

type doctorFixResult struct {
	Root    string            `json:"root"`
	Mode    string            `json:"mode"`
	Summary doctorFixSummary  `json:"summary"`
	Actions []doctorFixAction `json:"actions"`
}

func (c *CLI) runDoctor(args []string) int {
	outputFormat := "human"
	withFix := false
	fixPlan := false
	fixApply := false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printDoctorUsage(c.Out)
			return exitOK
		case "--fix":
			withFix = true
			args = args[1:]
		case "--plan":
			fixPlan = true
			args = args[1:]
		case "--apply":
			fixApply = true
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
	if withFix && fixPlan == fixApply {
		msg := "--fix requires exactly one of --plan or --apply"
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "doctor.fix",
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
	if !withFix && (fixPlan || fixApply) {
		msg := "--plan/--apply require --fix"
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
	if withFix {
		mode := "plan"
		if fixApply {
			mode = "apply"
		}
		result := runDoctorFix(root, mode, report)
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     result.Summary.Failed == 0,
				Action: "doctor.fix",
				Result: result,
			})
			if result.Summary.Failed > 0 {
				return exitError
			}
			return exitOK
		}
		useColorOut := writerSupportsColor(c.Out)
		lines := []string{
			fmt.Sprintf("%s: %s", styleAccent("root", useColorOut), result.Root),
			fmt.Sprintf("%s: %s", styleAccent("mode", useColorOut), result.Mode),
			fmt.Sprintf("%s: %d", styleAccent("planned", useColorOut), result.Summary.Planned),
			fmt.Sprintf("%s: %d", styleSuccess("applied", useColorOut), result.Summary.Applied),
			fmt.Sprintf("%s: %d", styleWarn("skipped", useColorOut), result.Summary.Skipped),
			fmt.Sprintf("%s: %d", styleError("failed", useColorOut), result.Summary.Failed),
		}
		if len(result.Actions) > 0 {
			lines = append(lines, "actions:")
			for _, a := range result.Actions {
				line := fmt.Sprintf("  - [%s] %s (%s)", a.Status, a.Target, a.Kind)
				if strings.TrimSpace(a.Reason) != "" {
					line += ": " + a.Reason
				}
				switch a.Status {
				case "applied":
					line = styleSuccess(line, useColorOut)
				case "failed":
					line = styleError(line, useColorOut)
				default:
					line = styleWarn(line, useColorOut)
				}
				lines = append(lines, line)
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
		if result.Summary.Failed > 0 {
			return exitError
		}
		return exitOK
	}
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

func runDoctorFix(root string, mode string, report doctorReport) doctorFixResult {
	actions := planDoctorFixActions(report)
	result := doctorFixResult{
		Root:    root,
		Mode:    mode,
		Actions: actions,
	}
	result.Summary.Planned = len(actions)
	if mode != "apply" {
		for i := range result.Actions {
			result.Actions[i].Status = "planned"
		}
		return result
	}
	for i := range result.Actions {
		switch result.Actions[i].Kind {
		case "remove_stale_lock":
			if err := os.Remove(result.Actions[i].Target); err != nil {
				if os.IsNotExist(err) {
					result.Actions[i].Status = "skipped"
					result.Actions[i].Reason = "already_missing"
					result.Summary.Skipped++
					continue
				}
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			result.Actions[i].Status = "applied"
			result.Summary.Applied++
		case "remove_legacy_workstate_file", "remove_legacy_baseline_file":
			if err := os.Remove(result.Actions[i].Target); err != nil {
				if os.IsNotExist(err) {
					result.Actions[i].Status = "skipped"
					result.Actions[i].Reason = "already_missing"
					result.Summary.Skipped++
					continue
				}
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			result.Actions[i].Status = "applied"
			result.Summary.Applied++
		case "migrate_legacy_baseline_file":
			workspaceID, err := workspaceIDFromLegacyBaselineTarget(root, result.Actions[i].Target)
			if err != nil {
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			if err := migrateLegacyWorkspaceBaseline(root, workspaceID); err != nil {
				if os.IsNotExist(err) {
					result.Actions[i].Status = "skipped"
					result.Actions[i].Reason = "already_missing"
					result.Summary.Skipped++
					continue
				}
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			result.Actions[i].Status = "applied"
			result.Summary.Applied++
		case "register_root":
			if err := touchRootRegistry(root); err != nil {
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			result.Actions[i].Status = "applied"
			result.Summary.Applied++
		case "create_workspace_baseline":
			workspaceID, err := workspaceIDFromWorkspaceMetaTarget(root, result.Actions[i].Target)
			if err != nil {
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			if err := createOrRefreshWorkspaceBaseline(context.Background(), root, workspaceID, time.Now().Unix()); err != nil {
				if os.IsNotExist(err) {
					result.Actions[i].Status = "skipped"
					result.Actions[i].Reason = "already_missing"
					result.Summary.Skipped++
					continue
				}
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			result.Actions[i].Status = "applied"
			result.Summary.Applied++
		case "normalize_workspace_work_state":
			workspaceID, err := workspaceIDFromWorkspaceMetaTarget(root, result.Actions[i].Target)
			if err != nil {
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			state, ok, err := resolveDoctorWorkspaceWorkState(context.Background(), root, workspaceID)
			if err != nil {
				if os.IsNotExist(err) {
					result.Actions[i].Status = "skipped"
					result.Actions[i].Reason = "already_missing"
					result.Summary.Skipped++
					continue
				}
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			if !ok {
				result.Actions[i].Status = "skipped"
				result.Actions[i].Reason = "manual_required"
				result.Summary.Skipped++
				continue
			}
			metaPath := filepath.Dir(result.Actions[i].Target)
			changed, err := setWorkspaceMetaWorkState(metaPath, state, time.Now().Unix())
			if err != nil {
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			if !changed {
				result.Actions[i].Status = "skipped"
				result.Actions[i].Reason = "already_normalized"
				result.Summary.Skipped++
				continue
			}
			result.Actions[i].Status = "applied"
			result.Summary.Applied++
		case "resume_ws_close":
			workspaceID, err := workspaceIDFromWSCloseJournalTarget(root, result.Actions[i].Target)
			if err != nil {
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			journal, err := loadWSCloseLifecycleJournal(root, workspaceID)
			if err != nil {
				if os.IsNotExist(err) {
					result.Actions[i].Status = "skipped"
					result.Actions[i].Reason = "already_missing"
					result.Summary.Skipped++
					continue
				}
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			if err := resumeWSCloseLifecycleJournal(context.Background(), root, journal, nil); err != nil {
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			result.Actions[i].Status = "applied"
			result.Summary.Applied++
		case "clear_ws_close_journal":
			workspaceID, err := workspaceIDFromWSCloseJournalTarget(root, result.Actions[i].Target)
			if err != nil {
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			journal, err := loadWSCloseLifecycleJournal(root, workspaceID)
			if err != nil {
				if os.IsNotExist(err) {
					result.Actions[i].Status = "skipped"
					result.Actions[i].Reason = "already_missing"
					result.Summary.Skipped++
					continue
				}
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			if ok, reason := canResetWSCloseLifecycleJournal(root, journal); !ok {
				result.Actions[i].Status = "skipped"
				result.Actions[i].Reason = "manual_required: " + reason
				result.Summary.Skipped++
				continue
			}
			if err := removeWSCloseLifecycleJournal(root, workspaceID); err != nil {
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			result.Actions[i].Status = "applied"
			result.Summary.Applied++
		case "reconcile_root_gitignore":
			changed, err := reconcileRootGitignore(root)
			if err != nil {
				result.Actions[i].Status = "failed"
				result.Actions[i].Reason = err.Error()
				result.Summary.Failed++
				continue
			}
			if !changed {
				result.Actions[i].Status = "skipped"
				result.Actions[i].Reason = "already_reconciled"
				result.Summary.Skipped++
				continue
			}
			result.Actions[i].Status = "applied"
			result.Summary.Applied++
		default:
			result.Actions[i].Status = "skipped"
			result.Actions[i].Reason = "manual_required"
			result.Summary.Skipped++
		}
	}
	return result
}

func planDoctorFixActions(report doctorReport) []doctorFixAction {
	actions := make([]doctorFixAction, 0, len(report.Findings))
	seen := map[string]bool{}
	nextID := 1
	migrateWorkspaceBaseline := map[string]bool{}
	for _, f := range report.Findings {
		if f.Code != "legacy_baseline_in_use" {
			continue
		}
		workspaceID, err := workspaceIDFromLegacyBaselineTarget(report.Root, f.Target)
		if err != nil {
			continue
		}
		migrateWorkspaceBaseline[workspaceID] = true
	}
	for _, f := range report.Findings {
		kind := ""
		target := f.Target
		switch f.Code {
		case "stale_lock":
			kind = "remove_stale_lock"
		case "root_not_registered":
			kind = "register_root"
		case "root_gitignore_missing_defaults", "runtime_state_not_ignored":
			kind = "reconcile_root_gitignore"
			target = rootGitignorePath(report.Root)
		case "legacy_workstate_file":
			kind = "remove_legacy_workstate_file"
		case "legacy_baseline_file", "legacy_baseline_orphan":
			kind = "remove_legacy_baseline_file"
		case "legacy_baseline_in_use":
			kind = "migrate_legacy_baseline_file"
		case "workspace_baseline_missing":
			workspaceID, err := workspaceIDFromWorkspaceMetaTarget(report.Root, f.Target)
			if err != nil || migrateWorkspaceBaseline[workspaceID] {
				continue
			}
			kind = "create_workspace_baseline"
		case "workspace_work_state_missing":
			if !canDoctorNormalizeWorkspaceWorkState(report.Root, f.Target, migrateWorkspaceBaseline) {
				continue
			}
			kind = "normalize_workspace_work_state"
		case "ws_close_resume_ready":
			kind = "resume_ws_close"
		case "ws_close_reset_ready":
			kind = "clear_ws_close_journal"
		default:
			continue
		}
		key := kind + "|" + target
		if seen[key] {
			continue
		}
		seen[key] = true
		actions = append(actions, doctorFixAction{
			ID:     fmt.Sprintf("fx-%03d", nextID),
			Kind:   kind,
			Target: target,
			Status: "planned",
		})
		nextID++
	}
	return actions
}

func workspaceIDFromLegacyBaselineTarget(root string, target string) (string, error) {
	expectedDir := filepath.Clean(filepath.Join(root, ".kra", "state", workspaceBaselineDirName))
	if filepath.Clean(filepath.Dir(target)) != expectedDir {
		return "", fmt.Errorf("legacy baseline target is outside expected directory: %s", target)
	}
	base := strings.TrimSpace(filepath.Base(target))
	if !strings.HasSuffix(base, ".json") {
		return "", fmt.Errorf("legacy baseline target must be a json file: %s", target)
	}
	workspaceID := strings.TrimSuffix(base, ".json")
	if err := validateWorkspaceID(workspaceID); err != nil {
		return "", fmt.Errorf("invalid workspace id from legacy baseline target: %w", err)
	}
	return workspaceID, nil
}

func workspaceIDFromWorkspaceMetaTarget(root string, target string) (string, error) {
	if strings.TrimSpace(filepath.Base(target)) != workspaceMetaFilename {
		return "", fmt.Errorf("workspace meta target must be %s: %s", workspaceMetaFilename, target)
	}
	workspaceDir := filepath.Dir(target)
	expectedBase := filepath.Clean(filepath.Join(root, "workspaces"))
	if filepath.Clean(filepath.Dir(workspaceDir)) != expectedBase {
		return "", fmt.Errorf("workspace meta target is outside expected active workspace directory: %s", target)
	}
	workspaceID := strings.TrimSpace(filepath.Base(workspaceDir))
	if err := validateWorkspaceID(workspaceID); err != nil {
		return "", fmt.Errorf("invalid workspace id from workspace meta target: %w", err)
	}
	return workspaceID, nil
}

func canDoctorNormalizeWorkspaceWorkState(root string, target string, migrateWorkspaceBaseline map[string]bool) bool {
	workspaceID, err := workspaceIDFromWorkspaceMetaTarget(root, target)
	if err != nil {
		return false
	}
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		return false
	}
	if strings.TrimSpace(meta.Workspace.WorkState) != "" {
		return false
	}
	if meta.Baseline != nil {
		return true
	}
	return migrateWorkspaceBaseline[workspaceID]
}

func resolveDoctorWorkspaceWorkState(ctx context.Context, root string, workspaceID string) (workspaceWorkState, bool, error) {
	wsPath := filepath.Join(root, "workspaces", workspaceID)
	meta, err := loadWorkspaceMetaFile(wsPath)
	if err != nil {
		return workspaceWorkStateTodo, false, err
	}
	if strings.TrimSpace(meta.Workspace.WorkState) != "" {
		return normalizeWorkspaceWorkState(workspaceWorkState(meta.Workspace.WorkState)), false, nil
	}
	if meta.Baseline == nil {
		return workspaceWorkStateTodo, false, nil
	}
	repos, err := listWorkspaceReposFromFilesystem(ctx, root, "active", workspaceID, meta)
	if err != nil {
		return workspaceWorkStateTodo, false, err
	}
	state, err := deriveWorkspaceWorkStateFromBaseline(ctx, root, workspaceID, repos)
	if err != nil {
		return workspaceWorkStateTodo, false, err
	}
	return state, true, nil
}

func touchRootRegistry(root string) error {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	return stateregistry.Touch(cleanRoot, time.Now())
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
	scanDoctorLegacyState(root, addOK, addWarn)
	scanDoctorRootHygiene(root, addOK, addWarn)
	scanDoctorWSCloseRecovery(root, addOK, addWarn)

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
		if expectStatus == "active" {
			metaPath := filepath.Join(wsPath, workspaceMetaFilename)
			if meta.Baseline == nil {
				addWarn("workspace_baseline_missing", metaPath, "active workspace is missing .kra.meta.json.baseline; doctor --fix can create it from current filesystem state")
			} else {
				addOK()
			}
			if strings.TrimSpace(meta.Workspace.WorkState) == "" {
				addWarn("workspace_work_state_missing", metaPath, "active workspace is missing .kra.meta.json.workspace.work_state; doctor --fix can normalize it when deterministic")
			} else {
				addOK()
			}
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

func scanDoctorLegacyState(
	root string,
	addOK func(),
	addWarn func(code string, target string, message string),
) {
	stateDir := filepath.Join(root, ".kra", "state")
	if fi, err := os.Stat(stateDir); err != nil {
		if os.IsNotExist(err) {
			addOK()
			return
		}
		addWarn("state_dir_stat_failed", stateDir, err.Error())
		return
	} else if !fi.IsDir() {
		addWarn("state_dir_not_directory", stateDir, "state path is not a directory")
		return
	}
	addOK()

	workstatePath := filepath.Join(stateDir, "workspace-workstate.json")
	if fi, err := os.Stat(workstatePath); err != nil {
		if os.IsNotExist(err) {
			addOK()
		} else {
			addWarn("legacy_workstate_stat_failed", workstatePath, err.Error())
		}
	} else if fi.IsDir() {
		addWarn("legacy_workstate_path_invalid", workstatePath, "legacy work-state path should be a file")
	} else {
		addWarn("legacy_workstate_file", workstatePath, "legacy workspace work-state file is unused and can be removed")
	}

	baselineDir := filepath.Join(stateDir, workspaceBaselineDirName)
	entries, err := os.ReadDir(baselineDir)
	if err != nil {
		if os.IsNotExist(err) {
			addOK()
			return
		}
		addWarn("legacy_baseline_dir_read_failed", baselineDir, err.Error())
		return
	}
	addOK()

	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		target := filepath.Join(baselineDir, ent.Name())
		workspaceID := strings.TrimSuffix(ent.Name(), ".json")
		if _, err := os.Stat(filepath.Join(root, "archive", workspaceID)); err == nil {
			addWarn("legacy_baseline_file", target, "archived workspace no longer needs legacy baseline file")
			continue
		}
		wsPath := filepath.Join(root, "workspaces", workspaceID)
		meta, err := loadWorkspaceMetaFile(wsPath)
		switch {
		case err == nil && meta.Baseline != nil:
			addWarn("legacy_baseline_file", target, "baseline already exists in .kra.meta.json; legacy file can be removed")
		case err == nil:
			addWarn("legacy_baseline_in_use", target, "workspace still relies on legacy baseline file; doctor --fix can migrate it into .kra.meta.json")
		case os.IsNotExist(err):
			addWarn("legacy_baseline_orphan", target, "workspace is missing; legacy baseline file can be removed")
		default:
			addWarn("legacy_baseline_check_failed", target, "unable to confirm safe cleanup automatically")
		}
	}
}

func scanDoctorRootHygiene(
	root string,
	addOK func(),
	addWarn func(code string, target string, message string),
) {
	gitignorePath := rootGitignorePath(root)
	contents := ""
	if b, err := os.ReadFile(gitignorePath); err == nil {
		contents = string(b)
		addOK()
	} else if !os.IsNotExist(err) {
		addWarn("root_gitignore_read_failed", gitignorePath, err.Error())
		return
	}

	missing := missingManagedRootGitignorePatterns(contents)
	if len(missing) > 0 {
		addWarn("root_gitignore_missing_defaults", gitignorePath, fmt.Sprintf("missing default ignore patterns: %s", strings.Join(missing, ", ")))
	} else {
		addOK()
	}

	runtimeTargets, err := collectDoctorRuntimeStateTargets(root)
	if err != nil {
		addWarn("runtime_state_scan_failed", filepath.Join(root, ".kra", "state"), err.Error())
	} else if len(runtimeTargets) == 0 {
		addOK()
	} else {
		for _, target := range runtimeTargets {
			rel, relErr := filepath.Rel(root, target)
			if relErr != nil {
				addWarn("runtime_state_ignore_check_failed", target, relErr.Error())
				continue
			}
			ignored, ignoreErr := gitutil.IsIgnored(context.Background(), root, filepath.ToSlash(rel))
			if ignoreErr != nil {
				addWarn("runtime_state_ignore_check_failed", target, ignoreErr.Error())
				continue
			}
			if !ignored {
				addWarn("runtime_state_not_ignored", target, "runtime state file is not ignored; reconcile root .gitignore")
				continue
			}
			addOK()
		}
	}

	trackedNoise, err := listDoctorTrackedLocalNoiseFiles(root)
	if err != nil {
		addWarn("tracked_local_noise_scan_failed", root, err.Error())
		return
	}
	if len(trackedNoise) == 0 {
		addOK()
		return
	}
	for _, rel := range trackedNoise {
		addWarn("tracked_local_noise_file", filepath.Join(root, filepath.FromSlash(rel)), "tracked local-noise file should be untracked or ignored manually; doctor --fix will not modify tracked files")
	}
}

func collectDoctorRuntimeStateTargets(root string) ([]string, error) {
	targets := make([]string, 0, 4)
	for _, rel := range []string{
		filepath.Join(".kra", "state", "cmux-sessions.json"),
		filepath.Join(".kra", "state", "cmux-workspaces.json"),
		filepath.Join(".kra", "state", "root-repos.json"),
	} {
		target := filepath.Join(root, rel)
		fi, err := os.Stat(target)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if fi.IsDir() {
			continue
		}
		targets = append(targets, target)
	}

	opsDir := filepath.Join(root, ".kra", "state", "operations")
	if _, err := os.Stat(opsDir); err != nil {
		if os.IsNotExist(err) {
			slices.Sort(targets)
			return targets, nil
		}
		return nil, err
	}
	if err := filepath.WalkDir(opsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		targets = append(targets, path)
		return nil
	}); err != nil {
		return nil, err
	}
	slices.Sort(targets)
	return targets, nil
}

func listDoctorTrackedLocalNoiseFiles(root string) ([]string, error) {
	out, err := gitutil.Run(context.Background(), root, "ls-files", "-z", "--full-name")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	seen := map[string]bool{}
	files := make([]string, 0)
	for _, entry := range strings.Split(out, "\x00") {
		rel := filepath.ToSlash(strings.TrimSpace(strings.Trim(entry, "\x00")))
		if rel == "" || !isDoctorTrackedLocalNoisePath(rel) || seen[rel] {
			continue
		}
		seen[rel] = true
		files = append(files, rel)
	}
	slices.Sort(files)
	return files, nil
}

func isDoctorTrackedLocalNoisePath(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	switch {
	case rel == "":
		return false
	case rel == ".DS_Store" || strings.HasSuffix(rel, "/.DS_Store"):
		return true
	case strings.HasSuffix(rel, ".code-workspace"):
		return true
	case rel == ".idea/workspace.xml" || strings.HasSuffix(rel, "/.idea/workspace.xml"):
		return true
	case rel == ".idea/tasks.xml" || strings.HasSuffix(rel, "/.idea/tasks.xml"):
		return true
	default:
		return false
	}
}

func scanDoctorWSCloseRecovery(
	root string,
	addOK func(),
	addWarn func(code string, target string, message string),
) {
	journalDir := wsCloseJournalDir(root)
	entries, err := os.ReadDir(journalDir)
	if err != nil {
		if os.IsNotExist(err) {
			addOK()
			scanDoctorLegacyHalfClosedWSClose(root, map[string]bool{}, addWarn)
			return
		}
		addWarn("ws_close_journal_dir_read_failed", journalDir, err.Error())
		return
	}
	addOK()

	journalIDs := make(map[string]bool, len(entries))
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		target := filepath.Join(journalDir, ent.Name())
		journal, err := loadWSCloseLifecycleJournalFile(target)
		if err != nil {
			addWarn("ws_close_journal_invalid", target, err.Error())
			continue
		}
		journalIDs[journal.WorkspaceID] = true
		if journal.Phase == wsClosePhaseCompleted {
			addWarn("ws_close_journal_stale", target, "completed ws close journal should be removed")
			continue
		}
		resumeOK, resumeReason := canResumeWSCloseLifecycleJournal(root, journal)
		resetOK, resetReason := canResetWSCloseLifecycleJournal(root, journal)
		if resumeOK {
			addWarn("ws_close_resume_ready", target, fmt.Sprintf("workspace %s close can be resumed with doctor --fix --apply", journal.WorkspaceID))
			continue
		}
		if resetOK {
			addWarn("ws_close_reset_ready", target, fmt.Sprintf("workspace %s close journal can be cleared with doctor --fix --apply", journal.WorkspaceID))
			continue
		}
		reason := resumeReason
		if journal.Phase == wsClosePhaseRiskChecked {
			reason = resetReason
		}
		addWarn("ws_close_manual_required", target, fmt.Sprintf("workspace %s close requires manual recovery: %s", journal.WorkspaceID, reason))
	}

	scanDoctorLegacyHalfClosedWSClose(root, journalIDs, addWarn)
}

func scanDoctorLegacyHalfClosedWSClose(
	root string,
	journalIDs map[string]bool,
	addWarn func(code string, target string, message string),
) {
	archiveDir := filepath.Join(root, "archive")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		workspaceID := strings.TrimSpace(ent.Name())
		if journalIDs[workspaceID] {
			continue
		}
		ok, reason := detectLegacyHalfClosedWSClose(root, workspaceID)
		if !ok {
			continue
		}
		addWarn("ws_close_manual_required", filepath.Join(archiveDir, workspaceID), fmt.Sprintf("workspace %s close requires manual recovery: %s", workspaceID, reason))
	}
}
