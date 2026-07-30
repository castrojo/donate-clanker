package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	defaultHiveRepoURL = "https://github.com/kubestellar/hive"
	// origin/v2 via `git ls-remote --heads https://github.com/kubestellar/hive v2`
	// on 2026-07-29.
	defaultHiveCommit = "e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e"
	hiveCommitEnv     = "DONATE_CLANKER_HIVE_COMMIT"
)

var immutableCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type SetupOptions struct {
	ConfigPath     string
	RepoDir        string
	RepoURL        string
	Commit         string
	NonInteractive bool
	Runner         CommandRunner
}

func EnsureHiveSetup(ctx context.Context, opts SetupOptions) error {
	if strings.TrimSpace(opts.ConfigPath) == "" {
		return fmt.Errorf("missing hive config path")
	}
	if err := ValidateHiveConfig(opts.ConfigPath); err == nil {
		return nil
	} else if !errors.Is(err, ErrMissingFile) {
		return err
	}

	runner := opts.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}

	repoDir := strings.TrimSpace(opts.RepoDir)
	if repoDir == "" {
		return fmt.Errorf("missing hive source directory")
	}

	commit, err := normalizeImmutableCommit(firstNonEmpty(opts.Commit, defaultHiveCommit))
	if err != nil {
		return err
	}
	repoURL := firstNonEmpty(opts.RepoURL, defaultHiveRepoURL)
	justArgs := []string{
		"--working-directory", repoDir,
		"--justfile", filepath.Join(repoDir, "Justfile"),
		"contribute-setup", "goose",
	}

	attachedRunner, ok := runner.(AttachedCommandRunner)
	if opts.NonInteractive {
		return missingInteractiveSetupError(opts.ConfigPath, "non-interactive mode cannot answer the upstream prompts", commit)
	}
	if !ok || !attachedRunner.CanRunAttached() {
		return missingInteractiveSetupError(opts.ConfigPath, "stdin/stdout/stderr are not attached to a terminal", commit)
	}
	if err := os.MkdirAll(filepath.Dir(repoDir), 0o700); err != nil {
		return err
	}

	if err := ensurePinnedHiveCheckout(ctx, runner, repoDir, repoURL, commit); err != nil {
		return err
	}

	result, err := attachedRunner.RunAttached(ctx, "just", justArgs...)
	if err != nil {
		detail := summarizeCommandOutput(result)
		if detail == "" {
			return fmt.Errorf("run upstream contribute-setup goose: %w", err)
		}
		return fmt.Errorf("run upstream contribute-setup goose: %s", detail)
	}

	if err := ValidateHiveConfig(opts.ConfigPath); err != nil {
		return err
	}
	return nil
}

func missingInteractiveSetupError(configPath, reason, commit string) error {
	return fmt.Errorf(
		"%w: missing Hive setup at %s; %s. Re-run donate-clanker from an interactive terminal, or pre-seed it yourself from kubestellar/hive @ %s by running `just contribute-setup goose` in an interactive checkout (set %s to another full commit if needed)",
		ErrInteractiveSetupRequired,
		configPath,
		reason,
		commit,
		hiveCommitEnv,
	)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeImmutableCommit(raw string) (string, error) {
	commit := strings.ToLower(strings.TrimSpace(raw))
	if commit == "" {
		return "", fmt.Errorf("missing Hive commit pin; set %s to a full 40-character commit SHA", hiveCommitEnv)
	}
	if !immutableCommitPattern.MatchString(commit) {
		return "", fmt.Errorf("Hive commit pin must be a full 40-character commit SHA; branch names like v2 are not allowed (set %s to override)", hiveCommitEnv)
	}
	return commit, nil
}

func ensurePinnedHiveCheckout(ctx context.Context, runner CommandRunner, repoDir, repoURL, commit string) error {
	gitDir := filepath.Join(repoDir, ".git")
	if _, err := os.Stat(gitDir); errors.Is(err, os.ErrNotExist) {
		if err := ensureManagedRepoDir(repoDir); err != nil {
			return err
		}
		if _, err := runSetupCommand(ctx, runner, "init hive source checkout", "git", "init", repoDir); err != nil {
			return err
		}
		if _, err := runSetupCommand(ctx, runner, "set hive source origin", "git", "-C", repoDir, "remote", "add", "origin", repoURL); err != nil {
			return err
		}
	} else if err == nil {
		status, err := runSetupCommand(ctx, runner, "inspect hive source checkout", "git", "-C", repoDir, "status", "--porcelain")
		if err != nil {
			return err
		}
		if strings.TrimSpace(status.Stdout) != "" {
			return fmt.Errorf("hive source dir %s has local changes; use a clean checkout or delete it so donate-clanker can recreate the pinned source", repoDir)
		}

		origin, err := runSetupCommand(ctx, runner, "inspect hive source origin", "git", "-C", repoDir, "remote", "get-url", "origin")
		if err != nil {
			return err
		}
		actualOrigin := strings.TrimSpace(origin.Stdout)
		if normalizeGitRemote(actualOrigin) != normalizeGitRemote(repoURL) {
			return fmt.Errorf("hive source dir %s points at %q, want %q", repoDir, actualOrigin, repoURL)
		}
	} else {
		return err
	}

	if _, err := runSetupCommand(ctx, runner, "fetch pinned hive source", "git", "-C", repoDir, "fetch", "--depth", "1", "origin", commit); err != nil {
		return err
	}
	if _, err := runSetupCommand(ctx, runner, "checkout pinned hive source", "git", "-C", repoDir, "checkout", "--detach", "-f", "FETCH_HEAD"); err != nil {
		return err
	}

	head, err := runSetupCommand(ctx, runner, "verify pinned hive source", "git", "-C", repoDir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(head.Stdout) != commit {
		return fmt.Errorf("verify pinned hive source: got %q, want %q", strings.TrimSpace(head.Stdout), commit)
	}

	return nil
}

func ensureManagedRepoDir(repoDir string) error {
	info, err := os.Stat(repoDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("hive source dir %s must be a directory", repoDir)
	}

	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("hive source dir %s exists but is not a managed git checkout; move it aside or choose an empty directory", repoDir)
	}

	return nil
}

func runSetupCommand(ctx context.Context, runner CommandRunner, action string, name string, args ...string) (CommandResult, error) {
	result, err := runner.Run(ctx, name, args...)
	if err == nil {
		return result, nil
	}

	detail := summarizeCommandOutput(result)
	if detail == "" {
		return CommandResult{}, fmt.Errorf("%s: %w", action, err)
	}
	return CommandResult{}, fmt.Errorf("%s: %s", action, detail)
}

func normalizeGitRemote(raw string) string {
	normalized := strings.TrimSpace(raw)
	normalized = strings.TrimPrefix(normalized, "ssh://")
	normalized = strings.TrimSuffix(normalized, ".git")
	normalized = strings.TrimSuffix(normalized, "/")

	switch {
	case strings.HasPrefix(normalized, "git@github.com:"):
		return "https://github.com/" + strings.TrimPrefix(normalized, "git@github.com:")
	case strings.HasPrefix(normalized, "git@github.com/"):
		return "https://github.com/" + strings.TrimPrefix(normalized, "git@github.com/")
	default:
		return normalized
	}
}
