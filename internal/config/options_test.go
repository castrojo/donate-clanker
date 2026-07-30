package config

import (
	"errors"
	"path/filepath"
	"strings"
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

func TestParseRejectsProfileFlagBeforeRuntimeSelection(t *testing.T) {
	_, err := Parse([]string{"--profile", "Qwen3.5-4B", "--model", "override"}, map[string]string{
		"HOME": "/home/tester",
		"PWD":  "/work/repo",
	})
	if !errors.Is(err, ErrProfileUnsupported) {
		t.Fatalf("Parse() error = %v, want ErrProfileUnsupported", err)
	}
	if err == nil || !strings.Contains(err.Error(), "Qwen3.5-4B") {
		t.Fatalf("Parse() error = %v, want profile id in message", err)
	}
}

func TestParseRejectsProfileEnvironmentOverride(t *testing.T) {
	_, err := Parse(nil, map[string]string{
		"HOME":                   "/home/tester",
		"PWD":                    "/work/repo",
		"DONATE_CLANKER_PROFILE": "Qwen3.5-9B",
	})
	if !errors.Is(err, ErrProfileUnsupported) {
		t.Fatalf("Parse() error = %v, want ErrProfileUnsupported", err)
	}
	if err == nil || !strings.Contains(err.Error(), "Qwen3.5-9B") {
		t.Fatalf("Parse() error = %v, want profile id in message", err)
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
