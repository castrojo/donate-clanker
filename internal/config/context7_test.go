package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadBundledGooseConfig(t *testing.T) {
	path := repoFile(t, "image", "config", "goose.yaml")

	data, err := LoadBundledGooseConfig(path)
	if err != nil {
		t.Fatalf("LoadBundledGooseConfig() error = %v", err)
	}

	got := string(data)
	for _, want := range []string{
		"context7:",
		"bundled: false",
		"enabled: true",
		"name: context7",
		"timeout: 30",
		"type: streamable_http",
		`uri: "https://mcp.context7.com/mcp"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("LoadBundledGooseConfig() missing %q in %q", want, got)
		}
	}
}

func TestLoadBundledGooseConfigMissing(t *testing.T) {
	_, err := LoadBundledGooseConfig(filepath.Join(repoScratch(t), "missing.yaml"))
	if !errors.Is(err, ErrMissingFile) {
		t.Fatalf("LoadBundledGooseConfig() error = %v, want ErrMissingFile", err)
	}
}

func TestLoadBundledGooseConfigEmpty(t *testing.T) {
	path := writeScratchFile(t, "empty.yaml", []byte(" \n\t"))

	_, err := LoadBundledGooseConfig(path)
	if !errors.Is(err, ErrEmptyFile) {
		t.Fatalf("LoadBundledGooseConfig() error = %v, want ErrEmptyFile", err)
	}
}

func TestLoadLocalAgentPolicy(t *testing.T) {
	path := repoFile(t, "image", "config", "local-agent-policy.md")

	policy, err := LoadLocalAgentPolicy(path)
	if err != nil {
		t.Fatalf("LoadLocalAgentPolicy() error = %v", err)
	}

	for _, want := range []string{
		"inspect local repository evidence first",
		"Context7 only when current external documentation is useful",
		"continue with local evidence when Context7 is unavailable",
	} {
		if !strings.Contains(policy, want) {
			t.Fatalf("LoadLocalAgentPolicy() missing %q in %q", want, policy)
		}
	}
}

func TestLoadLocalAgentPolicyTooLarge(t *testing.T) {
	path := writeScratchFile(t, "oversized-policy.md", bytesOfSize(policyMaxSize+1))

	_, err := LoadLocalAgentPolicy(path)
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("LoadLocalAgentPolicy() error = %v, want ErrFileTooLarge", err)
	}
}

func TestDefaultGooseEnvironmentDisablesThinking(t *testing.T) {
	env := DefaultGooseEnvironment("Qwen3.6-35B-A3B")

	for key, want := range map[string]string{
		"GOOSE_PROVIDER":        "openai",
		"GOOSE_MODEL":           "Qwen3.6-35B-A3B",
		"GOOSE_THINKING_EFFORT": "off",
		"OPENAI_BASE_URL":       "http://127.0.0.1:8000/v1",
		"OPENAI_API_KEY":        "local",
	} {
		if got := env[key]; got != want {
			t.Fatalf("DefaultGooseEnvironment()[%q] = %q, want %q", key, got, want)
		}
	}
}

func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	root := repoRoot(t)
	return filepath.Join(append([]string{root}, parts...)...)
}

func repoScratch(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "internal", "config", ".context7-test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func writeScratchFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	dir := repoScratch(t)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func bytesOfSize(n int64) []byte {
	return []byte(strings.Repeat("x", int(n)))
}
