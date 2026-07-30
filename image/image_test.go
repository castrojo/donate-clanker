package image_test

import (
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
		"ARG HIVE_CONTRIBUTOR_IMAGE=ghcr.io/kubestellar/hive-contributor@sha256:1ccbf9bdf9c5b8fb6b8d5d4b6b19ceb07852fc08f62ffa8cad7d8f00781737a4",
		"FROM ${HIVE_CONTRIBUTOR_IMAGE}",
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
		`ln -s "$workspace" "$HOME/workspace"`,
		`exec /usr/local/bin/contributor-agent.sh "$@"`,
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

func TestREADMELabelsImageAsCompatibilityMode(t *testing.T) {
	data, err := os.ReadFile(repoFile(t, "README.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	got := string(data)
	for _, want := range []string{
		"compatibility mode",
		"does **not**",
		"native Goose/RamaLama launcher",
		"no host container socket",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("README missing compatibility boundary %q", want)
		}
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
