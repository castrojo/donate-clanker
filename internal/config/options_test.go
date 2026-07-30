package config

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	opts, err := Parse(nil, map[string]string{
		"HOME": "/home/tester",
		"PWD":  "/work/repo",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if opts.Engine != EngineAuto {
		t.Fatalf("Parse() engine = %q, want %q", opts.Engine, EngineAuto)
	}
	if opts.Workspace != filepath.Clean("/work/repo") {
		t.Fatalf("Parse() workspace = %q, want /work/repo", opts.Workspace)
	}
	if opts.CacheDir != filepath.Clean("/home/tester/.local/state/donate-clanker/cache/ramalama") {
		t.Fatalf("Parse() cache_dir = %q", opts.CacheDir)
	}
	if opts.GooseConfigPath != filepath.Clean("/home/tester/.config/goose/config.yaml") {
		t.Fatalf("Parse() goose_config = %q", opts.GooseConfigPath)
	}
}

func TestParsePrefersExplicitHiveCommitOverride(t *testing.T) {
	opts, err := Parse([]string{"--hive-commit", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, map[string]string{
		"HOME":                       "/home/tester",
		"PWD":                        "/work/repo",
		"DONATE_CLANKER_HIVE_COMMIT": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.HiveCommit != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("Parse() hive commit = %q, want flag override", opts.HiveCommit)
	}
}

func TestParseRejectsConflictingProfileAndModel(t *testing.T) {
	_, err := Parse([]string{"--profile", "Qwen3.5-4B", "--model", "override"}, map[string]string{
		"HOME": "/home/tester",
		"PWD":  "/work/repo",
	})
	if !errors.Is(err, ErrConflictingSelection) {
		t.Fatalf("Parse() error = %v, want ErrConflictingSelection", err)
	}
}

func TestParseRejectsInvalidEngine(t *testing.T) {
	_, err := Parse([]string{"--engine", "banana"}, map[string]string{
		"HOME": "/home/tester",
		"PWD":  "/work/repo",
	})
	if !errors.Is(err, ErrInvalidEngine) {
		t.Fatalf("Parse() error = %v, want ErrInvalidEngine", err)
	}
}
