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
	if !strings.Contains(err.Error(), "goose configure") {
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

// scriptedRunner answers per command name, so a test can say "the keyring has
// this, gh has that" instead of handing every command the same output.
type scriptedRunner struct {
	responses map[string]string
	errs      map[string]error
	called    []string
}

func (s *scriptedRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	s.called = append(s.called, strings.Join(append([]string{name}, args...), " "))
	if err, ok := s.errs[name]; ok {
		return nil, err
	}
	out, ok := s.responses[name]
	if !ok {
		return nil, exec.ErrNotFound
	}
	return []byte(out), nil
}

const keyringEntry = `{"GITHUB_COPILOT_TOKEN":"ghu_keyring","OPENAI_API_KEY":"other"}`

// The keyring holds the only credential Copilot inference accepts, so it must
// beat the gh token the VM path used to settle for.
func TestResolvePrefersKeyringOverGhAuthToken(t *testing.T) {
	runner := &scriptedRunner{responses: map[string]string{
		"secret-tool": keyringEntry,
		"gh":          "gho_from_gh",
	}}
	got, err := Resolve(context.Background(), "github_copilot", "", map[string]string{}, runner)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Secret != "ghu_keyring" {
		t.Fatalf("Secret = %q, want the keyring token", got.Secret)
	}
	if got.Source != KeyringSource {
		t.Fatalf("Source = %q, want %q", got.Source, KeyringSource)
	}
	if got.Advisory != "" {
		t.Fatalf("a working Copilot credential needs no advisory, got %q", got.Advisory)
	}
}

// An explicit GITHUB_COPILOT_TOKEN is the contributor speaking directly; the
// keyring must not be touched at all in that case.
func TestResolveSkipsKeyringWhenTokenIsExported(t *testing.T) {
	runner := &scriptedRunner{responses: map[string]string{"secret-tool": keyringEntry}}
	got, err := Resolve(context.Background(), "github_copilot", "", map[string]string{"GITHUB_COPILOT_TOKEN": "ghu_env"}, runner)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Secret != "ghu_env" {
		t.Fatalf("Secret = %q, want the exported token", got.Secret)
	}
	if len(runner.called) != 0 {
		t.Fatalf("the keyring was probed unnecessarily: %v", runner.called)
	}
}

// No secret-tool, no session bus, a locked collection or a junk entry must all
// behave identically: fall through, never fail the launch.
func TestResolveKeyringFailureFallsThrough(t *testing.T) {
	for name, runner := range map[string]*scriptedRunner{
		"secret-tool missing": {responses: map[string]string{"gh": "gho_from_gh"}},
		"keyring locked":      {responses: map[string]string{"gh": "gho_from_gh"}, errs: map[string]error{"secret-tool": errors.New("no such collection")}},
		"malformed entry":     {responses: map[string]string{"secret-tool": "not json", "gh": "gho_from_gh"}},
		"entry without token": {responses: map[string]string{"secret-tool": `{"OPENAI_API_KEY":"x"}`, "gh": "gho_from_gh"}},
	} {
		got, err := Resolve(context.Background(), "github_copilot", "", map[string]string{}, runner)
		if err != nil {
			t.Fatalf("%s: Resolve() error = %v", name, err)
		}
		if got.Secret != "gho_from_gh" {
			t.Fatalf("%s: Secret = %q, want the gh fallback", name, got.Secret)
		}
	}
}

// The keyring is only meaningful for Copilot; other providers keep their own
// explicit configuration and must not have their keyring read behind them.
func TestResolveDoesNotReadKeyringForOtherProviders(t *testing.T) {
	runner := &scriptedRunner{responses: map[string]string{"secret-tool": keyringEntry}}
	if _, err := Resolve(context.Background(), "anthropic", "", map[string]string{}, runner); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("error = %v, want ErrNoCredential", err)
	}
	if len(runner.called) != 0 {
		t.Fatalf("the keyring was probed for a non-Copilot provider: %v", runner.called)
	}
}

// Settling for a gh token is allowed, but it must never be silent: Copilot
// rejects it and the failure otherwise appears inside the guest.
func TestResolveAdvisesWhenOnlyAGhTokenIsAvailable(t *testing.T) {
	cases := map[string]Resolved{}
	runner := &scriptedRunner{responses: map[string]string{"gh": "gho_from_gh"}}
	got, err := Resolve(context.Background(), "github_copilot", "", map[string]string{}, runner)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	cases["gh auth token"] = got

	got, err = Resolve(context.Background(), "github_copilot", "", map[string]string{"GITHUB_TOKEN": "gho_env"}, &scriptedRunner{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	cases["GITHUB_TOKEN"] = got

	for name, resolved := range cases {
		if resolved.Advisory == "" {
			t.Fatalf("%s: expected an advisory", name)
		}
		if !strings.Contains(resolved.Advisory, "goose configure") {
			t.Fatalf("%s: advisory must carry the fix, got %q", name, resolved.Advisory)
		}
		if strings.Contains(resolved.Advisory, resolved.Secret) {
			t.Fatalf("%s: advisory leaked the credential", name)
		}
	}
}
