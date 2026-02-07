package cli

import (
	"errors"
	"fmt"
	"strings"
)

var errWorkspaceFlowCanceled = errors.New("workspace flow canceled")

type workspaceFlowSelection struct {
	ID string
}

type workspaceFlowRiskStage struct {
	HasRisk bool
	Print   func(useColor bool)
}

type workspaceSelectRiskResultFlowConfig struct {
	FlowName string

	SelectItems func() ([]workspaceFlowSelection, error)

	CollectRiskStage func([]workspaceFlowSelection) (workspaceFlowRiskStage, error)
	ConfirmRisk      func() (bool, error)

	ApplyOne func(workspaceFlowSelection) error

	ResultVerb  string
	ResultMark  string
	PrintResult func(done []string, total int, useColor bool)
}

func (c *CLI) runWorkspaceSelectRiskResultFlow(cfg workspaceSelectRiskResultFlowConfig, useColor bool) ([]string, error) {
	if cfg.SelectItems == nil {
		return nil, fmt.Errorf("workspace flow: SelectItems is required")
	}
	if cfg.ApplyOne == nil {
		return nil, fmt.Errorf("workspace flow: ApplyOne is required")
	}
	flowName := strings.TrimSpace(cfg.FlowName)
	if flowName == "" {
		flowName = "workspace flow"
	}

	selectedItems, err := cfg.SelectItems()
	if err != nil {
		return nil, err
	}
	selectedIDs := workspaceFlowSelectionIDs(selectedItems)
	c.debugf("%s selected=%v", flowName, selectedIDs)

	if cfg.CollectRiskStage != nil {
		riskStage, err := cfg.CollectRiskStage(selectedItems)
		if err != nil {
			return nil, err
		}
		if riskStage.HasRisk {
			c.debugf("%s risk stage entered", flowName)
			if riskStage.Print != nil {
				riskStage.Print(useColor)
			}
			if cfg.ConfirmRisk != nil {
				ok, err := cfg.ConfirmRisk()
				if err != nil {
					return nil, err
				}
				if !ok {
					c.printWorkspaceFlowAbortedResult("canceled at Risk", useColor)
					c.debugf("%s canceled at risk stage", flowName)
					return nil, errWorkspaceFlowCanceled
				}
			}
		}
	}

	done := make([]string, 0, len(selectedItems))
	for _, item := range selectedItems {
		if err := cfg.ApplyOne(item); err != nil {
			return done, fmt.Errorf("apply workspace %s: %w", item.ID, err)
		}
		done = append(done, item.ID)
	}

	if cfg.PrintResult != nil {
		cfg.PrintResult(done, len(selectedItems), useColor)
	} else {
		c.printWorkspaceFlowResult(cfg.ResultVerb, cfg.ResultMark, done, len(selectedItems), useColor)
	}
	c.debugf("%s result done=%v", flowName, done)
	return done, nil
}

func workspaceFlowSelectionIDs(items []workspaceFlowSelection) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func (c *CLI) printWorkspaceFlowAbortedResult(reason string, useColor bool) {
	fmt.Fprintln(c.Out)
	fmt.Fprintln(c.Out, renderResultTitle(useColor))
	fmt.Fprintf(c.Out, "%saborted: %s\n", uiIndent, reason)
}

func (c *CLI) printWorkspaceFlowResult(verb string, mark string, done []string, total int, useColor bool) {
	if verb == "" {
		verb = "Done"
	}
	if mark == "" {
		mark = "✔"
	}

	fmt.Fprintln(c.Out)
	fmt.Fprintln(c.Out, renderResultTitle(useColor))
	fmt.Fprintf(c.Out, "%s%s %d / %d\n", uiIndent, verb, len(done), total)
	for _, id := range done {
		fmt.Fprintf(c.Out, "%s%s %s\n", uiIndent, mark, id)
	}
}
