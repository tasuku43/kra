package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tasuku43/kra/internal/core/repospec"
	"github.com/tasuku43/kra/internal/core/repostore"
	"github.com/tasuku43/kra/internal/infra/gitutil"
)

const repoPoolAddDefaultWorkers = 4

type repoPoolAddRequest struct {
	RepoSpecInput string
	DisplayName   string
	AlreadyInRoot bool
}

type repoPoolAddOutcome struct {
	RepoKey string
	Success bool
	Skipped bool
	Reason  string
}

type repoPoolAddProgressType string

const (
	repoPoolAddProgressStart repoPoolAddProgressType = "start"
	repoPoolAddProgressDone  repoPoolAddProgressType = "done"
)

type repoPoolAddProgressEvent struct {
	Index   int
	Type    repoPoolAddProgressType
	RepoKey string
	Success bool
	Reason  string
}

func applyRepoPoolAddsWithProgress(ctx context.Context, repoPoolPath string, requests []repoPoolAddRequest, workers int, debugf func(string, ...any), progressOut io.Writer, useColor bool) []repoPoolAddOutcome {
	if workers <= 0 {
		workers = repoPoolAddDefaultWorkers
	}
	progressEvents := make(chan repoPoolAddProgressEvent, workers*2+1)
	done := make(chan struct{})
	go func() {
		printRepoPoolProgress(progressOut, useColor, requests, progressEvents)
		close(done)
	}()

	outcomes := applyRepoPoolAdds(ctx, repoPoolPath, requests, workers, debugf, func(ev repoPoolAddProgressEvent) {
		progressEvents <- ev
	})
	close(progressEvents)
	<-done
	return outcomes
}

func applyRepoPoolAdds(ctx context.Context, repoPoolPath string, requests []repoPoolAddRequest, workers int, debugf func(string, ...any), onProgress func(repoPoolAddProgressEvent)) []repoPoolAddOutcome {
	if workers <= 0 {
		workers = repoPoolAddDefaultWorkers
	}
	if workers > len(requests) && len(requests) > 0 {
		workers = len(requests)
	}
	type job struct {
		index int
		req   repoPoolAddRequest
	}
	jobs := make(chan job)
	outcomes := make([]repoPoolAddOutcome, len(requests))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				outcomes[j.index] = applyOneRepoPoolAdd(ctx, repoPoolPath, j.index, j.req, debugf, onProgress)
			}
		}()
	}
	for i, req := range requests {
		jobs <- job{index: i, req: req}
	}
	close(jobs)
	wg.Wait()
	return outcomes
}

func applyOneRepoPoolAdd(ctx context.Context, repoPoolPath string, reqIndex int, req repoPoolAddRequest, debugf func(string, ...any), onProgress func(repoPoolAddProgressEvent)) repoPoolAddOutcome {
	specInput := strings.TrimSpace(req.RepoSpecInput)
	progressKey := resolveRepoPoolDisplayName(req)
	if onProgress != nil {
		onProgress(repoPoolAddProgressEvent{Index: reqIndex, Type: repoPoolAddProgressStart, RepoKey: progressKey})
	}

	outcome := repoPoolAddOutcome{RepoKey: progressKey}
	if specInput == "" {
		outcome.Success = false
		outcome.Reason = "repo spec is empty"
		emitRepoPoolDone(onProgress, reqIndex, outcome)
		return outcome
	}

	spec, err := repospec.Normalize(specInput)
	if err != nil {
		outcome.Success = false
		outcome.Reason = err.Error()
		emitRepoPoolDone(onProgress, reqIndex, outcome)
		return outcome
	}
	repoUID := fmt.Sprintf("%s/%s/%s", spec.Host, spec.Owner, spec.Repo)
	repoKey := fmt.Sprintf("%s/%s", spec.Owner, spec.Repo)
	outcome.RepoKey = repoKey

	barePath := repostore.StorePath(repoPoolPath, spec)
	existingBare := false
	if fi, err := os.Stat(barePath); err == nil {
		if !fi.IsDir() {
			outcome.Success = false
			outcome.Reason = fmt.Sprintf("bare path is not a directory: %s", barePath)
			emitRepoPoolDone(onProgress, reqIndex, outcome)
			return outcome
		}
		existingBare = true
		existingURL, err := gitutil.RunBare(ctx, barePath, "config", "--get", "remote.origin.url")
		if err == nil && strings.TrimSpace(existingURL) != "" && strings.TrimSpace(existingURL) != specInput {
			outcome.Success = false
			outcome.Reason = fmt.Sprintf("remote_url mismatch (existing=%s)", strings.TrimSpace(existingURL))
			emitRepoPoolDone(onProgress, reqIndex, outcome)
			return outcome
		}
	}
	if existingBare {
		if req.AlreadyInRoot {
			if debugf != nil {
				debugf("repo pool upsert skipped already registered repo_uid=%s bare_path=%s", repoUID, barePath)
			}
			outcome.Success = true
			outcome.Skipped = true
			outcome.Reason = "already added in current root"
			emitRepoPoolDone(onProgress, reqIndex, outcome)
			return outcome
		}
		if debugf != nil {
			debugf("repo pool upsert reused existing bare repo_uid=%s bare_path=%s", repoUID, barePath)
		}
		outcome.Success = true
		emitRepoPoolDone(onProgress, reqIndex, outcome)
		return outcome
	}

	if _, err := gitutil.Run(ctx, "", "clone", "--bare", specInput, barePath); err != nil {
		outcome.Success = false
		outcome.Reason = err.Error()
		emitRepoPoolDone(onProgress, reqIndex, outcome)
		return outcome
	}
	// Keep origin tracking refs stable for later ws add-repo fetch/preflight.
	_, _ = gitutil.RunBare(ctx, barePath, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")

	if debugf != nil {
		debugf("repo pool upsert success repo_uid=%s bare_path=%s", repoUID, barePath)
	}
	outcome.Success = true
	emitRepoPoolDone(onProgress, reqIndex, outcome)
	return outcome
}

func emitRepoPoolDone(onProgress func(repoPoolAddProgressEvent), reqIndex int, outcome repoPoolAddOutcome) {
	if onProgress == nil {
		return
	}
	onProgress(repoPoolAddProgressEvent{
		Index:   reqIndex,
		Type:    repoPoolAddProgressDone,
		RepoKey: outcome.RepoKey,
		Success: outcome.Success,
		Reason:  outcome.Reason,
	})
}

func resolveRepoPoolDisplayName(req repoPoolAddRequest) string {
	name := strings.TrimSpace(req.DisplayName)
	if name != "" {
		return name
	}
	specInput := strings.TrimSpace(req.RepoSpecInput)
	if specInput == "" {
		return "(empty)"
	}
	if spec, err := repospec.Normalize(specInput); err == nil {
		return fmt.Sprintf("%s/%s", spec.Owner, spec.Repo)
	}
	return specInput
}

type repoPoolProgressStatus string

const (
	repoPoolProgressQueued  repoPoolProgressStatus = "queued"
	repoPoolProgressRunning repoPoolProgressStatus = "running"
	repoPoolProgressDone    repoPoolProgressStatus = "done"
	repoPoolProgressFailed  repoPoolProgressStatus = "failed"
)

type repoPoolProgressRow struct {
	name   string
	status repoPoolProgressStatus
	reason string
}

func printRepoPoolProgress(out io.Writer, useColor bool, requests []repoPoolAddRequest, events <-chan repoPoolAddProgressEvent) {
	rows := make([]repoPoolProgressRow, 0, len(requests))
	for _, req := range requests {
		rows = append(rows, repoPoolProgressRow{name: resolveRepoPoolDisplayName(req), status: repoPoolProgressQueued})
	}
	file, tty := out.(*os.File)
	if !tty || !writerIsTTY(file) {
		printRepoPoolProgressPlain(out, useColor, events)
		return
	}
	printRepoPoolProgressTTY(out, useColor, rows, events)
}

func printRepoPoolProgressPlain(out io.Writer, useColor bool, events <-chan repoPoolAddProgressEvent) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, renderProgressTitle(useColor))
	for ev := range events {
		switch ev.Type {
		case repoPoolAddProgressStart:
			prefix := "…"
			if useColor {
				prefix = styleInfo(prefix, useColor)
			}
			fmt.Fprintf(out, "%s%s %s\n", uiIndent, prefix, ev.RepoKey)
		case repoPoolAddProgressDone:
			if ev.Success {
				prefix := "✔"
				if useColor {
					prefix = styleSuccess(prefix, useColor)
				}
				fmt.Fprintf(out, "%s%s %s\n", uiIndent, prefix, ev.RepoKey)
				continue
			}
			prefix := "!"
			if useColor {
				prefix = styleError(prefix, useColor)
			}
			fmt.Fprintf(out, "%s%s %s (%s)\n", uiIndent, prefix, ev.RepoKey, ev.Reason)
		}
	}
}

func printRepoPoolProgressTTY(out io.Writer, useColor bool, rows []repoPoolProgressRow, events <-chan repoPoolAddProgressEvent) {
	spinnerFrames := []string{"-", "\\", "|", "/"}
	spinnerIndex := 0
	printedLines := 0
	render := func() {
		lines := renderRepoPoolProgressLines(useColor, rows, spinnerFrames[spinnerIndex])
		if printedLines > 0 {
			fmt.Fprintf(out, "\x1b[%dA", printedLines)
		}
		for _, line := range lines {
			fmt.Fprintf(out, "\x1b[2K%s\n", line)
		}
		printedLines = len(lines)
	}

	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	channelOpen := true
	render()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				channelOpen = false
				render()
				return
			}
			if ev.Index >= 0 && ev.Index < len(rows) {
				if strings.TrimSpace(ev.RepoKey) != "" {
					rows[ev.Index].name = ev.RepoKey
				}
				switch ev.Type {
				case repoPoolAddProgressStart:
					rows[ev.Index].status = repoPoolProgressRunning
					rows[ev.Index].reason = ""
				case repoPoolAddProgressDone:
					if ev.Success {
						rows[ev.Index].status = repoPoolProgressDone
						rows[ev.Index].reason = ""
					} else {
						rows[ev.Index].status = repoPoolProgressFailed
						rows[ev.Index].reason = ev.Reason
					}
				}
			}
			spinnerIndex = (spinnerIndex + 1) % len(spinnerFrames)
			render()
		case <-ticker.C:
			if !channelOpen {
				continue
			}
			if !hasRepoPoolRunningRow(rows) {
				continue
			}
			spinnerIndex = (spinnerIndex + 1) % len(spinnerFrames)
			render()
		}
	}
}

func hasRepoPoolRunningRow(rows []repoPoolProgressRow) bool {
	for _, row := range rows {
		if row.status == repoPoolProgressRunning {
			return true
		}
	}
	return false
}

func renderRepoPoolProgressLines(useColor bool, rows []repoPoolProgressRow, spinner string) []string {
	lines := make([]string, 0, len(rows)+2)
	lines = append(lines, "")
	lines = append(lines, renderProgressTitle(useColor))
	for _, row := range rows {
		prefix := "·"
		switch row.status {
		case repoPoolProgressRunning:
			prefix = spinner
		case repoPoolProgressDone:
			prefix = "✔"
		case repoPoolProgressFailed:
			prefix = "!"
		}
		if useColor {
			switch row.status {
			case repoPoolProgressRunning:
				prefix = styleInfo(prefix, useColor)
			case repoPoolProgressDone:
				prefix = styleSuccess(prefix, useColor)
			case repoPoolProgressFailed:
				prefix = styleError(prefix, useColor)
			default:
				prefix = styleMuted(prefix, useColor)
			}
		}
		line := fmt.Sprintf("%s%s %s", uiIndent, prefix, row.name)
		if row.status == repoPoolProgressFailed && strings.TrimSpace(row.reason) != "" {
			line = fmt.Sprintf("%s (%s)", line, row.reason)
		}
		lines = append(lines, line)
	}
	return lines
}

func printRepoPoolSection(out io.Writer, requests []repoPoolAddRequest, useColor bool) {
	fmt.Fprintln(out, styleBold("Repo pool:", useColor))
	fmt.Fprintln(out)
	if len(requests) == 0 {
		fmt.Fprintf(out, "%s(none)\n", uiIndent)
		return
	}
	for _, req := range requests {
		fmt.Fprintf(out, "%s- %s\n", uiIndent, resolveRepoPoolDisplayName(req))
	}
}

func printRepoPoolAddResult(out io.Writer, outcomes []repoPoolAddOutcome, useColor bool) {
	total := len(outcomes)
	added := 0
	skipped := 0
	failed := 0
	for _, r := range outcomes {
		if !r.Success {
			failed++
			continue
		}
		if r.Skipped {
			skipped++
			continue
		}
		added++
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, renderResultTitle(useColor))
	summary := fmt.Sprintf("Added %d / %d", added, total)
	if useColor {
		switch {
		case total == 0:
			summary = styleMuted(summary, useColor)
		case failed == 0 && added == total:
			summary = styleSuccess(summary, useColor)
		case failed == 0 && skipped == total:
			summary = styleMuted(summary, useColor)
		case failed == 0:
			summary = styleInfo(summary, useColor)
		case added == 0:
			summary = styleError(summary, useColor)
		default:
			summary = styleWarn(summary, useColor)
		}
	}
	fmt.Fprintf(out, "%s%s\n", uiIndent, summary)
	if skipped > 0 {
		line := fmt.Sprintf("Skipped %d (already added in current root)", skipped)
		if useColor {
			line = styleMuted(line, useColor)
		}
		fmt.Fprintf(out, "%s%s\n", uiIndent, line)
	}
	for _, r := range outcomes {
		if r.Success && r.Skipped {
			prefix := "-"
			if useColor {
				prefix = styleMuted(prefix, useColor)
			}
			fmt.Fprintf(out, "%s%s %s\n", uiIndent, prefix, r.RepoKey)
			continue
		}
		if !r.Success {
			prefix := "!"
			if useColor {
				prefix = styleError(prefix, useColor)
			}
			fmt.Fprintf(out, "%s%s %s (reason: %s)\n", uiIndent, prefix, r.RepoKey, r.Reason)
		}
	}
}

func repoPoolAddHadFailure(outcomes []repoPoolAddOutcome) bool {
	for _, r := range outcomes {
		if !r.Success {
			return true
		}
	}
	return false
}
