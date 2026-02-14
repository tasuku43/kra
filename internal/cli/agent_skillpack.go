package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

type bootstrapSkillpackFile struct {
	relativePath string
	content      string
}

func defaultAgentSkillpackFiles() []bootstrapSkillpackFile {
	return []bootstrapSkillpackFile{
		{
			relativePath: ".kra-skillpack.yaml",
			content: `version: "v1"
pack: "kra-flow"
skills:
  - flow-investigation
  - flow-execution
  - flow-insight-capture
`,
		},
		{
			relativePath: "flow-investigation/SKILL.md",
			content: `---
name: flow-investigation
description: Use this skill to run structured investigation flow and extract reusable findings.
---

# Flow: Investigation

## Goal

Run structured investigation with clear evidence and reusable conclusions.

## Steps

1. Clarify scope, assumptions, and success criteria.
2. Gather evidence from repositories, metrics, logs, and related docs.
3. Separate facts from hypotheses.
4. Summarize key findings and unresolved questions.
5. If a reusable insight appears, hand off to flow-insight-capture.
`,
		},
		{
			relativePath: "flow-execution/SKILL.md",
			content: `---
name: flow-execution
description: Use this skill to execute changes in small validated slices with explicit risk reporting.
---

# Flow: Execution

## Goal

Execute changes safely with minimal blast radius and clear verification.

## Steps

1. Confirm target behavior and non-goals.
2. Implement in small slices.
3. Verify with focused tests first, then wider checks.
4. Report what changed, what was verified, and remaining risks.
5. If a reusable insight appears, hand off to flow-insight-capture.
`,
		},
		{
			relativePath: "flow-insight-capture/SKILL.md",
			content: `---
name: flow-insight-capture
description: Use this skill when a high-value insight appears; propose capture first and persist only after explicit user approval.
---

# Flow: Insight Capture

## Goal

Capture only high-value, reusable insights without always-on logging.

## Trigger

Use this flow when one of the following appears:

- reusable debugging pattern or mitigation
- decision rationale worth reusing
- failure mode and recovery pattern

## Required behavior

1. Propose capture in conversation first.
2. Persist only when user explicitly approves.
3. Write with:
   kra ws insight add --id <workspace-id> --ticket <ticket> --session-id <session-id> --what "<summary>" --context "<context>" --why "<why>" --next "<next>" --tag <tag> ... --approved
4. Never write when approval is missing.
`,
		},
	}
}

func ensureBootstrapDefaultSkillpack(skillsRoot string, result *bootstrapAgentSkillsResult) error {
	for _, file := range defaultAgentSkillpackFiles() {
		path := filepath.Join(skillsRoot, filepath.FromSlash(file.relativePath))
		if err := ensureBootstrapSkillpackFile(path, file.content, result); err != nil {
			return err
		}
	}
	return nil
}

func ensureBootstrapSkillpackFile(path string, content string, result *bootstrapAgentSkillsResult) error {
	parent := filepath.Dir(path)
	parentInfo, err := os.Stat(parent)
	if err == nil {
		if !parentInfo.IsDir() {
			appendBootstrapConflict(result, parent, "exists and is not a directory")
			return nil
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", parent, err)
		}
		appendUniquePath(&result.Created, parent)
	} else {
		return fmt.Errorf("stat %s: %w", parent, err)
	}

	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode().IsRegular() {
			// Preserve user/tool-managed edits; bootstrap remains non-destructive.
			appendUniquePath(&result.Skipped, path)
			return nil
		}
		appendBootstrapConflict(result, path, "exists and is not a regular file")
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("lstat %s: %w", path, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	appendUniquePath(&result.Created, path)
	return nil
}

func appendUniquePath(items *[]string, path string) {
	for _, item := range *items {
		if item == path {
			return
		}
	}
	*items = append(*items, path)
}
