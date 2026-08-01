package image_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestContainerfileIsPortableCompatibilityWrapper(t *testing.T) {
	data, err := os.ReadFile(repoFile(t, "image", "Containerfile"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	got := string(data)
	for _, want := range []string{
		"ARG HIVE_CONTRIBUTOR_IMAGE=ghcr.io/kubestellar/hive-contributor:latest",
		"FROM ${HIVE_CONTRIBUTOR_IMAGE}",
		"ARG GOOSE_REFRESH=0",
		"https://github.com/aaif-goose/goose/releases/latest/download/goose-${goose_arch}-unknown-linux-gnu.tar.gz",
		"install -m 0755 \"$workdir/goose\" /usr/local/bin/goose",
		"/usr/local/bin/goose run --help >/dev/null",
		"COPY --chmod=0755 image/compat-entrypoint.sh /usr/local/bin/donate-clanker-entrypoint",
		"USER dev",
		"WORKDIR /workspace",
		`ENTRYPOINT ["/usr/local/bin/donate-clanker-entrypoint"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Containerfile missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"/var/run/docker.sock",
		"/run/podman/podman.sock",
		"GOOSE_IMAGE",
		"ramalama",
	} {
		if strings.Contains(strings.ToLower(got), strings.ToLower(forbidden)) {
			t.Fatalf("Containerfile unexpectedly contains %q", forbidden)
		}
	}
}

func TestCompatibilityEntrypointMapsPortableMounts(t *testing.T) {
	data, err := os.ReadFile(repoFile(t, "image", "compat-entrypoint.sh"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	got := string(data)
	for _, want := range []string{
		`config_root="${DONATE_CLANKER_CONFIG_DIR:-/config}"`,
		`workspace="${DONATE_CLANKER_WORKSPACE_DIR:-/workspace}"`,
		`ln -s "$hive_config" "$HOME/.config/hive"`,
		`auth_env="$hive_config/gh-auth.env"`,
		`. "$auth_env"`,
		`export GH_TOKEN="${GH_TOKEN:-}"`,
		`ln -s "$workspace" "$HOME/workspace"`,
		`/usr/local/bin/contributor-agent.sh "$@" &`,
		`tmux has-session -t contributor`,
		`tmux attach-session -t contributor`,
		`tmux kill-session -t contributor`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("entrypoint missing %q", want)
		}
	}
	for _, forbidden := range []string{"/var/run/docker.sock", "/run/podman/podman.sock"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("entrypoint unexpectedly contains %q", forbidden)
		}
	}
}

func TestAgentContractFixtureIncludesRequiredPathsAndValidationCommands(t *testing.T) {
	data, err := os.ReadFile(repoFile(t, "image", "config", "agent-contract.json"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var manifest struct {
		Version           int `json:"version"`
		RequiredDocuments []struct {
			Path string `json:"path"`
		} `json:"required_documents"`
		Rules              []string `json:"rules"`
		ValidationCommands []string `json:"validation_commands"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if got, want := manifest.Version, 1; got != want {
		t.Fatalf("manifest.Version = %d, want %d", got, want)
	}

	paths := make([]string, len(manifest.RequiredDocuments))
	for i, doc := range manifest.RequiredDocuments {
		paths[i] = doc.Path
	}
	if got, want := paths, []string{
		"AGENTS.md",
		"docs/SKILL.md",
		"docs/skills/skill-improvement.md",
	}; !equalStrings(got, want) {
		t.Fatalf("required document paths = %#v, want %#v", got, want)
	}

	if got, want := manifest.Rules, []string{
		"Read AGENTS.md first, then docs/SKILL.md.",
		"If you discover a durable workaround, pattern, or convention, update the nearest docs/skills/*.md file in the same change.",
		"Use the repository validation command family: go test ./..., git diff --check, gofmt -l ., just --justfile just/61-donate-clanker.just --list.",
	}; !equalStrings(got, want) {
		t.Fatalf("manifest.Rules = %#v, want %#v", got, want)
	}

	if got, want := manifest.ValidationCommands, []string{
		"go test ./...",
		"git diff --check",
		"gofmt -l .",
		"just --justfile just/61-donate-clanker.just --list",
	}; !equalStrings(got, want) {
		t.Fatalf("manifest.ValidationCommands = %#v, want %#v", got, want)
	}
}

func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	return filepath.Join(append([]string{repoRoot(t)}, parts...)...)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
