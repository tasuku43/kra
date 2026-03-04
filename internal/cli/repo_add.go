package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tasuku43/kra/internal/app/repocmd"
	"github.com/tasuku43/kra/internal/core/repospec"
	"github.com/tasuku43/kra/internal/core/repostore"
	"github.com/tasuku43/kra/internal/infra/appports"
)

func (c *CLI) runRepoAdd(args []string) int {
	outputFormat := "human"
	repoSpecs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "-h", "--help", "help":
			c.printRepoAddUsage(c.Out)
			return exitOK
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(c.Err, "--format requires a value")
				c.printRepoAddUsage(c.Err)
				return exitUsage
			}
			outputFormat = strings.TrimSpace(args[i+1])
			i++
		default:
			if strings.HasPrefix(arg, "--format=") {
				outputFormat = strings.TrimSpace(strings.TrimPrefix(arg, "--format="))
				continue
			}
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(c.Err, "unknown flag for repo add: %q\n", arg)
				c.printRepoAddUsage(c.Err)
				return exitUsage
			}
			repoSpecs = append(repoSpecs, arg)
		}
	}
	switch outputFormat {
	case "human", "json":
	default:
		fmt.Fprintf(c.Err, "unsupported --format: %q (supported: human, json)\n", outputFormat)
		c.printRepoAddUsage(c.Err)
		return exitUsage
	}
	if len(repoSpecs) == 0 {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.add",
				Error: &cliJSONError{
					Code:    "invalid_argument",
					Message: "repo add requires at least one <repo-spec>",
				},
			})
			return exitUsage
		}
		c.printRepoAddUsage(c.Err)
		return exitUsage
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(c.Err, "get working dir: %v\n", err)
		return exitError
	}
	ctx := context.Background()
	repoUC := repocmd.NewService(appports.NewRepoPort(c.ensureDebugLog, c.touchStateRegistry))
	session, err := repoUC.Run(ctx, repocmd.Request{
		CWD:           wd,
		DebugTag:      "repo-add",
		RequireGit:    true,
		TouchRegistry: true,
	})
	if err != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.add",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: err.Error(),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "%v\n", err)
		return exitError
	}
	c.debugf("run repo add count=%d", len(repoSpecs))

	existing, err := loadRootRepoRegistry(session.Root)
	if err != nil {
		if outputFormat == "json" {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.add",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("load root repo registry: %v", err),
				},
			})
			return exitError
		}
		fmt.Fprintf(c.Err, "load root repo registry: %v\n", err)
		return exitError
	}
	existingByUID := make(map[string]rootRepoRegistryEntry, len(existing))
	for _, it := range existing {
		existingByUID[strings.TrimSpace(it.RepoUID)] = it
	}

	requests := make([]repoPoolAddRequest, 0, len(repoSpecs))
	for _, arg := range repoSpecs {
		specInput := strings.TrimSpace(arg)
		req := repoPoolAddRequest{RepoSpecInput: specInput}
		if spec, normErr := repospec.Normalize(specInput); normErr == nil {
			repoKey := fmt.Sprintf("%s/%s", spec.Owner, spec.Repo)
			repoUID := fmt.Sprintf("%s/%s", spec.Host, repoKey)
			if existingEntry, ok := existingByUID[repoUID]; ok && strings.TrimSpace(existingEntry.RemoteURL) == specInput {
				// Only skip when both root index and bare pool entry already exist.
				barePath := repostore.StorePath(session.RepoPoolPath, spec)
				if fi, statErr := os.Stat(barePath); statErr == nil && fi.IsDir() {
					req.AlreadyInRoot = true
				}
			}
		}
		requests = append(requests, req)
	}
	if outputFormat == "json" {
		outcomes := applyRepoPoolAdds(ctx, session.RepoPoolPath, requests, repoPoolAddDefaultWorkers, c.debugf, nil)
		if err := syncRootRepoRegistryFromPoolAddRequests(session.Root, requests, outcomes); err != nil {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     false,
				Action: "repo.add",
				Error: &cliJSONError{
					Code:    "internal_error",
					Message: fmt.Sprintf("update root repo registry: %v", err),
				},
			})
			return exitError
		}
		items := make([]map[string]any, 0, len(outcomes))
		added := 0
		failed := 0
		skipped := 0
		for _, o := range outcomes {
			if !o.Success {
				failed++
			} else if o.Skipped {
				skipped++
			} else {
				added++
			}
			items = append(items, map[string]any{
				"repo_key": o.RepoKey,
				"success":  o.Success,
				"skipped":  o.Skipped,
				"reason":   strings.TrimSpace(o.Reason),
			})
		}
		if failed == 0 {
			_ = writeCLIJSON(c.Out, cliJSONResponse{
				OK:     true,
				Action: "repo.add",
				Result: map[string]any{
					"added":   added,
					"skipped": skipped,
					"total":   len(outcomes),
					"items":   items,
				},
			})
			return exitOK
		}
		_ = writeCLIJSON(c.Out, cliJSONResponse{
			OK:     false,
			Action: "repo.add",
			Result: map[string]any{
				"added":   added,
				"skipped": skipped,
				"total":   len(outcomes),
				"items":   items,
			},
			Error: &cliJSONError{
				Code:    "conflict",
				Message: fmt.Sprintf("failed to add %d repo(s)", failed),
			},
		})
		return exitError
	}

	useColorOut := writerSupportsColor(c.Out)
	printRepoPoolSection(c.Out, requests, useColorOut)
	outcomes := applyRepoPoolAddsWithProgress(ctx, session.RepoPoolPath, requests, repoPoolAddDefaultWorkers, c.debugf, c.Out, useColorOut)
	if err := syncRootRepoRegistryFromPoolAddRequests(session.Root, requests, outcomes); err != nil {
		fmt.Fprintf(c.Err, "update root repo registry: %v\n", err)
		return exitError
	}
	printRepoPoolAddResult(c.Out, outcomes, useColorOut)
	if repoPoolAddHadFailure(outcomes) {
		return exitError
	}
	return exitOK
}

func syncRootRepoRegistryFromPoolAddRequests(root string, requests []repoPoolAddRequest, outcomes []repoPoolAddOutcome) error {
	if len(requests) == 0 || len(outcomes) == 0 {
		return nil
	}
	additions := make([]rootRepoRegistryEntry, 0, len(outcomes))
	limit := len(outcomes)
	if len(requests) < limit {
		limit = len(requests)
	}
	for i := 0; i < limit; i++ {
		if !outcomes[i].Success || outcomes[i].Skipped {
			continue
		}
		spec, err := repospec.Normalize(strings.TrimSpace(requests[i].RepoSpecInput))
		if err != nil {
			continue
		}
		repoKey := fmt.Sprintf("%s/%s", spec.Owner, spec.Repo)
		additions = append(additions, rootRepoRegistryEntry{
			RepoUID:   fmt.Sprintf("%s/%s", spec.Host, repoKey),
			RepoKey:   repoKey,
			RemoteURL: strings.TrimSpace(requests[i].RepoSpecInput),
		})
	}
	if len(additions) == 0 {
		return nil
	}
	return upsertRootRepoRegistryEntries(root, additions)
}
