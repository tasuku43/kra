package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tasuku43/gion-core/repospec"
	"github.com/tasuku43/gion-core/repostore"
	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/statestore"
)

const repoPoolAddDefaultWorkers = 4

type repoPoolAddRequest struct {
	RepoSpecInput string
	DisplayName   string
}

type repoPoolAddOutcome struct {
	RepoKey string
	Success bool
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

func applyRepoPoolAddsWithProgress(ctx context.Context, db *sql.DB, repoPoolPath string, requests []repoPoolAddRequest, workers int, debugf func(string, ...any), progressOut io.Writer, useColor bool) []repoPoolAddOutcome {
	if workers <= 0 {
		workers = repoPoolAddDefaultWorkers
	}
	progressEvents := make(chan repoPoolAddProgressEvent, workers*2+1)
	done := make(chan struct{})
	go func() {
		printRepoPoolProgress(progressOut, useColor, requests, progressEvents)
		close(done)
	}()

	outcomes := applyRepoPoolAdds(ctx, db, repoPoolPath, requests, workers, debugf, func(ev repoPoolAddProgressEvent) {
		progressEvents <- ev
	})
	close(progressEvents)
	<-done
	return outcomes
}

func applyRepoPoolAdds(ctx context.Context, db *sql.DB, repoPoolPath string, requests []repoPoolAddRequest, workers int, debugf func(string, ...any), onProgress func(repoPoolAddProgressEvent)) []repoPoolAddOutcome {
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
				outcomes[j.index] = applyOneRepoPoolAdd(ctx, db, repoPoolPath, j.index, j.req, debugf, onProgress)
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

func applyOneRepoPoolAdd(ctx context.Context, db *sql.DB, repoPoolPath string, reqIndex int, req repoPoolAddRequest, debugf func(string, ...any), onProgress func(repoPoolAddProgressEvent)) repoPoolAddOutcome {
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

	existingURL, ok, err := statestore.LookupRepoRemoteURL(ctx, db, repoUID)
	if err != nil {
		outcome.Success = false
		outcome.Reason = err.Error()
		emitRepoPoolDone(onProgress, reqIndex, outcome)
		return outcome
	}
	if ok && strings.TrimSpace(existingURL) != specInput {
		outcome.Success = false
		outcome.Reason = fmt.Sprintf("remote_url mismatch (existing=%s)", existingURL)
		emitRepoPoolDone(onProgress, reqIndex, outcome)
		return outcome
	}

	defaultBranch, err := gitutil.DefaultBranchFromRemote(ctx, specInput)
	if err != nil {
		outcome.Success = false
		outcome.Reason = err.Error()
		emitRepoPoolDone(onProgress, reqIndex, outcome)
		return outcome
	}
	barePath := repostore.StorePath(repoPoolPath, spec)
	if _, err := gitutil.EnsureBareRepoFetched(ctx, specInput, barePath, defaultBranch); err != nil {
		outcome.Success = false
		outcome.Reason = err.Error()
		emitRepoPoolDone(onProgress, reqIndex, outcome)
		return outcome
	}

	now := time.Now().Unix()
	if err := statestore.EnsureRepo(ctx, db, statestore.EnsureRepoInput{
		RepoUID:   repoUID,
		RepoKey:   repoKey,
		RemoteURL: specInput,
		Now:       now,
	}); err != nil {
		outcome.Success = false
		outcome.Reason = err.Error()
		emitRepoPoolDone(onProgress, reqIndex, outcome)
		return outcome
	}
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
				prefix = styleInfo(prefix, true)
			}
			fmt.Fprintf(out, "%s%s %s\n", uiIndent, prefix, ev.RepoKey)
		case repoPoolAddProgressDone:
			if ev.Success {
				prefix := "✔"
				if useColor {
					prefix = styleSuccess(prefix, true)
				}
				fmt.Fprintf(out, "%s%s %s\n", uiIndent, prefix, ev.RepoKey)
				continue
			}
			prefix := "!"
			if useColor {
				prefix = styleError(prefix, true)
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
				prefix = styleInfo(prefix, true)
			case repoPoolProgressDone:
				prefix = styleSuccess(prefix, true)
			case repoPoolProgressFailed:
				prefix = styleError(prefix, true)
			default:
				prefix = styleMuted(prefix, true)
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

func printRepoPoolSection(out io.Writer, requests []repoPoolAddRequest) {
	fmt.Fprintln(out, "Repo pool:")
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
	success := 0
	for _, r := range outcomes {
		if r.Success {
			success++
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, renderResultTitle(useColor))
	summary := fmt.Sprintf("Added %d / %d", success, total)
	if useColor {
		switch {
		case total > 0 && success == total:
			summary = styleSuccess(summary, true)
		case success == 0:
			summary = styleError(summary, true)
		default:
			summary = styleWarn(summary, true)
		}
	}
	fmt.Fprintf(out, "%s%s\n", uiIndent, summary)
	for _, r := range outcomes {
		if !r.Success {
			prefix := "!"
			if useColor {
				prefix = styleError(prefix, true)
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
