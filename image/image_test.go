package image_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestContainerfileDerivesFromHiveAndLayersContext(t *testing.T) {
	got := readRepoFile(t, "image", "Containerfile")

	for _, want := range []string{
		// Digest-pinned, not tagged: Hive's :latest moves with its v2 branch.
		// Assert the pinned prefix only so Renovate digest bumps stay green.
		"ARG HIVE_CONTRIBUTOR_IMAGE=ghcr.io/kubestellar/hive-contributor@sha256:",
		"FROM ${HIVE_CONTRIBUTOR_IMAGE}",
		"ARG GOOSE_REFRESH=0",
		"https://github.com/aaif-goose/goose/releases/latest/download/goose-${goose_arch}-unknown-linux-gnu.tar.gz",
		"install -m 0755 \"$workdir/goose\" /usr/local/bin/goose",
		"/usr/local/bin/goose run --help >/dev/null",
		// The controlled Goose config must not land on the path Hive rewrites.
		"image/config/goose.yaml /opt/bluefin/goose/config/config.yaml",
		"COPY --chmod=0755 image/git-hooks/ /opt/bluefin/git-hooks/",
		"--out /home/dev/.agents/skills",
		"COPY --chmod=0755 image/entrypoint.sh /usr/local/bin/donate-clanker-entrypoint",
		"USER dev",
		// Hive's own image uses /home/dev; /workspace was never a Hive convention.
		"WORKDIR /home/dev",
		`ENTRYPOINT ["/usr/local/bin/donate-clanker-entrypoint"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Containerfile missing %q", want)
		}
	}

	for _, forbidden := range []string{
		"/var/run/docker.sock",
		"/run/podman/podman.sock",
		"ramalama",
		"models.json",
		"agent-contract.json",
		// Replacing Hive's gh wrapper would silently undo its restrictions.
		"/usr/local/bin/gh",
	} {
		if strings.Contains(strings.ToLower(got), strings.ToLower(forbidden)) {
			t.Fatalf("Containerfile unexpectedly contains %q", forbidden)
		}
	}
}

// The controlled config exists only because Hive overwrites
// ~/.config/goose/config.yaml on every start. It must not pin a provider or
// model, which the launcher passes through from the contributor's own account.
func TestGooseConfigIsPassthroughAndBindsContext7(t *testing.T) {
	got := readRepoFile(t, "image", "config", "goose.yaml")

	for _, want := range []string{
		"type: streamable_http",
		"name: context7",
		"enabled: true",
		`uri: "https://mcp.context7.com/mcp"`,
		"GOOSE_MODE: smart_approve",
		"GOOSE_MAX_TOOL_RESPONSE_SIZE:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("goose.yaml missing %q", want)
		}
	}

	for _, forbidden := range []string{
		"GOOSE_PROVIDER",
		"GOOSE_MODEL",
		"127.0.0.1:8000",
		"api_key",
	} {
		if strings.Contains(stripYAMLComments(got), forbidden) {
			t.Fatalf("goose.yaml unexpectedly pins %q", forbidden)
		}
	}
}

// stripYAMLComments removes whole-line and trailing comments so contract checks
// inspect configuration rather than the prose explaining it.
func stripYAMLComments(text string) string {
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if idx := strings.Index(line, "#"); idx != -1 {
			line = line[:idx]
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestEntrypointWrapsHiveWithoutOwningItsLifecycle(t *testing.T) {
	got := readRepoFile(t, "image", "entrypoint.sh")

	for _, want := range []string{
		// GOOSE_PATH_ROOT is the whole reason our config survives Hive's rewrite.
		`export GOOSE_PATH_ROOT=`,
		"GOOSE_DISABLE_KEYRING=1",
		// Hive's knowledge export lands on CLAUDE.md, which Goose ignores by default.
		"CONTEXT_FILE_NAMES",
		"CLAUDE.md",
		"core.hooksPath /opt/bluefin/git-hooks",
		"mcp.context7.com",
		`/usr/local/bin/contributor-agent.sh "$@" &`,
		"tmux has-session -t contributor",
		"tmux attach-session -t contributor",
		"tmux kill-session -t contributor",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("entrypoint missing %q", want)
		}
	}

	for _, forbidden := range []string{
		"/var/run/docker.sock",
		"/run/podman/podman.sock",
		// Invented mount points that Hive does not use.
		`:-/config}`,
		`:-/workspace}`,
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("entrypoint unexpectedly contains %q", forbidden)
		}
	}
}

// Hooks run in every repository via a global core.hooksPath, so they must never
// claim to be enforcement: --no-verify bypasses all of them.
func TestGitHooksAreHonestAboutBeingBypassable(t *testing.T) {
	for _, hook := range []string{"pre-commit", "commit-msg", "post-checkout"} {
		got := readRepoFile(t, "image", "git-hooks", hook)
		if !strings.HasPrefix(got, "#!") {
			t.Fatalf("%s: missing shebang", hook)
		}
		if hook == "post-checkout" {
			continue
		}
		if !strings.Contains(got, "no-verify") {
			t.Fatalf("%s: must document that --no-verify bypasses it", hook)
		}
	}
}

func TestPostCheckoutExcludesGeneratedSkillProjections(t *testing.T) {
	got := readRepoFile(t, "image", "git-hooks", "post-checkout")
	for _, want := range []string{"info/exclude", ".agents/skills/", "docs/skills/index.json"} {
		if !strings.Contains(got, want) {
			t.Fatalf("post-checkout missing %q", want)
		}
	}
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(repoFile(t, parts...))
	if err != nil {
		t.Fatalf("ReadFile(%v) error = %v", parts, err)
	}
	return string(data)
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
