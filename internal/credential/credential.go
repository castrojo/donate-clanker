// Package credential resolves the contributor's own model access so it can be
// passed through to Goose inside the VM.
//
// Goose stores provider secrets in the system keyring by default, which is not
// readable from a launcher. Rather than guessing at keyring internals, this
// package resolves a credential the host can actually produce, in a fixed
// order, and fails loudly when none is available.
package credential

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrNoCredential is returned when no supported credential source yields a
// secret for the selected provider.
var ErrNoCredential = errors.New("no model credential available")

// DefaultProvider matches the launcher default: GitHub Copilot, because a
// contributor who can open a pull request usually already has it.
const DefaultProvider = "github_copilot"

// Resolved is a provider/model/secret triple ready for bootstrap.
type Resolved struct {
	Provider string
	Model    string
	Secret   string
	Source   string
}

// Runner executes a host command. Injected so tests never shell out.
type Runner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct{}

func (ExecRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// providerEnvKeys maps a Goose provider to the environment variables that
// conventionally carry its API key.
var providerEnvKeys = map[string][]string{
	"github_copilot": {"GITHUB_TOKEN", "GH_TOKEN"},
	"anthropic":      {"ANTHROPIC_API_KEY"},
	"openai":         {"OPENAI_API_KEY"},
	"google":         {"GOOGLE_API_KEY", "GEMINI_API_KEY"},
	"openrouter":     {"OPENROUTER_API_KEY"},
	"ollama":         {},
}

// SupportedProviders lists providers this launcher can resolve a secret for.
func SupportedProviders() []string {
	names := make([]string, 0, len(providerEnvKeys))
	for name := range providerEnvKeys {
		names = append(names, name)
	}
	return names
}

// Resolve produces a credential for the given provider.
//
// Order, highest priority first:
//
//  1. the provider's conventional environment variable
//  2. `gh auth token`, for github_copilot only
//
// The system keyring is deliberately never consulted: it is not readable
// without a session bus and fails differently on every contributor's machine.
func Resolve(ctx context.Context, provider, model string, env map[string]string, runner Runner) (Resolved, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = DefaultProvider
	}
	if runner == nil {
		runner = ExecRunner{}
	}

	keys, known := providerEnvKeys[provider]
	if !known {
		return Resolved{}, fmt.Errorf("%w: unsupported provider %q", ErrNoCredential, provider)
	}

	resolved := Resolved{Provider: provider, Model: strings.TrimSpace(model)}

	// Ollama is local and needs no secret.
	if provider == "ollama" {
		resolved.Source = "none required"
		return resolved, nil
	}

	for _, key := range keys {
		if value := strings.TrimSpace(env[key]); value != "" {
			resolved.Secret = value
			resolved.Source = "environment " + key
			return resolved, nil
		}
	}

	if provider == DefaultProvider {
		if token, err := ghAuthToken(ctx, runner); err == nil && token != "" {
			resolved.Secret = token
			resolved.Source = "gh auth token"
			return resolved, nil
		}
	}

	return Resolved{}, fmt.Errorf("%w for provider %q: %s", ErrNoCredential, provider, remedy(provider, keys))
}

func ghAuthToken(ctx context.Context, runner Runner) (string, error) {
	out, err := runner.Output(ctx, "gh", "auth", "token")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func remedy(provider string, keys []string) string {
	if provider == DefaultProvider {
		return "run `gh auth login --web --hostname github.com --scopes repo,read:org`, or export GITHUB_TOKEN"
	}
	if len(keys) == 0 {
		return "no credential source is defined for this provider"
	}
	return "export " + strings.Join(keys, " or ")
}

// EnvironmentMap converts os.Environ()-style pairs into a lookup map.
func EnvironmentMap(values []string) map[string]string {
	env := make(map[string]string, len(values))
	for _, item := range values {
		if key, value, ok := strings.Cut(item, "="); ok {
			env[key] = value
		}
	}
	return env
}

// HostEnvironment reads the live process environment.
func HostEnvironment() map[string]string { return EnvironmentMap(os.Environ()) }
