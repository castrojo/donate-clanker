package setup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckGitHubAuthMissingGH(t *testing.T) {
	runner := fakeCommandRunner{
		err: exec.ErrNotFound,
	}

	err := CheckGitHubAuth(context.Background(), &runner)
	if err == nil {
		t.Fatal("CheckGitHubAuth() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "gh auth login --web --hostname github.com --scopes repo,read:org") {
		t.Fatalf("CheckGitHubAuth() error = %q, want login command", err)
	}
}

func TestCheckGitHubAuthInvalidSessionRedactsSecrets(t *testing.T) {
	runner := fakeCommandRunner{
		result: CommandResult{
			Stderr: "github.com\n  X authentication failed\n  GH_TOKEN=gho_super_secret\n",
		},
		err: errors.New("exit status 1"),
	}

	err := CheckGitHubAuth(context.Background(), &runner)
	if err == nil {
		t.Fatal("CheckGitHubAuth() error = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "authentication failed") {
		t.Fatalf("CheckGitHubAuth() error = %q, want stderr detail", got)
	}
	if strings.Contains(err.Error(), "gho_super_secret") {
		t.Fatalf("CheckGitHubAuth() error leaked secret: %q", err)
	}
}

func TestCheckGitHubAuthUsesGitHubHostname(t *testing.T) {
	runner := fakeCommandRunner{}

	_ = CheckGitHubAuth(context.Background(), &runner)

	if runner.name != "gh" {
		t.Fatalf("runner name = %q, want gh", runner.name)
	}
	wantArgs := []string{"auth", "status", "--hostname", "github.com"}
	if strings.Join(runner.args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("runner args = %q, want %q", runner.args, wantArgs)
	}
}

func TestValidateHiveConfigMissingFile(t *testing.T) {
	_, path := repoScratchPath(t, "missing-contributor.env")

	err := ValidateHiveConfig(path)
	if !errors.Is(err, ErrMissingFile) {
		t.Fatalf("ValidateHiveConfig() error = %v, want ErrMissingFile", err)
	}
}

func TestValidateHiveConfigRejectsEmptyRequiredValuesWithoutLeakingSecrets(t *testing.T) {
	path := writeScratchFile(t, "contributor.env", strings.Join([]string{
		"HIVE_REGISTRATION_TOKEN=super-secret-token",
		"HIVE_HUB=",
		"CONTRIBUTOR_ID=abc123",
		"CONTRIBUTOR_USERNAME=castrojo",
		"AGENT_BACKEND=goose",
		"",
	}, "\n"))

	err := ValidateHiveConfig(path)
	if err == nil {
		t.Fatal("ValidateHiveConfig() error = nil, want error")
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ValidateHiveConfig() error = %v, want ErrInvalidConfig", err)
	}
	if got := err.Error(); !strings.Contains(got, "HIVE_HUB") {
		t.Fatalf("ValidateHiveConfig() error = %q, want missing key", got)
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("ValidateHiveConfig() error leaked secret: %q", err)
	}
}

func TestCheckGooseLocalConfigRejectsMissingEndpointWithoutLeakingSecrets(t *testing.T) {
	path := writeScratchFile(t, "config.yaml", strings.Join([]string{
		"provider: openai",
		"base_url:",
		"api_key: local-secret",
		"model: local",
		"",
	}, "\n"))

	err := CheckGooseLocalConfig(path)
	if err == nil {
		t.Fatal("CheckGooseLocalConfig() error = nil, want error")
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("CheckGooseLocalConfig() error = %v, want ErrInvalidConfig", err)
	}
	if got := err.Error(); !strings.Contains(got, "base_url") {
		t.Fatalf("CheckGooseLocalConfig() error = %q, want missing key", got)
	}
	if strings.Contains(err.Error(), "local-secret") {
		t.Fatalf("CheckGooseLocalConfig() error leaked secret: %q", err)
	}
}

func TestValidateHiveConfigAcceptsSupportedGooseSetup(t *testing.T) {
	path := writeScratchFile(t, "valid-contributor.env", strings.Join([]string{
		"HIVE_REGISTRATION_TOKEN=redacted",
		"HIVE_HUB=wss://example.hive.kubestellar.io/contribute",
		"CONTRIBUTOR_ID=abc123",
		"CONTRIBUTOR_USERNAME=castrojo",
		"AGENT_BACKEND=goose",
		"",
	}, "\n"))

	if err := ValidateHiveConfig(path); err != nil {
		t.Fatalf("ValidateHiveConfig() error = %v, want nil", err)
	}
}

func TestValidateHiveConfigRejectsNonContributeHubURL(t *testing.T) {
	path := writeScratchFile(t, "invalid-hub.env", strings.Join([]string{
		"HIVE_REGISTRATION_TOKEN=redacted",
		"HIVE_HUB=https://example.hive.kubestellar.io",
		"CONTRIBUTOR_ID=abc123",
		"CONTRIBUTOR_USERNAME=castrojo",
		"AGENT_BACKEND=goose",
		"",
	}, "\n"))

	err := ValidateHiveConfig(path)
	if err == nil {
		t.Fatal("ValidateHiveConfig() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "HIVE_HUB") {
		t.Fatalf("ValidateHiveConfig() error = %q, want HIVE_HUB detail", err)
	}
}

func TestValidateHiveConfigRejectsUnencryptedHubURL(t *testing.T) {
	path := writeScratchFile(t, "unencrypted-hub.env", strings.Join([]string{
		"HIVE_REGISTRATION_TOKEN=redacted",
		"HIVE_HUB=ws://example.hive.kubestellar.io/contribute",
		"CONTRIBUTOR_ID=abc123",
		"CONTRIBUTOR_USERNAME=castrojo",
		"AGENT_BACKEND=goose",
		"",
	}, "\n"))

	err := ValidateHiveConfig(path)
	if err == nil {
		t.Fatal("ValidateHiveConfig() error = nil, want error")
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ValidateHiveConfig() error = %v, want ErrInvalidConfig", err)
	}
	if !strings.Contains(err.Error(), "use wss") {
		t.Fatalf("ValidateHiveConfig() error = %q, want wss detail", err)
	}
}

func TestCheckGooseLocalConfigAcceptsLocalOpenAISetup(t *testing.T) {
	path := writeScratchFile(t, "valid-config.yaml", strings.Join([]string{
		"provider: openai",
		"base_url: http://127.0.0.1:8000/v1",
		"api_key: local",
		"model: local",
		"",
	}, "\n"))

	if err := CheckGooseLocalConfig(path); err != nil {
		t.Fatalf("CheckGooseLocalConfig() error = %v, want nil", err)
	}
}

func TestCheckGooseLocalConfigRejectsUnsupportedProvider(t *testing.T) {
	path := writeScratchFile(t, "invalid-provider.yaml", strings.Join([]string{
		"provider: anthropic",
		"base_url: http://127.0.0.1:8000/v1",
		"api_key: local",
		"model: local",
		"",
	}, "\n"))

	err := CheckGooseLocalConfig(path)
	if err == nil {
		t.Fatal("CheckGooseLocalConfig() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "provider must be openai") {
		t.Fatalf("CheckGooseLocalConfig() error = %q, want provider detail", err)
	}
}

type fakeCommandRunner struct {
	result CommandResult
	err    error
	name   string
	args   []string
}

func (f *fakeCommandRunner) Run(_ context.Context, name string, args ...string) (CommandResult, error) {
	f.name = name
	f.args = append([]string(nil), args...)
	return f.result, f.err
}

func repoScratchPath(t *testing.T, name string) (string, string) {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "internal", "setup", ".auth-test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir, filepath.Join(dir, name)
}

func writeScratchFile(t *testing.T, name string, contents string) string {
	t.Helper()
	_, path := repoScratchPath(t, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
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
