package image_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestContainerfileUsesUpstreamGooseRuntime(t *testing.T) {
	data, err := os.ReadFile(repoFile(t, "image", "Containerfile"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	got := string(data)
	for _, want := range []string{
		"ARG GOOSE_IMAGE=ghcr.io/aaif-goose/goose@sha256:3e2f39d15e26198f14807966109cb48992156390b969d4f292d84663e5e161bc",
		"FROM ${GOOSE_IMAGE}",
		"COPY --from=build --chown=goose:goose /out/contributor /usr/local/bin/contributor",
		"COPY --chown=goose:goose image/config/ /etc/donate-clanker/",
		"RUN install -d -o goose -g goose /etc/donate-clanker /var/lib/donate-clanker",
		"ENV DONATE_CLANKER_RUNTIME_DIR=/var/lib/donate-clanker",
		"USER goose",
		`ENTRYPOINT ["/usr/local/bin/contributor"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Containerfile missing %q", want)
		}
	}
	if strings.Contains(got, "GOARCH=amd64") {
		t.Fatal("Containerfile should not hard-code amd64")
	}
	if !regexp.MustCompile(`(?m)^FROM golang:1\.22-bookworm@sha256:[0-9a-f]{64} AS build$`).MatchString(got) {
		t.Fatal("Containerfile should build contributor in a dedicated Go stage")
	}
	for _, forbidden := range []string{"/var/run/docker.sock", "/run/podman/podman.sock"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("Containerfile unexpectedly contains %q", forbidden)
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
