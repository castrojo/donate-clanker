package profile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadCatalogProfiles(t *testing.T) {
	catalog, err := Load(repoFile(t, "image", "config", "models.json"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	wantThinking := []string{
		"Qwen3.5-4B",
		"Qwen3.5-9B",
		"Qwen3-Coder-30B-A3B",
		"Qwen3.6-35B-A3B",
	}
	for _, id := range wantThinking {
		profile, ok := catalog[id]
		if !ok {
			t.Fatalf("Load() missing profile %q", id)
		}
		if profile.Thinking {
			t.Fatalf("Load() profile %q has Thinking=true", id)
		}
		if profile.ContextSize != defaultContextSize {
			t.Fatalf("Load() profile %q context_size = %d, want %d", id, profile.ContextSize, defaultContextSize)
		}
		if !hasRuntimeArgs(profile.RuntimeArgs, "--thinking", "false") {
			t.Fatalf("Load() profile %q missing --thinking false", id)
		}
	}

	for _, id := range []string{"Qwen3.5-4B", "Qwen3.5-9B", "Qwen3.6-35B-A3B"} {
		profile := catalog[id]
		if !hasRuntimeArgs(profile.RuntimeArgs, "--chat-template-kwargs", `{"enable_thinking":false}`) {
			t.Fatalf("Load() profile %q missing chat template kwargs", id)
		}
	}

	if hasRuntimeArgs(catalog["Qwen3-Coder-30B-A3B"].RuntimeArgs, "--chat-template-kwargs", `{"enable_thinking":false}`) {
		t.Fatal("Load() Qwen3-Coder-30B-A3B unexpectedly has chat template kwargs")
	}
}

func TestLoadRejectsThinkingEnabledProfile(t *testing.T) {
	path := writeScratchFile(t, "thinking.json", []byte(`{
		"bad": {
			"context_size": 32768,
			"thinking": true,
			"runtime_args": ["--thinking", "false"]
		}
	}`))

	_, err := Load(path)
	if !errors.Is(err, ErrThinkingEnabled) {
		t.Fatalf("Load() error = %v, want ErrThinkingEnabled", err)
	}
}

func TestLoadRejectsMissingServerDisable(t *testing.T) {
	path := writeScratchFile(t, "missing-disable.json", []byte(`{
		"bad": {
			"context_size": 32768,
			"thinking": false,
			"runtime_args": ["--context-size", "32768"]
		}
	}`))

	_, err := Load(path)
	if !errors.Is(err, ErrMissingServerDisable) {
		t.Fatalf("Load() error = %v, want ErrMissingServerDisable", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(repoScratch(t), "missing.json"))
	if !errors.Is(err, ErrMissingFile) {
		t.Fatalf("Load() error = %v, want ErrMissingFile", err)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	path := writeScratchFile(t, "empty.json", []byte(" \n\t"))

	_, err := Load(path)
	if !errors.Is(err, ErrEmptyFile) {
		t.Fatalf("Load() error = %v, want ErrEmptyFile", err)
	}
}

func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	return filepath.Join(append([]string{repoRoot(t)}, parts...)...)
}

func repoScratch(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "internal", "profile", ".profile-test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func writeScratchFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(repoScratch(t), name)
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
