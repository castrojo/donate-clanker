package config

import (
	"errors"
	"strings"
	"testing"
)

func TestParseDefaultsToGooseCopilot(t *testing.T) {
	opts, err := Parse(nil, map[string]string{"HOME": "/home/tester"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.GooseProvider != DefaultGooseProvider {
		t.Fatalf("GooseProvider = %q, want %q", opts.GooseProvider, DefaultGooseProvider)
	}
	if Backend != "goose" {
		t.Fatalf("Backend = %q, want goose", Backend)
	}
	if !strings.HasPrefix(opts.HiveConfigDir, "/home/tester") {
		t.Fatalf("HiveConfigDir = %q", opts.HiveConfigDir)
	}
}

func TestParseRejectsNonHexCommit(t *testing.T) {
	_, err := Parse([]string{"-hive-commit", "not-a-sha"}, map[string]string{"HOME": "/home/tester"})
	if !errors.Is(err, ErrInvalidCommit) {
		t.Fatalf("error = %v, want ErrInvalidCommit", err)
	}
}

func TestParseAcceptsFullCommit(t *testing.T) {
	sha := "e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e"
	opts, err := Parse([]string{"-hive-commit", sha}, map[string]string{"HOME": "/home/tester"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.HiveCommit != sha {
		t.Fatalf("HiveCommit = %q", opts.HiveCommit)
	}
}

func TestParseReadsProviderFromEnvironment(t *testing.T) {
	opts, err := Parse(nil, map[string]string{
		"HOME":           "/home/tester",
		"GOOSE_PROVIDER": "anthropic",
		"GOOSE_MODEL":    "claude-sonnet-4",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.GooseProvider != "anthropic" || opts.GooseModel != "claude-sonnet-4" {
		t.Fatalf("unexpected provider/model: %q/%q", opts.GooseProvider, opts.GooseModel)
	}
}

func TestParseFlagsBeatEnvironment(t *testing.T) {
	opts, err := Parse([]string{"-goose-provider", "openai"}, map[string]string{
		"HOME":           "/home/tester",
		"GOOSE_PROVIDER": "anthropic",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if opts.GooseProvider != "openai" {
		t.Fatalf("GooseProvider = %q, want openai", opts.GooseProvider)
	}
}
