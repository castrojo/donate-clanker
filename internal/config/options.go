package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	defaultReadinessTimeout = 2 * time.Minute

	// Backend is fixed. donate-clanker runs Goose and nothing else.
	Backend = "goose"

	// DefaultGooseProvider matches the launcher default.
	DefaultGooseProvider = "github_copilot"
)

var (
	ErrMissingHiveConfig = errors.New("missing Hive configuration")
	ErrInvalidCommit     = errors.New("invalid Hive commit")
)

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Options is the launcher's host-side configuration. It deliberately carries no
// container-engine, model-profile, or workspace-mount settings: donate-clanker
// no longer runs inference and no longer mounts a workspace into the guest.
type Options struct {
	HiveConfigDir  string
	HiveSourceDir  string
	HiveCommit     string
	GooseProvider  string
	GooseModel     string
	RunnerImage    string
	StateDir       string
	NonInteractive bool
	ReadyTimeout   time.Duration
}

func Parse(args []string, env map[string]string) (Options, error) {
	defaults, err := defaultOptions(env)
	if err != nil {
		return Options{}, err
	}

	fs := flag.NewFlagSet("donate-clanker", flag.ContinueOnError)
	fs.SetOutput(nil)

	hiveConfigDir := fs.String("hive-config-dir", defaults.HiveConfigDir, "Hive config directory")
	hiveSourceDir := fs.String("hive-source-dir", defaults.HiveSourceDir, "upstream Hive checkout location")
	hiveCommit := fs.String("hive-commit", defaults.HiveCommit, "immutable upstream Hive commit (40-hex SHA)")
	gooseProvider := fs.String("goose-provider", defaults.GooseProvider, "Goose provider to pass through")
	gooseModel := fs.String("goose-model", defaults.GooseModel, "Goose model to pass through (optional)")
	runnerImage := fs.String("runner-image", defaults.RunnerImage, "pinned VM runner image reference")
	stateDir := fs.String("state-dir", defaults.StateDir, "per-run control directory")
	nonInteractive := fs.Bool("non-interactive", defaults.NonInteractive, "disable prompts")
	readyTimeout := fs.Duration("ready-timeout", defaults.ReadyTimeout, "guest readiness timeout")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	opts := Options{
		HiveConfigDir:  normalizePath(*hiveConfigDir),
		HiveSourceDir:  normalizePath(*hiveSourceDir),
		HiveCommit:     strings.ToLower(strings.TrimSpace(*hiveCommit)),
		GooseProvider:  strings.TrimSpace(*gooseProvider),
		GooseModel:     strings.TrimSpace(*gooseModel),
		RunnerImage:    strings.TrimSpace(*runnerImage),
		StateDir:       normalizePath(*stateDir),
		NonInteractive: *nonInteractive,
		ReadyTimeout:   *readyTimeout,
	}

	if opts.HiveConfigDir == "" {
		return Options{}, ErrMissingHiveConfig
	}
	if opts.HiveCommit != "" && !commitPattern.MatchString(opts.HiveCommit) {
		return Options{}, fmt.Errorf("%w: %q is not a 40-hex SHA", ErrInvalidCommit, opts.HiveCommit)
	}
	if opts.GooseProvider == "" {
		opts.GooseProvider = DefaultGooseProvider
	}

	return opts, nil
}

func defaultOptions(env map[string]string) (Options, error) {
	home := strings.TrimSpace(env["HOME"])
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Options{}, err
		}
	}

	return Options{
		HiveConfigDir: firstNonEmpty(env["DONATE_CLANKER_HIVE_CONFIG_DIR"], filepath.Join(home, ".config", "hive")),
		HiveSourceDir: firstNonEmpty(env["DONATE_CLANKER_HIVE_SOURCE_DIR"], filepath.Join(home, ".local", "state", "donate-clanker", "hive-src")),
		HiveCommit:    strings.TrimSpace(env["DONATE_CLANKER_HIVE_COMMIT"]),
		GooseProvider: firstNonEmpty(env["GOOSE_PROVIDER"], DefaultGooseProvider),
		GooseModel:    strings.TrimSpace(env["GOOSE_MODEL"]),
		RunnerImage:   strings.TrimSpace(env["DONATE_CLANKER_VM_RUNNER_IMAGE"]),
		StateDir:      firstNonEmpty(env["DONATE_CLANKER_STATE_DIR"], filepath.Join(home, ".local", "state", "donate-clanker")),
		ReadyTimeout:  defaultReadinessTimeout,
	}, nil
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
