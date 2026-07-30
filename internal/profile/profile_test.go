package profile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCatalogJSONDeclaresThinkingFalse(t *testing.T) {
	data, err := os.ReadFile(repoFile(t, "image", "config", "models.json"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var raw map[string]map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	for _, id := range []string{
		"Qwen3.5-4B",
		"Qwen3.5-9B",
		"Qwen3-Coder-30B-A3B",
		"Qwen3.6-35B-A3B",
	} {
		profile, ok := raw[id]
		if !ok {
			t.Fatalf("models.json missing profile %q", id)
		}

		value, ok := profile["thinking"]
		if !ok {
			t.Fatalf("models.json profile %q omits thinking", id)
		}
		thinking, ok := value.(bool)
		if !ok || thinking {
			t.Fatalf("models.json profile %q thinking = %#v, want false", id, value)
		}
	}
}

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

func TestLoadRejectsMissingExplicitThinking(t *testing.T) {
	_, err := loadScratchCatalog(t, "missing-thinking.json", `{
		"bad": {
			"context_size": 32768,
			"runtime_args": ["--thinking", "false"]
		}
	}`)
	if !errors.Is(err, ErrThinkingRequired) {
		t.Fatalf("Load() error = %v, want ErrThinkingRequired", err)
	}
}

func TestLoadRejectsThinkingEnabledProfile(t *testing.T) {
	_, err := loadScratchCatalog(t, "thinking.json", `{
		"bad": {
			"context_size": 32768,
			"thinking": true,
			"runtime_args": ["--thinking", "false"]
		}
	}`)
	if !errors.Is(err, ErrThinkingEnabled) {
		t.Fatalf("Load() error = %v, want ErrThinkingEnabled", err)
	}
}

func TestLoadRejectsMissingServerDisable(t *testing.T) {
	_, err := loadScratchCatalog(t, "missing-disable.json", `{
		"bad": {
			"context_size": 32768,
			"thinking": false,
			"runtime_args": ["--context-size", "32768"]
		}
	}`)
	if !errors.Is(err, ErrMissingServerDisable) {
		t.Fatalf("Load() error = %v, want ErrMissingServerDisable", err)
	}
}

func TestLoadRejectsContradictoryThinkingArgs(t *testing.T) {
	_, err := loadScratchCatalog(t, "contradictory-thinking.json", `{
		"bad": {
			"context_size": 32768,
			"thinking": false,
			"runtime_args": ["--thinking", "false", "--thinking", "true"]
		}
	}`)
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

func loadScratchCatalog(t *testing.T, name, data string) (Catalog, error) {
	t.Helper()
	return Load(writeScratchFile(t, name, []byte(data)))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
