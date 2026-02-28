package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/cmuxmap"
	"github.com/tasuku43/kra/internal/infra/cmuxctl"
	"github.com/tasuku43/kra/internal/infra/paths"
)

type cmuxSwitchClient interface {
	SelectWorkspace(ctx context.Context, workspace string) error
}

var newCMUXSwitchClient = func() cmuxSwitchClient { return cmuxctl.NewClient() }

func (c *CLI) runCMUXSwitch(args []string) int {
	outputFormat := "human"
	workspaceID := ""
	cmuxHandle := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "help":
			c.printCMUXSwitchUsage(c.Out)
			return exitOK
		case "--format":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printCMUXSwitchUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[1])
			args = args[2:]
		case "--workspace":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--workspace requires a value")
				c.printCMUXSwitchUsage(c.Err)
				return exitUsage
			}
			workspaceID = strings.TrimSpace(args[1])
			args = args[2:]
		case "--cmux":
			if len(args) < 2 {
				fmt.Fprintln(c.Err, "--cmux requires a value")
				c.printCMUXSwitchUsage(c.Err)
				return exitUsage
			}
			cmuxHandle = strings.TrimSpace(args[1])
			args = args[2:]
		default:
			if strings.HasPrefix(args[0], "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(args[0], "--format="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--workspace=") {
				workspaceID = strings.TrimSpace(strings.TrimPrefix(args[0], "--workspace="))
				args = args[1:]
				continue
			}
			if strings.HasPrefix(args[0], "--cmux=") {
				cmuxHandle = strings.TrimSpace(strings.TrimPrefix(args[0], "--cmux="))
				args = args[1:]
				continue
			}
			fmt.Fprintf(c.Err, "unknown flag for cmux switch: %q\n", args[0])
			c.printCMUXSwitchUsage(c.Err)
			return exitUsage
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printCMUXSwitchUsage(c.Err)
		return exitUsage
	}
	if len(args) > 0 {
		fmt.Fprintf(c.Err, "unexpected args for cmux switch: %q\n", strings.Join(args, " "))
		c.printCMUXSwitchUsage(c.Err)
		return exitUsage
	}
	if workspaceID != "" {
		if err := validateWorkspaceID(workspaceID); err != nil {
			return c.writeCMUXSwitchError(outputFormat, "invalid_argument", workspaceID, fmt.Sprintf("invalid workspace id: %v", err), exitUsage)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return c.writeCMUXSwitchError(outputFormat, "internal_error", workspaceID, fmt.Sprintf("get working dir: %v", err), exitError)
	}
	root, err := paths.ResolveExistingRoot(wd)
	if err != nil {
		return c.writeCMUXSwitchError(outputFormat, "internal_error", workspaceID, fmt.Sprintf("resolve KRA_ROOT: %v", err), exitError)
	}

	store := newCMUXMapStore(root)
	mapping, err := store.Load()
	if err != nil {
		return c.writeCMUXSwitchError(outputFormat, "state_write_failed", workspaceID, fmt.Sprintf("load cmux mapping: %v", err), exitError)
	}
	if runtime, rerr := newCMUXRuntimeClient().ListWorkspaces(context.Background()); rerr == nil {
		reconciled, _, _, recErr := reconcileCMUXMappingWithRuntime(store, mapping, runtime, true)
		if recErr != nil {
			return c.writeCMUXSwitchError(outputFormat, "state_write_failed", workspaceID, fmt.Sprintf("reconcile cmux mapping: %v", recErr), exitError)
		}
		mapping = reconciled
	}

	resolvedWorkspaceID, resolvedEntry, code, msg := c.resolveCMUXSwitchTarget(mapping, workspaceID, cmuxHandle, outputFormat)
	if code != "" {
		exitCode := exitError
		if code == "invalid_argument" {
			exitCode = exitUsage
		}
		return c.writeCMUXSwitchError(outputFormat, code, workspaceID, msg, exitCode)
	}

	client := newCMUXSwitchClient()
	if err := client.SelectWorkspace(context.Background(), resolvedEntry.CMUXWorkspaceID); err != nil {
		return c.writeCMUXSwitchError(outputFormat, "cmux_select_failed", resolvedWorkspaceID, fmt.Sprintf("select cmux workspace: %v", err), exitError)
	}

	ws := mapping.Workspaces[resolvedWorkspaceID]
	for i := range ws.Entries {
		if ws.Entries[i].CMUXWorkspaceID == resolvedEntry.CMUXWorkspaceID {
			ws.Entries[i].LastUsedAt = time.Now().UTC().Format(time.RFC3339)
			resolvedEntry = ws.Entries[i]
			break
		}
	}
	mapping.Workspaces[resolvedWorkspaceID] = ws
	if err := store.Save(mapping); err != nil {
		return c.writeCMUXSwitchError(outputFormat, "state_write_failed", resolvedWorkspaceID, fmt.Sprintf("save cmux mapping: %v", err), exitError)
	}

	if outputFormat == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          true,
			Action:      "cmux.switch",
			WorkspaceID: resolvedWorkspaceID,
			Result: map[string]any{
				"kra_workspace_id":  resolvedWorkspaceID,
				"cmux_workspace_id": resolvedEntry.CMUXWorkspaceID,
				"ordinal":           resolvedEntry.Ordinal,
				"title":             resolvedEntry.TitleSnapshot,
			},
		})
		return exitOK
	}

	fmt.Fprintln(c.Out, "switched cmux workspace")
	fmt.Fprintf(c.Out, "  kra: %s\n", resolvedWorkspaceID)
	fmt.Fprintf(c.Out, "  cmux: %s\n", resolvedEntry.CMUXWorkspaceID)
	fmt.Fprintf(c.Out, "  title: %s\n", resolvedEntry.TitleSnapshot)
	return exitOK
}

func (c *CLI) resolveCMUXSwitchTarget(mapping cmuxmap.File, workspaceID string, cmuxHandle string, outputFormat string) (string, cmuxmap.Entry, string, string) {
	workspaceID = strings.TrimSpace(workspaceID)
	cmuxHandle = strings.TrimSpace(cmuxHandle)

	if workspaceID != "" {
		ws, ok := mapping.Workspaces[workspaceID]
		if !ok || len(ws.Entries) == 0 {
			return "", cmuxmap.Entry{}, "cmux_not_mapped", fmt.Sprintf("no cmux mapping found for workspace: %s", workspaceID)
		}
		if cmuxHandle == "" {
			return c.resolveCMUXEntryWithFallback(workspaceID, ws.Entries, outputFormat)
		}
		matches := filterCMUXEntries(ws.Entries, cmuxHandle)
		switch len(matches) {
		case 1:
			return workspaceID, matches[0], "", ""
		case 0:
			if outputFormat == "json" {
				return "", cmuxmap.Entry{}, "cmux_not_mapped", fmt.Sprintf("cmux target not found in workspace %s: %s", workspaceID, cmuxHandle)
			}
			return c.resolveCMUXEntryWithFallback(workspaceID, ws.Entries, outputFormat)
		default:
			if outputFormat == "json" {
				return "", cmuxmap.Entry{}, "cmux_ambiguous_target", fmt.Sprintf("multiple cmux targets matched: %s", cmuxHandle)
			}
			return c.resolveCMUXEntryWithFallback(workspaceID, matches, outputFormat)
		}
	}

	if cmuxHandle != "" {
		type match struct {
			workspaceID string
			entry       cmuxmap.Entry
		}
		all := []match{}
		for wsID, ws := range mapping.Workspaces {
			for _, e := range filterCMUXEntries(ws.Entries, cmuxHandle) {
				all = append(all, match{workspaceID: wsID, entry: e})
			}
		}
		if len(all) == 1 {
			return all[0].workspaceID, all[0].entry, "", ""
		}
		if outputFormat == "json" {
			if len(all) == 0 {
				return "", cmuxmap.Entry{}, "cmux_not_mapped", fmt.Sprintf("cmux target not found: %s", cmuxHandle)
			}
			return "", cmuxmap.Entry{}, "cmux_ambiguous_target", fmt.Sprintf("multiple cmux targets matched: %s", cmuxHandle)
		}
	}

	if outputFormat == "json" {
		return "", cmuxmap.Entry{}, "non_interactive_selection_required", "switch requires --workspace/--cmux in --format json mode"
	}
	wsID, code, msg := c.selectCMUXWorkspaceID(mapping)
	if code != "" {
		return "", cmuxmap.Entry{}, code, msg
	}
	return c.resolveCMUXEntryWithFallback(wsID, mapping.Workspaces[wsID].Entries, outputFormat)
}

func (c *CLI) selectCMUXWorkspaceID(mapping cmuxmap.File) (string, string, string) {
	ids := make([]string, 0, len(mapping.Workspaces))
	for wsID, ws := range mapping.Workspaces {
		if len(ws.Entries) > 0 {
			ids = append(ids, wsID)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return "", "cmux_not_mapped", "no cmux mappings available"
	}
	if len(ids) == 1 {
		return ids[0], "", ""
	}
	candidates := make([]workspaceSelectorCandidate, 0, len(ids))
	for _, id := range ids {
		candidates = append(candidates, workspaceSelectorCandidate{
			ID:    id,
			Title: fmt.Sprintf("%d mapped", len(mapping.Workspaces[id].Entries)),
		})
	}
	selected, err := c.promptWorkspaceSelectorSingle("active", "switch", candidates)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "tty") {
			return "", "non_interactive_selection_required", "interactive workspace selection requires a TTY"
		}
		return "", "cmux_not_mapped", err.Error()
	}
	if len(selected) != 1 {
		return "", "cmux_not_mapped", "cmux switch requires exactly one workspace selected"
	}
	return strings.TrimSpace(selected[0]), "", ""
}

func (c *CLI) resolveCMUXEntryWithFallback(workspaceID string, entries []cmuxmap.Entry, outputFormat string) (string, cmuxmap.Entry, string, string) {
	if len(entries) == 0 {
		return "", cmuxmap.Entry{}, "cmux_not_mapped", fmt.Sprintf("no cmux mapping found for workspace: %s", workspaceID)
	}
	if len(entries) == 1 {
		return workspaceID, entries[0], "", ""
	}
	if outputFormat == "json" {
		return "", cmuxmap.Entry{}, "cmux_ambiguous_target", "multiple cmux mappings found; provide --cmux"
	}
	candidates := make([]workspaceSelectorCandidate, 0, len(entries))
	for _, e := range entries {
		title := strings.TrimSpace(e.TitleSnapshot)
		if title == "" {
			title = fmt.Sprintf("ordinal=%d", e.Ordinal)
		}
		candidates = append(candidates, workspaceSelectorCandidate{
			ID:    e.CMUXWorkspaceID,
			Title: title,
		})
	}
	selected, err := c.promptWorkspaceSelectorWithOptionsAndMode("active", "switch", "cmux:", "cmux", candidates, true)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "tty") {
			return "", cmuxmap.Entry{}, "non_interactive_selection_required", "interactive cmux selection requires a TTY"
		}
		return "", cmuxmap.Entry{}, "cmux_not_mapped", err.Error()
	}
	if len(selected) != 1 {
		return "", cmuxmap.Entry{}, "cmux_not_mapped", "cmux switch requires exactly one target selected"
	}
	id := strings.TrimSpace(selected[0])
	for _, e := range entries {
		if e.CMUXWorkspaceID == id {
			return workspaceID, e, "", ""
		}
	}
	return "", cmuxmap.Entry{}, "cmux_not_mapped", fmt.Sprintf("selected cmux target not found: %s", id)
}

func filterCMUXEntries(entries []cmuxmap.Entry, handle string) []cmuxmap.Entry {
	handle = strings.TrimSpace(handle)
	out := make([]cmuxmap.Entry, 0, len(entries))
	for _, e := range entries {
		if e.CMUXWorkspaceID == handle {
			out = append(out, e)
			continue
		}
		if handle == fmt.Sprintf("workspace:%d", e.Ordinal) {
			out = append(out, e)
		}
	}
	return out
}

func (c *CLI) writeCMUXSwitchError(format string, code string, workspaceID string, message string, exitCode int) int {
	if format == "json" {
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:          false,
			Action:      "cmux.switch",
			WorkspaceID: workspaceID,
			Error: &cliJSONError{
				Code:    code,
				Message: message,
			},
		})
		return exitCode
	}
	if workspaceID != "" {
		fmt.Fprintf(c.Err, "cmux switch (%s): %s\n", workspaceID, message)
	} else {
		fmt.Fprintf(c.Err, "cmux switch: %s\n", message)
	}
	return exitCode
}
