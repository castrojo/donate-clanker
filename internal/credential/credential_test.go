package credential

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

type fakeRunner struct {
	out    string
	err    error
	called [][]string
}

func (f *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	f.called = append(f.called, append([]string{name}, args...))
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.out), nil
}

func TestResolvePrefersEnvironmentOverGh(t *testing.T) {
	runner := &fakeRunner{out: "gh-token"}
	got, err := Resolve(context.Background(), "github_copilot", "", map[string]string{"GITHUB_TOKEN": "env-token"}, runner)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Secret != "env-token" {
		t.Fatalf("Secret = %q, want env-token", got.Secret)
	}
	if len(runner.called) != 0 {
		t.Fatalf("gh was invoked despite an environment credential: %v", runner.called)
	}
}

func TestResolveFallsBackToGhAuthToken(t *testing.T) {
	runner := &fakeRunner{out: "gh-token\n"}
	got, err := Resolve(context.Background(), "github_copilot", "some-model", map[string]string{}, runner)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Secret != "gh-token" {
		t.Fatalf("Secret = %q, want gh-token", got.Secret)
	}
	if got.Model != "some-model" {
		t.Fatalf("Model = %q, want some-model", got.Model)
	}
	if got.Source != "gh auth token" {
		t.Fatalf("Source = %q", got.Source)
	}
}

// gh auth token is only consulted for github_copilot. Other providers must not
// silently receive a GitHub token as their model credential.
func TestResolveDoesNotUseGhTokenForOtherProviders(t *testing.T) {
	runner := &fakeRunner{out: "gh-token"}
	_, err := Resolve(context.Background(), "anthropic", "", map[string]string{}, runner)
	if !errors.Is(err, ErrNoCredential) {
		t.Fatalf("error = %v, want ErrNoCredential", err)
	}
	if len(runner.called) != 0 {
		t.Fatalf("gh was invoked for a non-Copilot provider: %v", runner.called)
	}
	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("error should name the expected variable, got %v", err)
	}
}

func TestResolveDefaultsToCopilot(t *testing.T) {
	got, err := Resolve(context.Background(), "", "", map[string]string{"GH_TOKEN": "t"}, &fakeRunner{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Provider != DefaultProvider {
		t.Fatalf("Provider = %q, want %q", got.Provider, DefaultProvider)
	}
}

func TestResolveOllamaNeedsNoSecret(t *testing.T) {
	got, err := Resolve(context.Background(), "ollama", "", map[string]string{}, &fakeRunner{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Secret != "" {
		t.Fatalf("Secret = %q, want empty", got.Secret)
	}
}

func TestResolveRejectsUnknownProvider(t *testing.T) {
	_, err := Resolve(context.Background(), "nonesuch", "", map[string]string{}, &fakeRunner{})
	if !errors.Is(err, ErrNoCredential) {
		t.Fatalf("error = %v, want ErrNoCredential", err)
	}
}

func TestResolveFailsLoudlyWhenGhUnavailable(t *testing.T) {
	runner := &fakeRunner{err: exec.ErrNotFound}
	_, err := Resolve(context.Background(), "github_copilot", "", map[string]string{}, runner)
	if !errors.Is(err, ErrNoCredential) {
		t.Fatalf("error = %v, want ErrNoCredential", err)
	}
	if !strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("error should tell the contributor how to fix it, got %v", err)
	}
}

// The resolver must never surface the secret in an error string.
func TestResolveErrorsDoNotLeakSecrets(t *testing.T) {
	_, err := Resolve(context.Background(), "nonesuch", "", map[string]string{"GITHUB_TOKEN": "super-secret"}, &fakeRunner{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("error leaked a credential: %v", err)
	}
}

func TestEnvironmentMap(t *testing.T) {
	env := EnvironmentMap([]string{"A=1", "B=2=3", "malformed"})
	if env["A"] != "1" || env["B"] != "2=3" {
		t.Fatalf("unexpected environment: %#v", env)
	}
	if _, ok := env["malformed"]; ok {
		t.Fatal("malformed entry should be skipped")
	}
}
