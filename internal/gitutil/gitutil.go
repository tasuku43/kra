package gitutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tasuku43/kra/internal/core/gitparse"
	"github.com/tasuku43/kra/internal/core/gitref"
	"github.com/tasuku43/kra/internal/procexec"
)

const gitCommandTimeout = 2 * time.Minute

func EnsureGitInPath() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH: %w", err)
	}
	return nil
}

func Run(ctx context.Context, dir string, args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("git args are required")
	}
	if err := EnsureGitInPath(); err != nil {
		return "", err
	}

	out, err := procexec.RunCombined(ctx, dir, "git", gitCommandTimeout, args...)
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("git %s failed: %w (output=%s)", strings.Join(args, " "), err, s)
	}
	return s, nil
}

func RunBare(ctx context.Context, gitDir string, args ...string) (string, error) {
	if strings.TrimSpace(gitDir) == "" {
		return "", fmt.Errorf("git dir is required")
	}
	full := append([]string{"--git-dir", gitDir}, args...)
	return Run(ctx, "", full...)
}

func CheckRefFormat(ctx context.Context, refname string) error {
	refname = strings.TrimSpace(refname)
	if refname == "" {
		return fmt.Errorf("refname is required")
	}
	_, err := Run(ctx, "", "check-ref-format", refname)
	return err
}

func ShowRefExistsBare(ctx context.Context, gitDir string, ref string) (bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false, fmt.Errorf("ref is required")
	}
	if err := EnsureGitInPath(); err != nil {
		return false, err
	}

	out, _, err := procexec.RunOutput(ctx, "", "git", gitCommandTimeout, "--git-dir", gitDir, "show-ref", "--verify", "--quiet", ref)
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// git show-ref returns exit status 1 when not found.
		if exitErr.ExitCode() == 1 {
			return false, nil
		}
	}

	return false, fmt.Errorf("git show-ref --verify failed: %w (output=%s)", err, strings.TrimSpace(string(out)))
}

func IsIgnored(ctx context.Context, dir string, path string) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, fmt.Errorf("path is required")
	}
	if err := EnsureGitInPath(); err != nil {
		return false, err
	}

	out, _, err := procexec.RunOutput(ctx, dir, "git", gitCommandTimeout, "check-ignore", "--quiet", "--", path)
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// git check-ignore returns exit status 1 when not ignored.
		if exitErr.ExitCode() == 1 {
			return false, nil
		}
	}
	return false, fmt.Errorf("git check-ignore %s failed: %w (output=%s)", path, err, strings.TrimSpace(string(out)))
}

func DefaultBranchFromRemote(ctx context.Context, remoteURL string) (string, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", fmt.Errorf("remote url is required")
	}
	out, err := Run(ctx, "", "ls-remote", "--symref", remoteURL, "HEAD")
	if err != nil {
		return "", err
	}
	branch, _ := gitparse.ParseRemoteHeadSymref(out)
	if strings.TrimSpace(branch) == "" {
		return "", fmt.Errorf("failed to detect default branch from remote HEAD")
	}
	return branch, nil
}

func EnsureBareRepoFetched(ctx context.Context, remoteURL string, barePath string, fallbackDefaultBranch string) (defaultBaseRef string, err error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", fmt.Errorf("remote url is required")
	}
	barePath = strings.TrimSpace(barePath)
	if barePath == "" {
		return "", fmt.Errorf("bare path is required")
	}

	if _, statErr := os.Stat(barePath); statErr != nil {
		if !os.IsNotExist(statErr) {
			return "", fmt.Errorf("stat bare repo: %w", statErr)
		}
		if _, err := Run(ctx, "", "clone", "--bare", remoteURL, barePath); err != nil {
			return "", err
		}
	}

	// Ensure remote fetchspec is set (avoid narrow default).
	_ = func() error {
		_, err := RunBare(ctx, barePath, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
		return err
	}()

	// Try to ensure refs/remotes/origin/HEAD exists.
	_, _ = RunBare(ctx, barePath, "remote", "set-head", "origin", "-a")

	if _, err := RunBare(ctx, barePath, "fetch", "origin", "--prune"); err != nil {
		return "", err
	}

	if ref, err := RunBare(ctx, barePath, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		if b, ok := gitref.ParseOriginHeadRef(ref); ok {
			return "origin/" + b, nil
		}
	}

	fallbackDefaultBranch = strings.TrimSpace(fallbackDefaultBranch)
	if fallbackDefaultBranch != "" {
		return "origin/" + fallbackDefaultBranch, nil
	}

	return "", fmt.Errorf("failed to detect default base ref (origin/HEAD missing)")
}
