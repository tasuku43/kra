package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tasuku43/gion-core/repospec"
	"github.com/tasuku43/gion-core/repostore"
	"github.com/tasuku43/gionx/internal/gitutil"
	"github.com/tasuku43/gionx/internal/statestore"
)

type repoPoolAddRequest struct {
	RepoSpecInput string
	DisplayName   string
}

type repoPoolAddOutcome struct {
	RepoKey string
	Success bool
	Reason  string
}

func applyRepoPoolAdds(ctx context.Context, db *sql.DB, repoPoolPath string, requests []repoPoolAddRequest, debugf func(string, ...any)) []repoPoolAddOutcome {
	outcomes := make([]repoPoolAddOutcome, 0, len(requests))
	for _, req := range requests {
		specInput := strings.TrimSpace(req.RepoSpecInput)
		displayName := strings.TrimSpace(req.DisplayName)
		if displayName == "" {
			displayName = specInput
		}
		if specInput == "" {
			outcomes = append(outcomes, repoPoolAddOutcome{
				RepoKey: displayName,
				Success: false,
				Reason:  "repo spec is empty",
			})
			continue
		}

		spec, err := repospec.Normalize(specInput)
		if err != nil {
			outcomes = append(outcomes, repoPoolAddOutcome{
				RepoKey: displayName,
				Success: false,
				Reason:  err.Error(),
			})
			continue
		}
		repoUID := fmt.Sprintf("%s/%s/%s", spec.Host, spec.Owner, spec.Repo)
		repoKey := fmt.Sprintf("%s/%s", spec.Owner, spec.Repo)

		existingURL, ok, err := statestore.LookupRepoRemoteURL(ctx, db, repoUID)
		if err != nil {
			outcomes = append(outcomes, repoPoolAddOutcome{RepoKey: repoKey, Success: false, Reason: err.Error()})
			continue
		}
		if ok && strings.TrimSpace(existingURL) != specInput {
			outcomes = append(outcomes, repoPoolAddOutcome{
				RepoKey: repoKey,
				Success: false,
				Reason:  fmt.Sprintf("remote_url mismatch (existing=%s)", existingURL),
			})
			continue
		}

		defaultBranch, err := gitutil.DefaultBranchFromRemote(ctx, specInput)
		if err != nil {
			outcomes = append(outcomes, repoPoolAddOutcome{RepoKey: repoKey, Success: false, Reason: err.Error()})
			continue
		}
		barePath := repostore.StorePath(repoPoolPath, spec)
		if _, err := gitutil.EnsureBareRepoFetched(ctx, specInput, barePath, defaultBranch); err != nil {
			outcomes = append(outcomes, repoPoolAddOutcome{RepoKey: repoKey, Success: false, Reason: err.Error()})
			continue
		}

		now := time.Now().Unix()
		if err := statestore.EnsureRepo(ctx, db, statestore.EnsureRepoInput{
			RepoUID:   repoUID,
			RepoKey:   repoKey,
			RemoteURL: specInput,
			Now:       now,
		}); err != nil {
			outcomes = append(outcomes, repoPoolAddOutcome{RepoKey: repoKey, Success: false, Reason: err.Error()})
			continue
		}
		if debugf != nil {
			debugf("repo pool upsert success repo_uid=%s bare_path=%s", repoUID, barePath)
		}
		outcomes = append(outcomes, repoPoolAddOutcome{RepoKey: repoKey, Success: true})
	}
	return outcomes
}

func printRepoPoolSection(out io.Writer, requests []repoPoolAddRequest) {
	fmt.Fprintln(out, "Repo pool:")
	fmt.Fprintln(out)
	if len(requests) == 0 {
		fmt.Fprintf(out, "%s(none)\n", uiIndent)
		return
	}
	for _, req := range requests {
		name := strings.TrimSpace(req.DisplayName)
		if name == "" {
			specInput := strings.TrimSpace(req.RepoSpecInput)
			if spec, err := repospec.Normalize(specInput); err == nil {
				name = fmt.Sprintf("%s/%s", spec.Owner, spec.Repo)
			} else {
				name = specInput
			}
		}
		if name == "" {
			name = "(empty)"
		}
		fmt.Fprintf(out, "%s- %s\n", uiIndent, name)
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
	fmt.Fprintf(out, "%sAdded %d / %d\n", uiIndent, success, total)
	for _, r := range outcomes {
		if r.Success {
			fmt.Fprintf(out, "%s+ %s\n", uiIndent, r.RepoKey)
			continue
		}
		fmt.Fprintf(out, "%s! %s (reason: %s)\n", uiIndent, r.RepoKey, r.Reason)
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
