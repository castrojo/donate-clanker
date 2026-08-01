// Package credential resolves the contributor's own model access so it can be
// passed through to Goose inside the VM.
//
// Goose stores provider secrets in the system keyring, so a launcher that
// refuses to look there cannot see the one credential Copilot inference
// accepts. This package therefore resolves a credential in a fixed order,
// treats the keyring as a best-effort source that can never fail a launch, and
// says plainly when the only credential it found will not satisfy the model.
package credential

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
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

	// Advisory is set when the resolved secret authenticates something, but
	// not model inference. A launch that proceeds on such a credential fails
	// far away from its cause -- inside the guest, at the agent's first model
	// call -- so the launcher says so up front instead.
	Advisory string
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
	// GITHUB_COPILOT_TOKEN first: Goose's github_copilot provider wants the
	// long-lived OAuth token from the Copilot editor device flow (a "ghu_"
	// user-to-server token), which is the only one its API accepts. A plain
	// GITHUB_TOKEN/GH_TOKEN is a different client with different scopes and
	// fails at the first model call with "failed to get api info", so they
	// stay only as last-resort fallbacks for contributors who set them
	// deliberately.
	"github_copilot": {"GITHUB_COPILOT_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"},
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
//  2. the login keyring, for github_copilot only
//  3. `gh auth token`, for github_copilot only
//
// Step 2 exists because Goose's github_copilot provider accepts exactly one
// credential -- the long-lived "ghu_" OAuth token minted by the Copilot editor
// device flow -- and on a desktop that token lives only in the keyring. The
// container path already reads it there; resolving it here keeps the VM path
// from booting on a credential Copilot rejects. It is strictly best-effort: a
// missing secret-tool, no session bus, a locked keyring or a malformed entry
// all fall through to the next source, and none of them can turn a launch that
// used to work into a failure.
//
// Step 3 yields a token Copilot inference rejects. It is kept because it still
// authenticates the GitHub API surface an agent uses, but the caller is handed
// an Advisory saying so, because otherwise the failure surfaces as an opaque
// "failed to get api info" inside the guest.
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
			if key != "GITHUB_COPILOT_TOKEN" && provider == DefaultProvider {
				resolved.Advisory = copilotAdvisory(key)
			}
			return resolved, nil
		}
	}

	if provider == DefaultProvider {
		if token := keyringCopilotToken(ctx, runner); token != "" {
			resolved.Secret = token
			resolved.Source = KeyringSource
			return resolved, nil
		}
		if token, err := ghAuthToken(ctx, runner); err == nil && token != "" {
			resolved.Secret = token
			resolved.Source = "gh auth token"
			resolved.Advisory = copilotAdvisory("gh auth token")
			return resolved, nil
		}
	}

	return Resolved{}, fmt.Errorf("%w for provider %q: %s", ErrNoCredential, provider, remedy(provider, keys))
}

// KeyringSource labels a credential read out of the login keyring.
const KeyringSource = "login keyring (secret-tool)"

// keyringTimeout bounds the keyring probe. secret-tool can block indefinitely
// on a locked collection waiting for an unlock prompt nobody is watching, and a
// best-effort lookup that hangs the launcher is worse than no lookup at all.
const keyringTimeout = 5 * time.Second

// keyringCopilotToken reads Goose's stored secrets from the login keyring.
// Every failure is silent on purpose: this source is an optimisation over
// making the contributor re-type a device code, never a precondition.
func keyringCopilotToken(ctx context.Context, runner Runner) string {
	lookupCtx, cancel := context.WithTimeout(ctx, keyringTimeout)
	defer cancel()

	out, err := runner.Output(lookupCtx, "secret-tool", "lookup", "service", "goose", "username", "secrets")
	if err != nil {
		return ""
	}
	return copilotTokenFromKeyringEntry(out)
}

// copilotTokenFromKeyringEntry pulls GITHUB_COPILOT_TOKEN out of the JSON blob
// Goose stores under service=goose, username=secrets. The shell path in
// just/61-donate-clanker.just has to do this with sed to stay a single line;
// here a real JSON decoder is free, so use one.
func copilotTokenFromKeyringEntry(raw []byte) string {
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil {
		return ""
	}
	var token string
	if err := json.Unmarshal(entry["GITHUB_COPILOT_TOKEN"], &token); err != nil {
		return ""
	}
	return strings.TrimSpace(token)
}

// copilotAdvisory explains, in the launcher's own voice, why a GitHub token is
// not a Copilot credential and what to do about it.
func copilotAdvisory(source string) string {
	return fmt.Sprintf(
		"%s authenticates the GitHub API but Copilot inference rejects it; the agent will fail its first model call with \"failed to get api info\". "+
			"Fix it by running `goose configure` on this host and completing the GitHub Copilot device flow, or by exporting GITHUB_COPILOT_TOKEN",
		source)
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
		// goose configure first: it mints the only credential Copilot
		// inference accepts, and it is the step a gh login cannot replace.
		return "run `goose configure` on this host and complete the GitHub Copilot device flow, or export GITHUB_COPILOT_TOKEN"
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
