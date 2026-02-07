package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestWorkspaceFlow_ApplyReceivesSelectedItems(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	c := New(&out, &errOut)

	var applied []string
	done, err := c.runWorkspaceSelectRiskResultFlow(workspaceSelectRiskResultFlowConfig{
		FlowName: "test flow",
		SelectItems: func() ([]workspaceFlowSelection, error) {
			return []workspaceFlowSelection{
				{ID: "WS-1"},
				{ID: "WS-2"},
			}, nil
		},
		ApplyOne: func(item workspaceFlowSelection) error {
			applied = append(applied, item.ID)
			return nil
		},
		ResultVerb: "Applied",
		ResultMark: "+",
	}, false)
	if err != nil {
		t.Fatalf("runWorkspaceSelectRiskResultFlow() err = %v", err)
	}
	if len(done) != 2 || done[0] != "WS-1" || done[1] != "WS-2" {
		t.Fatalf("done = %v, want [WS-1 WS-2]", done)
	}
	if len(applied) != 2 || applied[0] != "WS-1" || applied[1] != "WS-2" {
		t.Fatalf("applied = %v, want [WS-1 WS-2]", applied)
	}
}

func TestWorkspaceFlow_RiskCancelReturnsSentinelAndPrintsAbortedResult(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	c := New(&out, &errOut)

	_, err := c.runWorkspaceSelectRiskResultFlow(workspaceSelectRiskResultFlowConfig{
		FlowName: "test flow",
		SelectItems: func() ([]workspaceFlowSelection, error) {
			return []workspaceFlowSelection{{ID: "WS-1"}}, nil
		},
		CollectRiskStage: func(items []workspaceFlowSelection) (workspaceFlowRiskStage, error) {
			return workspaceFlowRiskStage{HasRisk: true}, nil
		},
		ConfirmRisk: func() (bool, error) {
			return false, nil
		},
		ApplyOne: func(item workspaceFlowSelection) error {
			return errors.New("must not be called")
		},
	}, false)
	if !errors.Is(err, errWorkspaceFlowCanceled) {
		t.Fatalf("err = %v, want errWorkspaceFlowCanceled", err)
	}
	if got := out.String(); got == "" || !containsAll(got, "Result:", "aborted: canceled at Risk") {
		t.Fatalf("stdout missing aborted result: %q", got)
	}
}

func TestWorkspaceFlow_UsesCustomResultPrinterWhenProvided(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	c := New(&out, &errOut)

	called := false
	_, err := c.runWorkspaceSelectRiskResultFlow(workspaceSelectRiskResultFlowConfig{
		FlowName: "test flow",
		SelectItems: func() ([]workspaceFlowSelection, error) {
			return []workspaceFlowSelection{{ID: "WS-1"}}, nil
		},
		ApplyOne: func(item workspaceFlowSelection) error {
			return nil
		},
		PrintResult: func(done []string, total int, useColor bool) {
			called = true
			if len(done) != 1 || done[0] != "WS-1" {
				t.Fatalf("done = %v, want [WS-1]", done)
			}
			if total != 1 {
				t.Fatalf("total = %d, want 1", total)
			}
			out.WriteString("custom result\n")
		},
	}, false)
	if err != nil {
		t.Fatalf("runWorkspaceSelectRiskResultFlow() err = %v", err)
	}
	if !called {
		t.Fatalf("custom result printer was not called")
	}
	got := out.String()
	if !containsAll(got, "custom result") {
		t.Fatalf("stdout missing custom result: %q", got)
	}
	if containsAll(got, "Result:") {
		t.Fatalf("default result should not be printed when custom result is set: %q", got)
	}
}

func TestWorkspaceFlow_DefaultResultUsesSharedIndent(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	c := New(&out, &errOut)

	_, err := c.runWorkspaceSelectRiskResultFlow(workspaceSelectRiskResultFlowConfig{
		FlowName: "test flow",
		SelectItems: func() ([]workspaceFlowSelection, error) {
			return []workspaceFlowSelection{{ID: "WS-1"}}, nil
		},
		ApplyOne: func(item workspaceFlowSelection) error {
			return nil
		},
		ResultVerb: "Applied",
		ResultMark: "+",
	}, false)
	if err != nil {
		t.Fatalf("runWorkspaceSelectRiskResultFlow() err = %v", err)
	}

	got := out.String()
	if !containsAll(got, "\n  Applied 1 / 1", "\n  + WS-1") {
		t.Fatalf("result body should use shared 2-space indentation: %q", got)
	}
	if strings.Contains(got, "Result:\n\n") {
		t.Fatalf("result heading should not have blank line before body: %q", got)
	}
}

func TestWorkspaceFlow_AbortedResultHasNoBlankAfterHeading(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	c := New(&out, &errOut)

	c.printWorkspaceFlowAbortedResult("canceled at Risk", false)
	got := out.String()

	if !containsAll(got, "Result:", "\n  aborted: canceled at Risk") {
		t.Fatalf("unexpected aborted result output: %q", got)
	}
	if strings.Contains(got, "Result:\n\n") {
		t.Fatalf("aborted result heading should not have blank line before body: %q", got)
	}
}

func containsAll(s string, wants ...string) bool {
	for _, w := range wants {
		if !bytes.Contains([]byte(s), []byte(w)) {
			return false
		}
	}
	return true
}
